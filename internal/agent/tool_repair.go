package agent

import (
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// RepairToolCall attempts to fix a mismatched tool name.
// It tries: 1) lowercase, 2) normalized (hyphens/spaces→underscores),
// 3) closest match by string similarity (cutoff 0.7).
// Returns the repaired name and true if found, or ("", false).
func RepairToolCall(name string, validNames map[string]bool) (string, bool) {
	if validNames[name] {
		return name, true
	}

	// 1. Lowercase
	lowered := strings.ToLower(name)
	if validNames[lowered] {
		return lowered, true
	}

	// 2. Normalize: hyphens and spaces to underscores
	normalized := strings.NewReplacer("-", "_", " ", "_").Replace(lowered)
	if validNames[normalized] {
		return normalized, true
	}

	// 3. Fuzzy match using Levenshtein distance (cutoff ratio 0.7)
	bestMatch := ""
	bestDist := -1

	for valid := range validNames {
		dist := levenshteinDistance(lowered, strings.ToLower(valid))
		maxLen := len(lowered)
		if len(valid) > maxLen {
			maxLen = len(valid)
		}
		if maxLen == 0 {
			continue
		}

		// Similarity ratio = 1 - (distance / maxLen)
		// We want ratio >= 0.7, so distance <= maxLen * 0.3
		threshold := int(float64(maxLen) * 0.3)
		if dist <= threshold && (bestDist < 0 || dist < bestDist) {
			bestMatch = valid
			bestDist = dist
		}
	}

	if bestMatch != "" {
		return bestMatch, true
	}

	return "", false
}

// RepairToolCalls repairs all tool calls in a batch, returning the repaired
// list and the count of repairs made.
func RepairToolCalls(toolCalls []llm.ToolCall, validNames map[string]bool) ([]llm.ToolCall, int) {
	repaired := 0
	result := make([]llm.ToolCall, len(toolCalls))
	copy(result, toolCalls)

	for i := range result {
		if !validNames[result[i].Function.Name] {
			if fixed, ok := RepairToolCall(result[i].Function.Name, validNames); ok {
				result[i].Function.Name = fixed
				repaired++
			}
		}
	}

	return result, repaired
}

// levenshteinDistance computes the Levenshtein edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use two rows for space efficiency
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,     // deletion
				curr[j-1]+1,   // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
