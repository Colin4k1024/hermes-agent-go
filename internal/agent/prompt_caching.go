package agent

import (
	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// CacheBreakpoint represents a cache control marker position in the message list.
type CacheBreakpoint struct {
	// Index is the message index where the breakpoint should be placed.
	Index int
	// TTL is the cache time-to-live. Currently only "ephemeral" (5 min) is supported.
	TTL string
}

// CalculateBreakpoints determines optimal cache_control breakpoint positions
// for a given message count. Strategy:
//   - After system prompt (index 0) -- always
//   - After first 3 messages (index 3) -- if enough messages
//   - Every 20 messages thereafter
func CalculateBreakpoints(messageCount int) []CacheBreakpoint {
	if messageCount == 0 {
		return nil
	}

	var breakpoints []CacheBreakpoint

	// Always place a breakpoint after the system prompt (index 0).
	breakpoints = append(breakpoints, CacheBreakpoint{Index: 0, TTL: "ephemeral"})

	// Place a breakpoint after the first 3 messages if available.
	if messageCount > 3 {
		breakpoints = append(breakpoints, CacheBreakpoint{Index: 3, TTL: "ephemeral"})
	}

	// Place breakpoints every 20 messages after the initial block.
	for i := 20; i < messageCount; i += 20 {
		// Avoid duplicating the index-3 breakpoint.
		if i == 3 {
			continue
		}
		breakpoints = append(breakpoints, CacheBreakpoint{Index: i, TTL: "ephemeral"})
	}

	// Filter out any breakpoints beyond the message list.
	var valid []CacheBreakpoint
	for _, bp := range breakpoints {
		if bp.Index < messageCount {
			valid = append(valid, bp)
		}
	}

	return valid
}

// ApplyPromptCaching inserts cache_control markers at strategic breakpoints
// in the message list to maximize Anthropic's prompt caching.
//
// This works by annotating messages at breakpoint positions with a special
// sentinel in the Reasoning field (which is stripped before sending in the
// Anthropic client). The actual cache_control JSON field is injected by
// the Anthropic client layer when it detects these markers.
//
// Breakpoints: after system prompt, after first 3 messages, every 20 messages.
func ApplyPromptCaching(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return messages
	}

	breakpoints := CalculateBreakpoints(len(messages))
	if len(breakpoints) == 0 {
		return messages
	}

	// Build a set of breakpoint indices for fast lookup.
	bpSet := make(map[int]string, len(breakpoints))
	for _, bp := range breakpoints {
		bpSet[bp.Index] = bp.TTL
	}

	// Create a new slice to avoid mutating the original.
	result := make([]llm.Message, len(messages))
	copy(result, messages)

	for i := range result {
		if ttl, ok := bpSet[i]; ok {
			// Clone the message to avoid mutating the original.
			msg := result[i]
			// Mark with cache control sentinel. The Anthropic client
			// recognizes this prefix and converts it to the proper
			// cache_control block in the API request.
			if msg.ReasoningContent == "" {
				msg.ReasoningContent = cacheControlSentinel(ttl)
			}
			result[i] = msg
		}
	}

	return result
}

// cacheControlSentinel returns the sentinel string that the Anthropic client
// layer looks for to inject cache_control.
func cacheControlSentinel(ttl string) string {
	return "__cache_control:" + ttl
}

// IsCacheControlSentinel checks whether a ReasoningContent string is a
// cache control marker (not actual reasoning).
func IsCacheControlSentinel(s string) bool {
	return len(s) > 16 && s[:16] == "__cache_control:"
}

// ParseCacheControlTTL extracts the TTL from a cache control sentinel.
func ParseCacheControlTTL(s string) string {
	if !IsCacheControlSentinel(s) {
		return ""
	}
	return s[16:]
}
