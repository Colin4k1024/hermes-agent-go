package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

func TestMCPMessagesToLLM(t *testing.T) {
	tests := []struct {
		name         string
		messages     []mcpSamplingMessage
		systemPrompt string
		wantLen      int
		wantFirst    string // role of first message
		wantLast     string // role of last message
	}{
		{
			name: "basic user message",
			messages: []mcpSamplingMessage{
				{Role: "user", Content: mcpSamplingContent{Type: "text", Text: "hello"}},
			},
			wantLen:   1,
			wantFirst: "user",
			wantLast:  "user",
		},
		{
			name: "with system prompt",
			messages: []mcpSamplingMessage{
				{Role: "user", Content: mcpSamplingContent{Type: "text", Text: "hello"}},
			},
			systemPrompt: "You are a helpful assistant",
			wantLen:      2,
			wantFirst:    "system",
			wantLast:     "user",
		},
		{
			name: "multi-turn conversation",
			messages: []mcpSamplingMessage{
				{Role: "user", Content: mcpSamplingContent{Type: "text", Text: "hello"}},
				{Role: "assistant", Content: mcpSamplingContent{Type: "text", Text: "hi there"}},
				{Role: "user", Content: mcpSamplingContent{Type: "text", Text: "how are you?"}},
			},
			wantLen:   3,
			wantFirst: "user",
			wantLast:  "user",
		},
		{
			name: "unknown role defaults to user",
			messages: []mcpSamplingMessage{
				{Role: "tool", Content: mcpSamplingContent{Type: "text", Text: "data"}},
			},
			wantLen:   1,
			wantFirst: "user",
			wantLast:  "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mcpMessagesToLLM(tt.messages, tt.systemPrompt)
			if len(result) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(result), tt.wantLen)
			}
			if len(result) > 0 && result[0].Role != tt.wantFirst {
				t.Errorf("first role = %q, want %q", result[0].Role, tt.wantFirst)
			}
			if len(result) > 0 && result[len(result)-1].Role != tt.wantLast {
				t.Errorf("last role = %q, want %q", result[len(result)-1].Role, tt.wantLast)
			}
		})
	}
}

func TestLLMResponseToMCP(t *testing.T) {
	resp := &llm.ChatResponse{
		Content: "Hello from the model",
	}

	result := llmResponseToMCP(resp, "test-model-v1")

	if result.Role != "assistant" {
		t.Errorf("role = %q, want %q", result.Role, "assistant")
	}
	if result.Content.Type != "text" {
		t.Errorf("content type = %q, want %q", result.Content.Type, "text")
	}
	if result.Content.Text != "Hello from the model" {
		t.Errorf("content text = %q, want %q", result.Content.Text, "Hello from the model")
	}
	if result.Model != "test-model-v1" {
		t.Errorf("model = %q, want %q", result.Model, "test-model-v1")
	}
}

func TestSamplingHandlerRateLimit(t *testing.T) {
	h := NewSamplingHandler(nil) // nil client -- requests will fail, but we can test rate limits
	h.maxRPM = 3

	for i := 0; i < 3; i++ {
		if err := h.checkRateLimit("test-server"); err != nil {
			t.Fatalf("call %d should succeed: %v", i, err)
		}
	}

	// 4th call should be rejected.
	if err := h.checkRateLimit("test-server"); err == nil {
		t.Error("4th call should be rate-limited")
	}

	// Different server should still work.
	if err := h.checkRateLimit("other-server"); err != nil {
		t.Errorf("different server should not be rate-limited: %v", err)
	}
}

func TestSamplingHandlerRateLimitReset(t *testing.T) {
	h := NewSamplingHandler(nil)
	h.maxRPM = 1

	if err := h.checkRateLimit("srv"); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	if err := h.checkRateLimit("srv"); err == nil {
		t.Fatal("second call should be rate-limited")
	}

	// Simulate minute elapsed.
	h.mu.Lock()
	h.lastReset = time.Now().Add(-2 * time.Minute)
	h.mu.Unlock()

	if err := h.checkRateLimit("srv"); err != nil {
		t.Fatalf("call after reset should succeed: %v", err)
	}
}

func TestSamplingHandlerLoopDepth(t *testing.T) {
	h := NewSamplingHandler(nil)
	h.maxLoopDepth = 2

	// Push twice should succeed.
	if err := h.pushDepth("srv"); err != nil {
		t.Fatalf("push 1 should succeed: %v", err)
	}
	if err := h.pushDepth("srv"); err != nil {
		t.Fatalf("push 2 should succeed: %v", err)
	}

	// Third push should fail.
	if err := h.pushDepth("srv"); err == nil {
		t.Error("push 3 should fail (max depth reached)")
	}

	// Pop and push again should succeed.
	h.popDepth("srv")
	if err := h.pushDepth("srv"); err != nil {
		t.Fatalf("push after pop should succeed: %v", err)
	}

	// Different server should be independent.
	if err := h.pushDepth("other"); err != nil {
		t.Fatalf("different server should succeed: %v", err)
	}
}

func TestSamplingHandlerNoClient(t *testing.T) {
	h := NewSamplingHandler(nil)

	params, _ := json.Marshal(mcpSamplingRequest{
		Messages: []mcpSamplingMessage{
			{Role: "user", Content: mcpSamplingContent{Type: "text", Text: "hi"}},
		},
	})

	resp := h.HandleRequest(context.Background(), "test-server", 1, params)
	if resp.Error == nil {
		t.Fatal("expected error when no LLM client is configured")
	}
	if resp.Error.Code != -32603 {
		t.Errorf("error code = %d, want -32603", resp.Error.Code)
	}
}

func TestSamplingHandlerInvalidParams(t *testing.T) {
	h := NewSamplingHandler(nil)

	resp := h.HandleRequest(context.Background(), "test", 1, json.RawMessage(`{invalid`))
	// With nil client, we get the "no LLM client" error first.
	if resp.Error == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestSamplingHandlerEmptyMessages(t *testing.T) {
	h := NewSamplingHandler(nil)

	params, _ := json.Marshal(mcpSamplingRequest{
		Messages: []mcpSamplingMessage{},
	})

	resp := h.HandleRequest(context.Background(), "test", 1, params)
	// With nil client, we get the "no LLM client" error first.
	if resp.Error == nil {
		t.Fatal("expected error")
	}
}

func TestMCPMessagesToLLMContentPreservation(t *testing.T) {
	msgs := []mcpSamplingMessage{
		{Role: "user", Content: mcpSamplingContent{Type: "text", Text: "What is 2+2?"}},
		{Role: "assistant", Content: mcpSamplingContent{Type: "text", Text: "4"}},
	}

	result := mcpMessagesToLLM(msgs, "")
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Content != "What is 2+2?" {
		t.Errorf("first message content = %q, want %q", result[0].Content, "What is 2+2?")
	}
	if result[1].Content != "4" {
		t.Errorf("second message content = %q, want %q", result[1].Content, "4")
	}
}

func TestSamplingHandlerLoopDepthPopBelowZero(t *testing.T) {
	h := NewSamplingHandler(nil)

	// Pop without push should not panic or go below zero.
	h.popDepth("srv")

	h.mu.Lock()
	depth := h.depths["srv"]
	h.mu.Unlock()

	if depth != 0 {
		t.Errorf("depth after pop below zero = %d, want 0", depth)
	}
}
