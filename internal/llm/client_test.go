package llm

import (
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
	if c.anthropic == nil {
		t.Error("Expected Anthropic client to be initialized")
	}
	if c.inner != nil {
		t.Error("Expected OpenAI inner client to be nil in Anthropic mode")
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
		Model:  "test-model",
		APIKey: "test-key",
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
