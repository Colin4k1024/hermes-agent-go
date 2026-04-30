package llm

import (
	"strings"
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

func TestNewClientWithParams(t *testing.T) {
	c, err := NewClientWithParams("gpt-4", "https://api.example.com/v1", "test-key", "custom")
	if err != nil {
		t.Fatalf("NewClientWithParams failed: %v", err)
	}
	if c.Model() != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", c.Model())
	}
	if c.Provider() != "custom" {
		t.Errorf("Expected provider 'custom', got '%s'", c.Provider())
	}
	if c.BaseURL() != "https://api.example.com/v1" {
		t.Errorf("Expected custom base URL, got '%s'", c.BaseURL())
	}
	if c.APIMode() != APIModeOpenAI {
		t.Errorf("Expected OpenAI mode, got '%s'", c.APIMode())
	}
}

func TestNewClientWithModeAnthropic(t *testing.T) {
	c, err := NewClientWithMode("claude-opus-4-6", "https://api.anthropic.com", "test-key", "anthropic", APIModeAnthropic)
	if err != nil {
		t.Fatalf("NewClientWithMode failed: %v", err)
	}
	if c.APIMode() != APIModeAnthropic {
		t.Errorf("Expected Anthropic mode, got '%s'", c.APIMode())
	}
	if c.transport == nil {
		t.Error("Expected transport to be initialized")
	}
	if !strings.HasPrefix(c.transport.Name(), "anthropic") {
		t.Errorf("Expected anthropic transport, got '%s'", c.transport.Name())
	}
}

func TestNewClientNoKey(t *testing.T) {
	_, err := NewClientWithParams("gpt-4", "https://api.example.com", "", "custom")
	if err == nil {
		t.Error("Expected error with empty API key")
	}
}

func TestNewClientFromConfig(t *testing.T) {
	cfg := &config.Config{
		Model:   "test-model",
		APIKey:  "test-key",
		BaseURL: "https://test.com/v1",
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if c.Model() != "test-model" {
		t.Errorf("Expected 'test-model', got '%s'", c.Model())
	}
}

func TestParseToolArgs(t *testing.T) {
	args, err := ParseToolArgs(`{"command":"echo hello","timeout":30}`)
	if err != nil {
		t.Fatalf("ParseToolArgs failed: %v", err)
	}
	if args["command"] != "echo hello" {
		t.Errorf("Expected 'echo hello', got %v", args["command"])
	}
	if args["timeout"] != float64(30) {
		t.Errorf("Expected 30, got %v", args["timeout"])
	}
}

func TestParseToolArgsInvalid(t *testing.T) {
	_, err := ParseToolArgs("not json")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestParseToolArgsEmpty(t *testing.T) {
	args, err := ParseToolArgs(`{}`)
	if err != nil {
		t.Fatalf("ParseToolArgs failed: %v", err)
	}
	if len(args) != 0 {
		t.Errorf("Expected empty map, got %v", args)
	}
}

func TestParseToolArgs_NestedJSON(t *testing.T) {
	args, err := ParseToolArgs(`{"outer":{"inner":"value"},"list":[1,2,3]}`)
	if err != nil {
		t.Fatalf("ParseToolArgs failed: %v", err)
	}
	outer, ok := args["outer"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'outer' to be a map")
	}
	if outer["inner"] != "value" {
		t.Errorf("Expected inner='value', got %v", outer["inner"])
	}
	list, ok := args["list"].([]any)
	if !ok {
		t.Fatal("Expected 'list' to be a slice")
	}
	if len(list) != 3 {
		t.Errorf("Expected 3 items, got %d", len(list))
	}
}

func TestClient_Model(t *testing.T) {
	c, err := NewClientWithParams("test-model", "https://api.example.com/v1", "key", "custom")
	if err != nil {
		t.Fatalf("NewClientWithParams failed: %v", err)
	}
	if c.Model() != "test-model" {
		t.Errorf("Expected 'test-model', got '%s'", c.Model())
	}
}

func TestClient_Provider(t *testing.T) {
	c, err := NewClientWithParams("model", "https://api.example.com/v1", "key", "my-provider")
	if err != nil {
		t.Fatalf("NewClientWithParams failed: %v", err)
	}
	if c.Provider() != "my-provider" {
		t.Errorf("Expected 'my-provider', got '%s'", c.Provider())
	}
}

func TestClient_BaseURL(t *testing.T) {
	c, err := NewClientWithParams("model", "https://api.example.com/v1", "key", "custom")
	if err != nil {
		t.Fatalf("NewClientWithParams failed: %v", err)
	}
	if c.BaseURL() != "https://api.example.com/v1" {
		t.Errorf("Expected 'https://api.example.com/v1', got '%s'", c.BaseURL())
	}
}

func TestClient_APIMode(t *testing.T) {
	c, err := NewClientWithMode("model", "https://api.anthropic.com", "key", "anthropic", APIModeAnthropic)
	if err != nil {
		t.Fatalf("NewClientWithMode failed: %v", err)
	}
	if c.APIMode() != APIModeAnthropic {
		t.Errorf("Expected APIModeAnthropic, got '%s'", c.APIMode())
	}
}

func TestDetectAPIMode_Explicit(t *testing.T) {
	tests := []struct {
		explicit string
		expected APIMode
	}{
		{"anthropic", APIModeAnthropic},
		{"anthropic_messages", APIModeAnthropic},
		{"openai", APIModeOpenAI},
		{"chat_completions", APIModeOpenAI},
	}

	for _, tt := range tests {
		result := detectAPIMode(tt.explicit, "", "")
		if result != tt.expected {
			t.Errorf("detectAPIMode(%q) = %s, want %s", tt.explicit, result, tt.expected)
		}
	}
}

func TestDetectAPIMode_ByProvider(t *testing.T) {
	result := detectAPIMode("", "anthropic", "")
	if result != APIModeAnthropic {
		t.Errorf("Expected APIModeAnthropic for provider 'anthropic', got %s", result)
	}

	result = detectAPIMode("", "openai", "")
	if result != APIModeOpenAI {
		t.Errorf("Expected APIModeOpenAI for provider 'openai', got %s", result)
	}
}

func TestDetectAPIMode_ByURL(t *testing.T) {
	result := detectAPIMode("", "", "https://api.anthropic.com/v1")
	if result != APIModeAnthropic {
		t.Errorf("Expected APIModeAnthropic for anthropic URL, got %s", result)
	}

	result = detectAPIMode("", "", "https://api.openai.com/v1")
	if result != APIModeOpenAI {
		t.Errorf("Expected APIModeOpenAI for openai URL, got %s", result)
	}
}

func TestNewClientWithMode_OpenAI(t *testing.T) {
	c, err := NewClientWithMode("gpt-4", "https://api.openai.com/v1", "key", "openai", APIModeOpenAI)
	if err != nil {
		t.Fatalf("NewClientWithMode failed: %v", err)
	}
	if c.transport == nil {
		t.Error("Expected transport to be initialized")
	}
	if !strings.HasPrefix(c.transport.Name(), "openai") {
		t.Errorf("Expected openai transport, got '%s'", c.transport.Name())
	}
}

func TestNewClientWithMode_NoKey(t *testing.T) {
	_, err := NewClientWithMode("gpt-4", "https://api.openai.com/v1", "", "openai", APIModeOpenAI)
	if err == nil {
		t.Error("Expected error with empty API key")
	}
}

func TestMessage_Fields(t *testing.T) {
	msg := Message{
		Role:         "assistant",
		Content:      "Hello",
		Reasoning:    "thinking...",
		FinishReason: "stop",
	}

	if msg.Role != "assistant" {
		t.Error("Expected role 'assistant'")
	}
	if msg.Content != "Hello" {
		t.Error("Expected content 'Hello'")
	}
	if msg.Reasoning != "thinking..." {
		t.Error("Expected reasoning")
	}
}

func TestToolCall_Fields(t *testing.T) {
	tc := ToolCall{
		ID:   "tc_1",
		Type: "function",
		Function: FunctionCall{
			Name:      "test_func",
			Arguments: `{"key":"value"}`,
		},
	}

	if tc.ID != "tc_1" {
		t.Error("Expected ID 'tc_1'")
	}
	if tc.Function.Name != "test_func" {
		t.Error("Expected function name 'test_func'")
	}
}

func TestUsage_Fields(t *testing.T) {
	u := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CacheReadTokens:  30,
		CacheWriteTokens: 20,
		ReasoningTokens:  10,
	}

	if u.PromptTokens != 100 {
		t.Error("Expected 100 prompt tokens")
	}
	if u.TotalTokens != 150 {
		t.Error("Expected 150 total tokens")
	}
}

func TestBuildOpenAIRequest(t *testing.T) {
	c, _ := NewClientWithParams("gpt-4", "https://api.openai.com/v1", "key", "openai")

	temp := float32(0.7)
	req := ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{ID: "tc_1", Type: "function", Function: FunctionCall{Name: "test", Arguments: `{}`}},
				},
			},
			{Role: "tool", Content: "result", ToolCallID: "tc_1"},
		},
		MaxTokens:   4096,
		Temperature: &temp,
	}

	apiReq := BuildOpenAIRequest(c.Model(), req)
	if apiReq.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", apiReq.Model)
	}
	if apiReq.MaxTokens != 4096 {
		t.Errorf("Expected max_tokens 4096, got %d", apiReq.MaxTokens)
	}
	if apiReq.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7, got %f", apiReq.Temperature)
	}
	if len(apiReq.Messages) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(apiReq.Messages))
	}
	if apiReq.Messages[0].Role != "system" {
		t.Errorf("Expected first role 'system', got '%s'", apiReq.Messages[0].Role)
	}
	if apiReq.Messages[3].ToolCallID != "tc_1" {
		t.Errorf("Expected tool call ID, got '%s'", apiReq.Messages[3].ToolCallID)
	}
}

func TestBuildOpenAIRequest_NoOptionals(t *testing.T) {
	c, _ := NewClientWithParams("gpt-4", "https://api.openai.com/v1", "key", "openai")

	req := ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	apiReq := BuildOpenAIRequest(c.Model(), req)
	if apiReq.MaxTokens != 0 {
		t.Errorf("Expected 0 max_tokens when not set, got %d", apiReq.MaxTokens)
	}
	if apiReq.Temperature != 0 {
		t.Errorf("Expected 0 temperature when not set, got %f", apiReq.Temperature)
	}
}

func TestStreamDelta_Fields(t *testing.T) {
	delta := StreamDelta{
		Content:   "hello",
		Reasoning: "thinking",
		Done:      false,
		ToolCalls: []ToolCall{
			{ID: "tc_1", Function: FunctionCall{Name: "test"}},
		},
	}

	if delta.Content != "hello" {
		t.Error("Expected content 'hello'")
	}
	if delta.Reasoning != "thinking" {
		t.Error("Expected reasoning")
	}
	if delta.Done {
		t.Error("Expected Done false")
	}
	if len(delta.ToolCalls) != 1 {
		t.Error("Expected 1 tool call")
	}
}

func TestChatRequest_Fields(t *testing.T) {
	req := ChatRequest{
		Messages:       []Message{{Role: "user", Content: "test"}},
		MaxTokens:      8192,
		ReasoningLevel: "medium",
	}

	if len(req.Messages) != 1 {
		t.Error("Expected 1 message")
	}
	if req.MaxTokens != 8192 {
		t.Error("Expected 8192 max tokens")
	}
	if req.ReasoningLevel != "medium" {
		t.Error("Expected reasoning level 'medium'")
	}
}

func TestAPIModeConstants(t *testing.T) {
	if APIModeOpenAI != "openai" {
		t.Errorf("Expected 'openai', got '%s'", APIModeOpenAI)
	}
	if APIModeAnthropic != "anthropic" {
		t.Errorf("Expected 'anthropic', got '%s'", APIModeAnthropic)
	}
}
