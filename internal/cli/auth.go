package cli

import (
	"os"
	"path/filepath"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// ProviderInfo describes a provider and its configuration status.
type ProviderInfo struct {
	Name       string   // Human-readable name.
	ID         string   // Machine identifier used in config.
	BaseURL    string   // Default API base URL.
	EnvKey     string   // Environment variable that holds the API key.
	Configured bool     // True if the API key is present.
	Models     []string // Well-known models for this provider.
}

// knownProviders is the ordered list of supported providers.
var knownProviders = []ProviderInfo{
	{
		Name:    "OpenRouter",
		ID:      "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		EnvKey:  "OPENROUTER_API_KEY",
		Models:  []string{"anthropic/claude-sonnet-4-20250514", "openai/gpt-4o", "google/gemini-2.5-pro", "deepseek/deepseek-chat"},
	},
	{
		Name:    "OpenAI",
		ID:      "openai",
		BaseURL: "https://api.openai.com/v1",
		EnvKey:  "OPENAI_API_KEY",
		Models:  []string{"gpt-4o", "gpt-4o-mini", "o1", "o3"},
	},
	{
		Name:    "Anthropic",
		ID:      "anthropic",
		BaseURL: "https://api.anthropic.com/v1",
		EnvKey:  "ANTHROPIC_API_KEY",
		Models:  []string{"claude-opus-4-20250514", "claude-sonnet-4-20250514", "claude-haiku-4-20250414"},
	},
	{
		Name:    "Nous Portal",
		ID:      "nous",
		BaseURL: "https://inference-api.nousresearch.com/v1",
		EnvKey:  "NOUS_API_KEY",
		Models:  []string{"hermes-3-llama-3.1-405b"},
	},
	{
		Name:    "DeepSeek",
		ID:      "deepseek",
		BaseURL: "https://api.deepseek.com/v1",
		EnvKey:  "DEEPSEEK_API_KEY",
		Models:  []string{"deepseek-chat", "deepseek-r1"},
	},
	{
		Name:    "Google AI",
		ID:      "google",
		BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		EnvKey:  "GOOGLE_API_KEY",
		Models:  []string{"gemini-2.5-pro", "gemini-2.5-flash"},
	},
}

// ResolveAuth determines the provider, base URL, and API key from multiple
// sources in priority order: config.yaml -> environment variables -> OAuth tokens.
func ResolveAuth() (provider, baseURL, apiKey string) {
	cfg := config.Load()

	// 1. Explicit config values take highest priority.
	if cfg.BaseURL != "" && cfg.APIKey != "" {
		provider = cfg.Provider
		if provider == "" {
			provider = "custom"
		}
		return provider, cfg.BaseURL, cfg.APIKey
	}

	// 2. Check env vars in priority order.
	for _, p := range knownProviders {
		if key := os.Getenv(p.EnvKey); key != "" {
			return p.ID, p.BaseURL, key
		}
	}

	// 3. Check for OAuth token files (future: ~/.hermes/oauth/*.json).
	oauthDir := filepath.Join(config.HermesHome(), "oauth")
	if entries, err := os.ReadDir(oauthDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				// Found at least one OAuth token file.
				return "oauth", "", ""
			}
		}
	}

	// 4. Fall back to config-level API key.
	if cfg.APIKey != "" {
		baseURL = cfg.BaseURL
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1"
		}
		provider = cfg.Provider
		if provider == "" {
			provider = "openrouter"
		}
		return provider, baseURL, cfg.APIKey
	}

	return "", "", ""
}

// ListProviders returns all known providers with their configuration status.
func ListProviders() []ProviderInfo {
	result := make([]ProviderInfo, len(knownProviders))
	copy(result, knownProviders)

	for i := range result {
		result[i].Configured = os.Getenv(result[i].EnvKey) != ""
	}

	return result
}

// GetProvider returns a single provider by ID.
func GetProvider(id string) *ProviderInfo {
	for i := range knownProviders {
		if knownProviders[i].ID == id {
			p := knownProviders[i]
			p.Configured = os.Getenv(p.EnvKey) != ""
			return &p
		}
	}
	return nil
}

// HasAnyProvider returns true if at least one provider API key is configured.
func HasAnyProvider() bool {
	for _, p := range knownProviders {
		if os.Getenv(p.EnvKey) != "" {
			return true
		}
	}
	cfg := config.Load()
	return cfg.APIKey != ""
}
