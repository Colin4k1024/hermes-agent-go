package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// Trajectory represents a recorded agent session for RL training data.
type Trajectory struct {
	SessionID string            `json:"session_id"`
	Model     string            `json:"model"`
	Messages  []llm.Message     `json:"messages"`
	ToolCalls int               `json:"tool_calls"`
	Tokens    map[string]int    `json:"tokens"`
	Duration  time.Duration     `json:"duration_ns"`
	Completed bool              `json:"completed"`
	Timestamp time.Time         `json:"timestamp"`
}

// SaveTrajectory serializes a trajectory to a JSON file in the given output directory.
// The filename is derived from the session ID and timestamp.
func SaveTrajectory(t *Trajectory, outputDir string) error {
	if t == nil {
		return fmt.Errorf("trajectory is nil")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Build a safe filename.
	ts := t.Timestamp.Format("20060102_150405")
	safeID := sanitizeFilename(t.SessionID)
	if len(safeID) > 32 {
		safeID = safeID[:32]
	}
	filename := fmt.Sprintf("traj_%s_%s.json", ts, safeID)
	path := filepath.Join(outputDir, filename)

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trajectory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write trajectory file: %w", err)
	}

	return nil
}

// LoadTrajectory reads a trajectory from a JSON file.
func LoadTrajectory(path string) (*Trajectory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trajectory file: %w", err)
	}

	var t Trajectory
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("unmarshal trajectory: %w", err)
	}

	return &t, nil
}

// CompressTrajectory returns a new trajectory with verbose tool results
// trimmed down. This reduces the data size for RL training while preserving
// the essential structure (roles, tool names, abbreviated results).
func CompressTrajectory(t *Trajectory) *Trajectory {
	if t == nil {
		return nil
	}

	compressed := &Trajectory{
		SessionID: t.SessionID,
		Model:     t.Model,
		ToolCalls: t.ToolCalls,
		Tokens:    t.Tokens,
		Duration:  t.Duration,
		Completed: t.Completed,
		Timestamp: t.Timestamp,
	}

	// Copy and compress messages.
	compressed.Messages = make([]llm.Message, len(t.Messages))
	for i, msg := range t.Messages {
		m := msg // copy
		if m.Role == "tool" && len(m.Content) > maxCompressedToolResult {
			// Truncate verbose tool results but keep the beginning for context.
			m.Content = m.Content[:maxCompressedToolResult] + fmt.Sprintf("\n... [truncated, %d chars total]", len(msg.Content))
		}
		// Strip reasoning from compressed trajectories to save space.
		m.Reasoning = ""
		m.ReasoningContent = ""
		compressed.Messages[i] = m
	}

	return compressed
}

const maxCompressedToolResult = 500

// NewTrajectoryFromResult creates a Trajectory from a ConversationResult.
func NewTrajectoryFromResult(result *ConversationResult, sessionID string, duration time.Duration) *Trajectory {
	toolCalls := 0
	for _, msg := range result.Messages {
		toolCalls += len(msg.ToolCalls)
	}

	tokens := map[string]int{
		"input":       result.InputTokens,
		"output":      result.OutputTokens,
		"total":       result.TotalTokens,
		"cache_read":  result.CacheReadTokens,
		"cache_write": result.CacheWriteTokens,
		"reasoning":   result.ReasoningTokens,
	}

	return &Trajectory{
		SessionID: sessionID,
		Model:     result.Model,
		Messages:  result.Messages,
		ToolCalls: toolCalls,
		Tokens:    tokens,
		Duration:  duration,
		Completed: result.Completed,
		Timestamp: time.Now(),
	}
}

// sanitizeFilename replaces unsafe characters in a string for use as a filename.
func sanitizeFilename(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, s)
}
