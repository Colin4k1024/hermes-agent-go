package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
	"github.com/hermes-agent/hermes-agent-go/internal/store"
	storeRedis "github.com/hermes-agent/hermes-agent-go/internal/store/rediscache"
)

// AgentFactory creates stateless agent executions backed by external state.
type AgentFactory struct {
	llmClient  *llm.Client
	store      store.Store
	redis      *storeRedis.Client // may be nil for local mode
	maxHistory int                // max messages to load (default 50)
}

// FactoryConfig holds configuration for creating an AgentFactory.
type FactoryConfig struct {
	LLMClient  *llm.Client
	Store      store.Store
	Redis      *storeRedis.Client
	MaxHistory int
}

// NewAgentFactory creates a new stateless agent factory.
func NewAgentFactory(cfg FactoryConfig) *AgentFactory {
	maxHistory := cfg.MaxHistory
	if maxHistory <= 0 {
		maxHistory = 50
	}
	return &AgentFactory{
		llmClient:  cfg.LLMClient,
		store:      cfg.Store,
		redis:      cfg.Redis,
		maxHistory: maxHistory,
	}
}

// ChatRequest represents an incoming chat request with tenant context.
type ChatRequest struct {
	TenantID  string
	SessionID string
	UserID    string
	Platform  string
	Text      string
	Model     string
}

// ChatResult represents the output of a stateless agent execution.
type ChatResult struct {
	Response  string
	SessionID string
	Tokens    TokenUsage
	Completed bool
}

// TokenUsage tracks token consumption for a turn.
type TokenUsage struct {
	Input  int
	Output int
}

// Run executes a stateless agent turn: load context → run → persist.
func (f *AgentFactory) Run(ctx context.Context, req ChatRequest) (*ChatResult, error) {
	tenantID := req.TenantID
	sessionID := req.SessionID

	// Acquire session lock if Redis available
	if f.redis != nil {
		acquired, err := f.redis.AcquireSessionLock(ctx, tenantID, sessionID, 30*time.Second)
		if err != nil {
			slog.Warn("Failed to acquire session lock", "error", err)
		} else if !acquired {
			return nil, fmt.Errorf("session %s is being processed by another instance", sessionID)
		}
		defer f.redis.ReleaseSessionLock(ctx, tenantID, sessionID)
	}

	// Load context: recent messages from store
	messages, err := f.loadContext(ctx, tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load context: %w", err)
	}

	// Append user message
	userMsg := llm.Message{Role: "user", Content: req.Text}
	messages = append(messages, userMsg)

	// Persist user message
	f.store.Messages().Append(ctx, tenantID, sessionID, &store.Message{
		SessionID: sessionID,
		Role:      "user",
		Content:   req.Text,
		Timestamp: time.Now(),
	})

	// Create temporary agent and run
	ag, agErr := New(
		WithModel(req.Model),
		WithSessionID(sessionID),
		WithPlatform(req.Platform),
		WithSkipMemory(true),
		WithPersistSession(false),
	)
	if agErr != nil {
		return nil, fmt.Errorf("create agent: %w", agErr)
	}

	result, err := ag.RunConversation(req.Text, messages[:len(messages)-1])
	if err != nil {
		return nil, fmt.Errorf("agent run: %w", err)
	}

	// Persist assistant response
	if result.FinalResponse != "" {
		f.store.Messages().Append(ctx, tenantID, sessionID, &store.Message{
			SessionID: sessionID,
			Role:      "assistant",
			Content:   result.FinalResponse,
			Reasoning: result.LastReasoning,
			Timestamp: time.Now(),
		})
	}

	// Update token counts
	f.store.Sessions().UpdateTokens(ctx, tenantID, sessionID, store.TokenDelta{
		Input:  result.InputTokens,
		Output: result.OutputTokens,
	})

	return &ChatResult{
		Response:  result.FinalResponse,
		SessionID: sessionID,
		Tokens:    TokenUsage{Input: result.InputTokens, Output: result.OutputTokens},
		Completed: result.Completed,
	}, nil
}

func (f *AgentFactory) loadContext(ctx context.Context, tenantID, sessionID string) ([]llm.Message, error) {
	// Check Redis cache first
	if f.redis != nil {
		cached, _ := f.redis.GetContextCache(ctx, tenantID, sessionID)
		if cached != "" {
			return []llm.Message{{Role: "system", Content: cached}}, nil
		}
	}

	// Load from store
	msgs, err := f.store.Messages().List(ctx, tenantID, sessionID, f.maxHistory, 0)
	if err != nil {
		return nil, err
	}

	var llmMsgs []llm.Message
	for _, m := range msgs {
		llmMsgs = append(llmMsgs, llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolName:   m.ToolName,
			Reasoning:  m.Reasoning,
		})
	}

	return llmMsgs, nil
}
