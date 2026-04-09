package llm

import (
	"os"
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

func TestResolveProviderOpenRouter(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "test-key-123")
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
	defer os.Unsetenv("OPENROUTER_API_KEY")

	cfg := &config.Config{}
	provider, baseURL, apiKey := ResolveProvider(cfg)
	if provider != "openrouter" {
		t.Errorf("Expected provider 'openrouter', got '%s'", provider)
	}
	if baseURL != OpenRouterBaseURL {
		t.Errorf("Expected OpenRouter base URL, got '%s'", baseURL)
	}
	if apiKey != "test-key-123" {
		t.Errorf("Expected api key 'test-key-123', got '%s'", apiKey)
	}
}

func TestResolveProviderExplicit(t *testing.T) {
	cfg := &config.Config{
		BaseURL: "https://custom.api.com/v1",
		APIKey:  "custom-key",
	}
	provider, baseURL, apiKey := ResolveProvider(cfg)
	if provider != "custom" {
		t.Errorf("Expected provider 'custom', got '%s'", provider)
	}
	if baseURL != "https://custom.api.com/v1" {
		t.Errorf("Expected custom base URL, got '%s'", baseURL)
	}
	if apiKey != "custom-key" {
		t.Errorf("Expected 'custom-key', got '%s'", apiKey)
	}
}

func TestIsOpenRouter(t *testing.T) {
	if !IsOpenRouter("https://openrouter.ai/api/v1") {
		t.Error("Expected true for OpenRouter URL")
	}
	if IsOpenRouter("https://api.openai.com/v1") {
		t.Error("Expected false for OpenAI URL")
	}
}

func TestIsAnthropic(t *testing.T) {
	if !IsAnthropic("anthropic") {
		t.Error("Expected true for anthropic provider")
	}
	if IsAnthropic("openai") {
		t.Error("Expected false for openai provider")
	}
}

func TestModelSupportsReasoning(t *testing.T) {
	reasoning := []string{
		"anthropic/claude-sonnet-4-20250514",
		"openai/o1",
		"deepseek/deepseek-r1",
	}
	for _, m := range reasoning {
		if !ModelSupportsReasoning(m) {
			t.Errorf("Expected %s to support reasoning", m)
		}
	}

	noReasoning := []string{
		"openai/gpt-4o",
		"meta-llama/llama-3-70b",
	}
	for _, m := range noReasoning {
		if ModelSupportsReasoning(m) {
			t.Errorf("Expected %s NOT to support reasoning", m)
		}
	}
}

func TestDetectAPIMode(t *testing.T) {
	if detectAPIMode("anthropic", "", "") != APIModeAnthropic {
		t.Error("Explicit 'anthropic' should return APIModeAnthropic")
	}
	if detectAPIMode("", "anthropic", "") != APIModeAnthropic {
		t.Error("Provider 'anthropic' should return APIModeAnthropic")
	}
	if detectAPIMode("", "", "https://api.anthropic.com/v1") != APIModeAnthropic {
		t.Error("Anthropic URL should return APIModeAnthropic")
	}
	if detectAPIMode("", "", "https://openrouter.ai/api/v1") != APIModeOpenAI {
		t.Error("OpenRouter URL should return APIModeOpenAI")
	}
	if detectAPIMode("", "", "") != APIModeOpenAI {
		t.Error("Default should be APIModeOpenAI")
	}
}
