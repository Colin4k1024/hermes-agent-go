// Package batch provides batch trajectory generation for running multiple
// prompts in parallel and collecting their results.
package batch

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/agent"
)

// BatchConfig describes the parameters for a batch run.
type BatchConfig struct {
	// Prompts is the list of prompts to run.
	Prompts []string

	// Model is the model to use for all prompts.
	Model string

	// MaxWorkers is the maximum number of concurrent goroutines.
	// Defaults to 4 if zero.
	MaxWorkers int

	// OutputDir is where trajectory files are saved.
	// Defaults to ~/.hermes/batch_output if empty.
	OutputDir string

	// ToolSets specifies which toolsets to enable for the batch agents.
	ToolSets []string

	// MaxIterations limits iterations per prompt. Defaults to 30 if zero.
	MaxIterations int
}

// BatchResult holds the outcome of a single prompt execution.
type BatchResult struct {
	Prompt    string        `json:"prompt"`
	Response  string        `json:"response"`
	ToolCalls int           `json:"tool_calls"`
	Tokens    int           `json:"tokens"`
	Duration  time.Duration `json:"duration_ns"`
	Error     string        `json:"error,omitempty"`
}

// RunBatch runs multiple prompts in parallel using a goroutine pool.
// It saves trajectories to the output directory and returns a summary.
func RunBatch(cfg BatchConfig) ([]BatchResult, error) {
	if len(cfg.Prompts) == 0 {
		return nil, fmt.Errorf("no prompts provided")
	}

	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 4
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 30
	}
	if cfg.OutputDir == "" {
		home, _ := os.UserHomeDir()
		cfg.OutputDir = filepath.Join(home, ".hermes", "batch_output")
	}

	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	results := make([]BatchResult, len(cfg.Prompts))
	sem := make(chan struct{}, cfg.MaxWorkers)
	var wg sync.WaitGroup

	for i, prompt := range cfg.Prompts {
		wg.Add(1)
		sem <- struct{}{} // acquire worker slot

		go func(idx int, p string) {
			defer wg.Done()
			defer func() { <-sem }() // release worker slot

			results[idx] = runSinglePrompt(p, cfg)
		}(i, prompt)
	}

	wg.Wait()

	// Save summary.
	if err := saveBatchSummary(cfg.OutputDir, results); err != nil {
		slog.Warn("Failed to save batch summary", "error", err)
	}

	return results, nil
}

// runSinglePrompt creates an agent and runs a single prompt, returning the result.
func runSinglePrompt(prompt string, cfg BatchConfig) BatchResult {
	start := time.Now()

	result := BatchResult{
		Prompt: prompt,
	}

	// Build agent options.
	opts := []agent.AgentOption{
		agent.WithPlatform("batch"),
		agent.WithQuietMode(true),
		agent.WithPersistSession(false),
		agent.WithMaxIterations(cfg.MaxIterations),
	}

	if cfg.Model != "" {
		opts = append(opts, agent.WithModel(cfg.Model))
	}
	if len(cfg.ToolSets) > 0 {
		opts = append(opts, agent.WithEnabledToolsets(cfg.ToolSets))
	}

	ag, err := agent.New(opts...)
	if err != nil {
		result.Error = fmt.Sprintf("create agent: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer ag.Close()

	convResult, err := ag.RunConversation(prompt, nil)
	if err != nil {
		result.Error = fmt.Sprintf("conversation: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Response = convResult.FinalResponse
	result.Tokens = convResult.TotalTokens
	result.Duration = time.Since(start)

	// Count tool calls across all messages.
	for _, msg := range convResult.Messages {
		result.ToolCalls += len(msg.ToolCalls)
	}

	// Save trajectory.
	traj := agent.NewTrajectoryFromResult(convResult, ag.SessionID(), result.Duration)
	if err := agent.SaveTrajectory(traj, cfg.OutputDir); err != nil {
		slog.Warn("Failed to save trajectory", "error", err, "prompt", truncatePrompt(prompt))
	}

	return result
}

// saveBatchSummary writes a JSON summary of all batch results.
func saveBatchSummary(outputDir string, results []BatchResult) error {
	summary := map[string]any{
		"timestamp":    time.Now().Format(time.RFC3339),
		"total":        len(results),
		"succeeded":    countSucceeded(results),
		"failed":       countFailed(results),
		"total_tokens": sumTokens(results),
		"results":      results,
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(outputDir, fmt.Sprintf("batch_summary_%s.json",
		time.Now().Format("20060102_150405")))
	return os.WriteFile(path, data, 0644)
}

func countSucceeded(results []BatchResult) int {
	n := 0
	for _, r := range results {
		if r.Error == "" {
			n++
		}
	}
	return n
}

func countFailed(results []BatchResult) int {
	n := 0
	for _, r := range results {
		if r.Error != "" {
			n++
		}
	}
	return n
}

func sumTokens(results []BatchResult) int {
	total := 0
	for _, r := range results {
		total += r.Tokens
	}
	return total
}

func truncatePrompt(s string) string {
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}
