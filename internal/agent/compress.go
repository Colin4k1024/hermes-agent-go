package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

const compressionThreshold = 0.75 // Compress when 75% of context is used

// ShouldCompress returns true if the conversation should be compressed.
func (a *AIAgent) ShouldCompress(messages []llm.Message) bool {
	meta := llm.GetModelMeta(a.model)
	totalTokens := 0
	for _, m := range messages {
		totalTokens += llm.EstimateTokens(m.Content)
		for _, tc := range m.ToolCalls {
			totalTokens += llm.EstimateTokens(tc.Function.Arguments)
		}
	}
	totalTokens += llm.EstimateTokens(a.systemPrompt)

	threshold := int(float64(meta.ContextLength) * compressionThreshold)
	return totalTokens > threshold
}

// CompressContext summarizes older messages to free context space.
func (a *AIAgent) CompressContext(ctx context.Context, messages []llm.Message) ([]llm.Message, error) {
	if len(messages) < 4 {
		return messages, nil
	}

	slog.Info("Compressing context", "message_count", len(messages))
	a.fireStatus("Compressing context...")

	// Keep last 4 messages, summarize the rest
	keepCount := 4
	if len(messages) <= keepCount {
		return messages, nil
	}

	toSummarize := messages[:len(messages)-keepCount]
	toKeep := messages[len(messages)-keepCount:]

	// Build summary request
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation history concisely. ")
	sb.WriteString("Focus on key decisions, facts learned, and current task state. ")
	sb.WriteString("Keep it under 500 words.\n\n")

	for _, m := range toSummarize {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, truncate(m.Content, 500)))
	}

	summaryReq := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "user", Content: sb.String()},
		},
	}

	resp, err := a.client.CreateChatCompletion(ctx, summaryReq)
	if err != nil {
		slog.Warn("Context compression failed", "error", err)
		return messages, nil // Return original on failure
	}

	// Build compressed message list
	compressed := []llm.Message{
		{
			Role:    "system",
			Content: fmt.Sprintf("[Context Summary]\n%s", resp.Content),
		},
	}
	compressed = append(compressed, toKeep...)

	slog.Info("Context compressed", "from", len(messages), "to", len(compressed))
	return compressed, nil
}
