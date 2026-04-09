package agent

import "strings"

// ModelAliases maps short model names to their full canonical identifiers.
var ModelAliases = map[string]string{
	// Anthropic
	"claude-opus":       "anthropic/claude-opus-4-20250514",
	"opus":              "anthropic/claude-opus-4-20250514",
	"claude-sonnet":     "anthropic/claude-sonnet-4-20250514",
	"sonnet":            "anthropic/claude-sonnet-4-20250514",
	"claude-haiku":      "anthropic/claude-haiku-4-20250414",
	"haiku":             "anthropic/claude-haiku-4-20250414",

	// OpenAI
	"gpt4o":             "openai/gpt-4o",
	"gpt-4o":            "openai/gpt-4o",
	"gpt4o-mini":        "openai/gpt-4o-mini",
	"gpt-4o-mini":       "openai/gpt-4o-mini",
	"o1":                "openai/o1",
	"o3":                "openai/o3",

	// Google
	"gemini-pro":        "google/gemini-2.5-pro",
	"gemini-2.5-pro":    "google/gemini-2.5-pro",
	"gemini-flash":      "google/gemini-2.5-flash",
	"gemini-2.5-flash":  "google/gemini-2.5-flash",

	// DeepSeek
	"deepseek":          "deepseek/deepseek-chat",
	"deepseek-chat":     "deepseek/deepseek-chat",
	"deepseek-r1":       "deepseek/deepseek-r1",

	// Meta
	"llama-4-maverick":  "meta-llama/llama-4-maverick",
	"maverick":          "meta-llama/llama-4-maverick",
}

// NormalizeModelName converts various model name formats to canonical form.
// If the input is already a full identifier (contains "/"), it is returned as-is.
// Otherwise, the input is looked up in the alias table.
//
// Examples:
//
//	"claude-opus"  -> "anthropic/claude-opus-4-20250514"
//	"gpt4o"        -> "openai/gpt-4o"
//	"sonnet"       -> "anthropic/claude-sonnet-4-20250514"
//	"openai/gpt-4o" -> "openai/gpt-4o" (unchanged)
func NormalizeModelName(input string) string {
	if input == "" {
		return input
	}

	// If it already has a provider prefix, return as-is.
	if strings.Contains(input, "/") {
		return input
	}

	// Try exact match in alias table.
	lower := strings.ToLower(input)
	if canonical, ok := ModelAliases[lower]; ok {
		return canonical
	}

	// Try partial match: check if any alias key is contained in the input
	// or the input is contained in an alias key.
	for alias, canonical := range ModelAliases {
		if strings.Contains(lower, alias) || strings.Contains(alias, lower) {
			return canonical
		}
	}

	// Return as-is if no match found.
	return input
}

// IsKnownModel returns true if the model name (or alias) resolves to a known model.
func IsKnownModel(input string) bool {
	normalized := NormalizeModelName(input)
	// Check against the known models list in llm package would create a circular
	// dependency, so we check the alias table instead.
	for _, canonical := range ModelAliases {
		if canonical == normalized {
			return true
		}
	}
	return false
}

// ListModelAliases returns all available model aliases grouped by provider.
func ListModelAliases() map[string][]string {
	groups := make(map[string][]string)
	seen := make(map[string]bool)

	for alias, canonical := range ModelAliases {
		// Extract provider from canonical name.
		provider := "other"
		if idx := strings.IndexByte(canonical, '/'); idx >= 0 {
			provider = canonical[:idx]
		}

		key := provider + ":" + alias
		if !seen[key] {
			seen[key] = true
			groups[provider] = append(groups[provider], alias)
		}
	}

	return groups
}
