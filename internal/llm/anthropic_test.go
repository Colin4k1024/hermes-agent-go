package llm

import (
	"encoding/json"
	"testing"
)

func TestAnthropicClientCreation(t *testing.T) {
	c := NewAnthropicClient("claude-opus-4-6", "https://api.anthropic.com", "test-key", "anthropic")
	if c.model != "claude-opus-4-6" {
		t.Errorf("Expected model 'claude-opus-4-6', got '%s'", c.model)
	}
	if c.apiKey != "test-key" {
		t.Errorf("Expected api key 'test-key', got '%s'", c.apiKey)
	}
	// baseURL should have /v1 stripped
	if c.baseURL != "https://api.anthropic.com" {
		t.Errorf("Expected stripped base URL, got '%s'", c.baseURL)
	}
}

func TestAnthropicClientBaseURLNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://api.anthropic.com/v1", "https://api.anthropic.com"},
		{"https://api.anthropic.com/v1/", "https://api.anthropic.com"},
		{"https://custom.api.com", "https://custom.api.com"},
		{"https://custom.api.com/", "https://custom.api.com"},
	}

	for _, tt := range tests {
		c := NewAnthropicClient("model", tt.input, "key", "")
		if c.baseURL != tt.expected {
			t.Errorf("For input '%s': expected '%s', got '%s'", tt.input, tt.expected, c.baseURL)
		}
	}
}

func TestAnthropicMessagesURL(t *testing.T) {
	c := NewAnthropicClient("model", "https://api.anthropic.com", "key", "")
	url := c.messagesURL()
	if url != "https://api.anthropic.com/v1/messages" {
		t.Errorf("Expected messages URL, got '%s'", url)
	}
}

func TestBuildAnthropicRequestBasic(t *testing.T) {
	c := NewAnthropicClient("claude-opus-4-6", "https://api.anthropic.com", "key", "")

	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: 4096,
	}

	apiReq := c.buildAnthropicRequest(req)

	// System is now an array of blocks with cache_control
	sysBlocks, ok := apiReq.System.([]anthropicSystemBlock)
	if !ok {
		t.Fatalf("Expected system to be []anthropicSystemBlock, got %T", apiReq.System)
	}
	if len(sysBlocks) != 1 || sysBlocks[0].Text != "You are helpful." {
		t.Errorf("Expected system text 'You are helpful.', got %+v", sysBlocks)
	}
	if sysBlocks[0].CacheControl == nil || sysBlocks[0].CacheControl.Type != "ephemeral" {
		t.Error("Expected cache_control ephemeral on system block")
	}
	if apiReq.MaxTokens != 4096 {
		t.Errorf("Expected max_tokens 4096, got %d", apiReq.MaxTokens)
	}
	if len(apiReq.Messages) == 0 {
		t.Error("Expected messages")
	}
	// First message should be user
	if apiReq.Messages[0].Role != "user" {
		t.Errorf("Expected first message role 'user', got '%s'", apiReq.Messages[0].Role)
	}
}

func TestBuildAnthropicRequestToolResults(t *testing.T) {
	c := NewAnthropicClient("model", "https://api.anthropic.com", "key", "")

	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "System."},
			{Role: "user", Content: "Do something."},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{
					{ID: "tc_1", Type: "function", Function: FunctionCall{Name: "terminal", Arguments: `{"command":"ls"}`}},
				},
			},
			{Role: "tool", Content: `{"stdout":"file.txt"}`, ToolCallID: "tc_1"},
		},
	}

	apiReq := c.buildAnthropicRequest(req)

	// Should have system extracted as cached block
	sysBlocks, ok := apiReq.System.([]anthropicSystemBlock)
	if !ok {
		t.Fatalf("Expected system to be []anthropicSystemBlock, got %T", apiReq.System)
	}
	if len(sysBlocks) != 1 || sysBlocks[0].Text != "System." {
		t.Errorf("Expected system text 'System.', got %+v", sysBlocks)
	}

	// Messages should have user, assistant (with tool_use), user (with tool_result)
	if len(apiReq.Messages) < 3 {
		t.Errorf("Expected at least 3 messages, got %d", len(apiReq.Messages))
	}
}

func TestConvertResponse(t *testing.T) {
	c := NewAnthropicClient("model", "https://api.anthropic.com", "key", "")

	resp := &anthropicResponse{
		StopReason: "end_turn",
		Content: []anthropicContentBlock{
			{Type: "text", Text: "Hello!"},
		},
		Usage: anthropicUsage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	result := c.convertResponse(resp)
	if result.Content != "Hello!" {
		t.Errorf("Expected 'Hello!', got '%s'", result.Content)
	}
	if result.FinishReason != "stop" {
		t.Errorf("Expected 'stop', got '%s'", result.FinishReason)
	}
	if result.Usage.PromptTokens != 100 {
		t.Errorf("Expected 100 prompt tokens, got %d", result.Usage.PromptTokens)
	}
}

func TestConvertResponseToolUse(t *testing.T) {
	c := NewAnthropicClient("model", "https://api.anthropic.com", "key", "")

	resp := &anthropicResponse{
		StopReason: "tool_use",
		Content: []anthropicContentBlock{
			{Type: "text", Text: "Let me check."},
			{Type: "tool_use", ID: "tc_1", Name: "terminal", Input: map[string]any{"command": "ls"}},
		},
		Usage: anthropicUsage{InputTokens: 200, OutputTokens: 100},
	}

	result := c.convertResponse(resp)
	if result.FinishReason != "tool_calls" {
		t.Errorf("Expected 'tool_calls', got '%s'", result.FinishReason)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Function.Name != "terminal" {
		t.Errorf("Expected tool name 'terminal', got '%s'", result.ToolCalls[0].Function.Name)
	}

	// Arguments should be valid JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(result.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Errorf("Tool arguments not valid JSON: %v", err)
	}
}

func TestEnsureAlternating(t *testing.T) {
	// Two consecutive user messages
	msgs := []anthropicMessage{
		{Role: "user", Content: "First"},
		{Role: "user", Content: "Second"},
	}
	result := ensureAlternating(msgs)
	if len(result) != 3 {
		t.Errorf("Expected 3 messages (inserted assistant), got %d", len(result))
	}
	if result[1].Role != "assistant" {
		t.Errorf("Expected inserted assistant, got '%s'", result[1].Role)
	}

	// First message is assistant
	msgs = []anthropicMessage{
		{Role: "assistant", Content: "Hi"},
	}
	result = ensureAlternating(msgs)
	if result[0].Role != "user" {
		t.Errorf("Expected user prepended, got '%s'", result[0].Role)
	}
}
