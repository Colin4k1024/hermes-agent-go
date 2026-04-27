package agent

import (
	"context"
	"log/slog"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
)

// ContextLoader builds the LLM message history from store for stateless execution.
type ContextLoader struct {
	store      store.Store
	maxHistory int
}

// NewContextLoader creates a context loader with the given history window.
func NewContextLoader(s store.Store, maxHistory int) *ContextLoader {
	if maxHistory <= 0 {
		maxHistory = 50
	}
	return &ContextLoader{store: s, maxHistory: maxHistory}
}

// Load retrieves the most recent messages for a session, respecting the window limit.
// If message count exceeds maxHistory, only the last maxHistory messages are returned.
func (cl *ContextLoader) Load(ctx context.Context, tenantID, sessionID string) ([]llm.Message, error) {
	total, err := cl.store.Messages().CountBySession(ctx, tenantID, sessionID)
	if err != nil {
		return nil, err
	}

	offset := 0
	if total > cl.maxHistory {
		offset = total - cl.maxHistory
		slog.Info("Context window trimmed", "total", total, "loaded", cl.maxHistory, "session", sessionID)
	}

	msgs, err := cl.store.Messages().List(ctx, tenantID, sessionID, cl.maxHistory, offset)
	if err != nil {
		return nil, err
	}

	var result []llm.Message
	for _, m := range msgs {
		msg := llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolName:   m.ToolName,
			Reasoning:  m.Reasoning,
		}
		result = append(result, msg)
	}

	return result, nil
}

// ShouldCompress checks if the loaded context needs compression based on token usage ratio.
func ShouldCompress(messages []llm.Message, contextLength int) bool {
	if contextLength <= 0 {
		return false
	}
	totalTokens := 0
	for _, m := range messages {
		totalTokens += llm.EstimateTokens(m.Content)
	}
	ratio := float64(totalTokens) / float64(contextLength)
	return ratio > 0.70
}
