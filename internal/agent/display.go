package agent

import (
	"fmt"
	"strings"
	"time"
)

// FormatDuration formats a duration into a human-readable string.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// FormatTokenCount formats a token count with K/M suffix.
func FormatTokenCount(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1000000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.2fM", float64(tokens)/1000000)
}

// FormatToolTrace formats a list of tool calls for display.
func FormatToolTrace(toolCalls []string) string {
	if len(toolCalls) == 0 {
		return "(no tools used)"
	}

	// Count occurrences
	counts := make(map[string]int)
	for _, t := range toolCalls {
		counts[t]++
	}

	var parts []string
	for tool, count := range counts {
		if count == 1 {
			parts = append(parts, tool)
		} else {
			parts = append(parts, fmt.Sprintf("%s x%d", tool, count))
		}
	}

	return strings.Join(parts, " → ")
}

// FormatConversationSummary creates a summary line for a conversation result.
func FormatConversationSummary(result *ConversationResult) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("model=%s", result.Model))
	parts = append(parts, fmt.Sprintf("calls=%d", result.APICalls))
	parts = append(parts, fmt.Sprintf("tokens=%s", FormatTokenCount(result.TotalTokens)))

	if result.EstimatedCostUSD > 0 {
		parts = append(parts, fmt.Sprintf("cost=%s", FormatCost(result.EstimatedCostUSD)))
	}

	if result.Interrupted {
		parts = append(parts, "interrupted")
	} else if result.Completed {
		parts = append(parts, "completed")
	} else {
		parts = append(parts, "partial")
	}

	return strings.Join(parts, " | ")
}
