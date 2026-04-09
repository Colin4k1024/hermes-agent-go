package llm

import (
	"os"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

const (
	OpenRouterBaseURL = "https://openrouter.ai/api/v1"
	NousAPIBaseURL    = "https://inference-api.nousresearch.com/v1"
	OpenAIBaseURL     = "https://api.openai.com/v1"
	AnthropicBaseURL  = "https://api.anthropic.com/v1"
)

// ResolveProvider determines the provider, base URL, and API key from config + env.
func ResolveProvider(cfg *config.Config) (provider, baseURL, apiKey string) {
	// Explicit config takes priority
	if cfg.BaseURL != "" && cfg.APIKey != "" {
		provider = cfg.Provider
		if provider == "" {
			provider = "custom"
		}
		return provider, cfg.BaseURL, cfg.APIKey
	}

	// Try to detect from model prefix
	model := cfg.Model
	if strings.Contains(model, "/") {
		parts := strings.SplitN(model, "/", 2)
		providerPrefix := parts[0]

		switch providerPrefix {
		case "openai":
			if key := os.Getenv("OPENAI_API_KEY"); key != "" {
				return "openai", OpenAIBaseURL, key
			}
		case "anthropic":
			if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
				return "anthropic", AnthropicBaseURL, key
			}
		}
	}

	// Check for API keys in priority order
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		return "openrouter", OpenRouterBaseURL, key
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return "openai", OpenAIBaseURL, key
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return "anthropic", AnthropicBaseURL, key
	}

	// Check config-level key
	if cfg.APIKey != "" {
		baseURL = cfg.BaseURL
		if baseURL == "" {
			baseURL = OpenRouterBaseURL
		}
		provider = cfg.Provider
		if provider == "" {
			provider = "openrouter"
		}
		return provider, baseURL, cfg.APIKey
	}

	return "", "", ""
}

// IsOpenRouter returns true if the provider is OpenRouter.
func IsOpenRouter(baseURL string) bool {
	return strings.Contains(baseURL, "openrouter.ai")
}

// IsAnthropic returns true if using Anthropic directly.
func IsAnthropic(provider string) bool {
	return provider == "anthropic"
}

// ModelSupportsReasoning returns true if the model supports extended thinking.
func ModelSupportsReasoning(model string) bool {
	lower := strings.ToLower(model)
	reasoningModels := []string{
		"claude-3-7", "claude-sonnet-4", "claude-opus-4",
		"o1", "o3", "o4",
		"deepseek-r1", "qwq",
	}
	for _, rm := range reasoningModels {
		if strings.Contains(lower, rm) {
			return true
		}
	}
	return false
}
