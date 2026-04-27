package agent

import "github.com/hermes-agent/hermes-agent-go/internal/llm"

// reasoningProviders lists providers that natively support reasoning fields.
// For all other providers, reasoning content is stripped to prevent leaks.
var reasoningProviders = map[string]bool{
	"anthropic": true,
	"bedrock":   true,
}

// StripReasoningForProvider returns a copy of messages with reasoning content
// removed to prevent cross-provider reasoning leaks during fallback.
// Reasoning fields are preserved only for providers in the allowlist.
func StripReasoningForProvider(messages []llm.Message, targetProvider string) []llm.Message {
	if reasoningProviders[targetProvider] {
		return messages
	}
	result := make([]llm.Message, len(messages))
	for i, m := range messages {
		result[i] = m
		result[i].Reasoning = ""
		result[i].ReasoningContent = ""
	}
	return result
}
