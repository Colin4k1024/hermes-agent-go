package cli

import (
	"strings"
)

// ModelInfo describes a model in the catalog.
type ModelInfo struct {
	Name          string  // Full model identifier, e.g. "anthropic/claude-sonnet-4-20250514"
	ShortName     string  // Short alias, e.g. "sonnet"
	Provider      string  // Provider name, e.g. "anthropic"
	ContextLength int     // Maximum context window in tokens
	MaxOutput     int     // Maximum output tokens
	InputPrice    float64 // USD per million input tokens
	OutputPrice   float64 // USD per million output tokens
	SupportsTools bool    // Whether the model supports tool/function calling
	Vision        bool    // Whether the model supports vision/image input
	Reasoning     bool    // Whether the model supports extended thinking / chain-of-thought
}

// ModelCatalog is the built-in list of well-known models.
var ModelCatalog = []ModelInfo{
	// --- Anthropic ---
	{
		Name: "anthropic/claude-opus-4-20250514", ShortName: "opus",
		Provider: "anthropic", ContextLength: 200000, MaxOutput: 32000,
		InputPrice: 15.0, OutputPrice: 75.0,
		SupportsTools: true, Vision: true, Reasoning: true,
	},
	{
		Name: "anthropic/claude-sonnet-4-20250514", ShortName: "sonnet",
		Provider: "anthropic", ContextLength: 200000, MaxOutput: 16000,
		InputPrice: 3.0, OutputPrice: 15.0,
		SupportsTools: true, Vision: true, Reasoning: true,
	},
	{
		Name: "anthropic/claude-haiku-4-20250414", ShortName: "haiku",
		Provider: "anthropic", ContextLength: 200000, MaxOutput: 8192,
		InputPrice: 0.80, OutputPrice: 4.0,
		SupportsTools: true, Vision: true, Reasoning: false,
	},

	// --- OpenAI ---
	{
		Name: "openai/gpt-4o", ShortName: "gpt4o",
		Provider: "openai", ContextLength: 128000, MaxOutput: 16384,
		InputPrice: 2.50, OutputPrice: 10.0,
		SupportsTools: true, Vision: true, Reasoning: false,
	},
	{
		Name: "openai/gpt-4o-mini", ShortName: "gpt4o-mini",
		Provider: "openai", ContextLength: 128000, MaxOutput: 16384,
		InputPrice: 0.15, OutputPrice: 0.60,
		SupportsTools: true, Vision: true, Reasoning: false,
	},
	{
		Name: "openai/o1", ShortName: "o1",
		Provider: "openai", ContextLength: 200000, MaxOutput: 100000,
		InputPrice: 15.0, OutputPrice: 60.0,
		SupportsTools: true, Vision: true, Reasoning: true,
	},
	{
		Name: "openai/o3", ShortName: "o3",
		Provider: "openai", ContextLength: 200000, MaxOutput: 100000,
		InputPrice: 10.0, OutputPrice: 40.0,
		SupportsTools: true, Vision: true, Reasoning: true,
	},

	// --- Google ---
	{
		Name: "google/gemini-2.5-pro", ShortName: "gemini-pro",
		Provider: "google", ContextLength: 1048576, MaxOutput: 65536,
		InputPrice: 1.25, OutputPrice: 10.0,
		SupportsTools: true, Vision: true, Reasoning: true,
	},
	{
		Name: "google/gemini-2.5-flash", ShortName: "gemini-flash",
		Provider: "google", ContextLength: 1048576, MaxOutput: 65536,
		InputPrice: 0.15, OutputPrice: 0.60,
		SupportsTools: true, Vision: true, Reasoning: false,
	},

	// --- DeepSeek ---
	{
		Name: "deepseek/deepseek-chat", ShortName: "deepseek",
		Provider: "deepseek", ContextLength: 65536, MaxOutput: 8192,
		InputPrice: 0.27, OutputPrice: 1.10,
		SupportsTools: true, Vision: false, Reasoning: false,
	},
	{
		Name: "deepseek/deepseek-r1", ShortName: "deepseek-r1",
		Provider: "deepseek", ContextLength: 65536, MaxOutput: 8192,
		InputPrice: 0.55, OutputPrice: 2.19,
		SupportsTools: true, Vision: false, Reasoning: true,
	},

	// --- Meta ---
	{
		Name: "meta-llama/llama-4-maverick", ShortName: "maverick",
		Provider: "meta", ContextLength: 1048576, MaxOutput: 32768,
		InputPrice: 0.20, OutputPrice: 0.60,
		SupportsTools: true, Vision: true, Reasoning: false,
	},

	// --- Nous ---
	{
		Name: "nousresearch/hermes-3-llama-3.1-405b", ShortName: "hermes-405b",
		Provider: "nous", ContextLength: 131072, MaxOutput: 16384,
		InputPrice: 2.0, OutputPrice: 6.0,
		SupportsTools: true, Vision: false, Reasoning: false,
	},

	// --- Mistral ---
	{
		Name: "mistralai/mistral-large-latest", ShortName: "mistral-large",
		Provider: "mistral", ContextLength: 131072, MaxOutput: 8192,
		InputPrice: 2.0, OutputPrice: 6.0,
		SupportsTools: true, Vision: false, Reasoning: false,
	},

	// --- Qwen ---
	{
		Name: "qwen/qwen-2.5-72b-instruct", ShortName: "qwen-72b",
		Provider: "qwen", ContextLength: 131072, MaxOutput: 8192,
		InputPrice: 0.35, OutputPrice: 0.40,
		SupportsTools: true, Vision: false, Reasoning: false,
	},
}

// SearchModels searches the catalog by name, short name, or provider.
// The query is case-insensitive and matched as a substring.
func SearchModels(query string) []ModelInfo {
	q := strings.ToLower(query)
	var results []ModelInfo
	for _, m := range ModelCatalog {
		if strings.Contains(strings.ToLower(m.Name), q) ||
			strings.Contains(strings.ToLower(m.ShortName), q) ||
			strings.Contains(strings.ToLower(m.Provider), q) {
			results = append(results, m)
		}
	}
	return results
}

// GetModelByShortName returns a model by its short alias.
// Returns nil if not found.
func GetModelByShortName(shortName string) *ModelInfo {
	lower := strings.ToLower(shortName)
	for i := range ModelCatalog {
		if strings.ToLower(ModelCatalog[i].ShortName) == lower {
			return &ModelCatalog[i]
		}
	}
	return nil
}

// GetModelByName returns a model by its full name.
// Returns nil if not found.
func GetModelByName(name string) *ModelInfo {
	lower := strings.ToLower(name)
	for i := range ModelCatalog {
		if strings.ToLower(ModelCatalog[i].Name) == lower {
			return &ModelCatalog[i]
		}
	}
	return nil
}

// ListModelsByProvider returns all models for a given provider.
func ListModelsByProvider(provider string) []ModelInfo {
	lower := strings.ToLower(provider)
	var results []ModelInfo
	for _, m := range ModelCatalog {
		if strings.ToLower(m.Provider) == lower {
			results = append(results, m)
		}
	}
	return results
}

// ListProviders returns the unique set of providers in the catalog.
func ListCatalogProviders() []string {
	seen := make(map[string]bool)
	var providers []string
	for _, m := range ModelCatalog {
		if !seen[m.Provider] {
			seen[m.Provider] = true
			providers = append(providers, m.Provider)
		}
	}
	return providers
}

// ResolveModelName resolves a short name or partial match to a full model name.
// Tries: exact full name -> exact short name -> substring match (returns first hit).
// Returns empty string if nothing matched.
func ResolveModelName(input string) string {
	// Exact full name.
	if m := GetModelByName(input); m != nil {
		return m.Name
	}
	// Exact short name.
	if m := GetModelByShortName(input); m != nil {
		return m.Name
	}
	// Substring search.
	matches := SearchModels(input)
	if len(matches) > 0 {
		return matches[0].Name
	}
	// Not found — return input as-is (may be a custom model).
	return input
}
