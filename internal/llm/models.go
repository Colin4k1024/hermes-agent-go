package llm

import "strings"

// ModelMeta holds metadata for a known model.
type ModelMeta struct {
	ContextLength int
	MaxOutput     int
	SupportsTools bool
	SupportsVision bool
}

// KnownModels maps model identifiers to their metadata.
var KnownModels = map[string]ModelMeta{
	"anthropic/claude-opus-4-20250514":   {ContextLength: 200000, MaxOutput: 32000, SupportsTools: true, SupportsVision: true},
	"anthropic/claude-sonnet-4-20250514": {ContextLength: 200000, MaxOutput: 16000, SupportsTools: true, SupportsVision: true},
	"anthropic/claude-haiku-4-20250414":  {ContextLength: 200000, MaxOutput: 8192, SupportsTools: true, SupportsVision: true},
	"openai/gpt-4o":                      {ContextLength: 128000, MaxOutput: 16384, SupportsTools: true, SupportsVision: true},
	"openai/gpt-4o-mini":                 {ContextLength: 128000, MaxOutput: 16384, SupportsTools: true, SupportsVision: true},
	"openai/o1":                          {ContextLength: 200000, MaxOutput: 100000, SupportsTools: true, SupportsVision: true},
	"openai/o3":                          {ContextLength: 200000, MaxOutput: 100000, SupportsTools: true, SupportsVision: true},
	"google/gemini-2.5-pro":              {ContextLength: 1048576, MaxOutput: 65536, SupportsTools: true, SupportsVision: true},
	"google/gemini-2.5-flash":            {ContextLength: 1048576, MaxOutput: 65536, SupportsTools: true, SupportsVision: true},
	"deepseek/deepseek-chat":             {ContextLength: 65536, MaxOutput: 8192, SupportsTools: true, SupportsVision: false},
	"deepseek/deepseek-r1":               {ContextLength: 65536, MaxOutput: 8192, SupportsTools: true, SupportsVision: false},
	"meta-llama/llama-4-maverick":        {ContextLength: 1048576, MaxOutput: 32768, SupportsTools: true, SupportsVision: true},
}

// GetModelMeta returns metadata for a model, with fallback defaults.
func GetModelMeta(model string) ModelMeta {
	if meta, ok := KnownModels[model]; ok {
		return meta
	}

	// Try partial match
	lower := strings.ToLower(model)
	for key, meta := range KnownModels {
		if strings.Contains(lower, strings.ToLower(key)) {
			return meta
		}
	}

	// Default for unknown models
	return ModelMeta{
		ContextLength:  128000,
		MaxOutput:      8192,
		SupportsTools:  true,
		SupportsVision: false,
	}
}

// EstimateTokens gives a rough token count for a string.
// Uses the ~4 chars per token heuristic.
func EstimateTokens(text string) int {
	return len(text) / 4
}
