package agent

import (
	"log/slog"
	"sort"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/state"
)

// GetUsageInsights aggregates usage statistics from the session database.
// Returns a map of insight metrics over the given number of days.
func GetUsageInsights(sessionDB *state.SessionDB, days int) map[string]any {
	if sessionDB == nil {
		return map[string]any{
			"error": "session database not available",
		}
	}

	if days <= 0 {
		days = 7
	}

	// Fetch recent sessions.
	sessions, err := sessionDB.ListSessions("", 1000, 0)
	if err != nil {
		slog.Warn("Failed to fetch sessions for insights", "error", err)
		return map[string]any{
			"error": "failed to fetch sessions",
		}
	}

	cutoffTime := time.Now().AddDate(0, 0, -days)
	cutoffEpoch := float64(cutoffTime.UnixMilli()) / 1000.0

	// Filter to sessions within the time window.
	var filtered []map[string]any
	for _, sess := range sessions {
		startedAt, ok := sess["started_at"].(float64)
		if !ok {
			continue
		}
		if startedAt >= cutoffEpoch {
			filtered = append(filtered, sess)
		}
	}

	// Aggregate metrics.
	totalSessions := len(filtered)
	var totalInputTokens, totalOutputTokens int64
	modelCounts := make(map[string]int)
	dailyCounts := make(map[string]int)
	var totalMessages int64

	for _, sess := range filtered {
		// Token counts.
		if v, ok := sess["input_tokens"].(int64); ok {
			totalInputTokens += v
		}
		if v, ok := sess["output_tokens"].(int64); ok {
			totalOutputTokens += v
		}

		// Message count.
		if v, ok := sess["message_count"].(int64); ok {
			totalMessages += v
		}

		// Model usage.
		if model, ok := sess["model"].(string); ok && model != "" {
			modelCounts[model]++
		}

		// Daily breakdown.
		if startedAt, ok := sess["started_at"].(float64); ok {
			t := time.Unix(int64(startedAt), 0)
			day := t.Format("2006-01-02")
			dailyCounts[day]++
		}
	}

	totalTokens := totalInputTokens + totalOutputTokens

	// Estimate cost (rough).
	totalCost := EstimateCost("anthropic/claude-sonnet-4-20250514", int(totalInputTokens), int(totalOutputTokens))

	// Top models sorted by usage.
	type modelEntry struct {
		Model string `json:"model"`
		Count int    `json:"count"`
	}
	var topModels []modelEntry
	for model, count := range modelCounts {
		topModels = append(topModels, modelEntry{Model: model, Count: count})
	}
	sort.Slice(topModels, func(i, j int) bool {
		return topModels[i].Count > topModels[j].Count
	})
	if len(topModels) > 10 {
		topModels = topModels[:10]
	}

	// Convert to map for JSON.
	topModelsMap := make([]map[string]any, len(topModels))
	for i, m := range topModels {
		topModelsMap[i] = map[string]any{
			"model": m.Model,
			"count": m.Count,
		}
	}

	// Sessions per day sorted by date.
	type dayEntry struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	var sessionsPerDay []dayEntry
	for day, count := range dailyCounts {
		sessionsPerDay = append(sessionsPerDay, dayEntry{Date: day, Count: count})
	}
	sort.Slice(sessionsPerDay, func(i, j int) bool {
		return sessionsPerDay[i].Date < sessionsPerDay[j].Date
	})

	sessionsPerDayMap := make([]map[string]any, len(sessionsPerDay))
	for i, d := range sessionsPerDay {
		sessionsPerDayMap[i] = map[string]any{
			"date":  d.Date,
			"count": d.Count,
		}
	}

	// Average tokens per session.
	avgTokensPerSession := int64(0)
	if totalSessions > 0 {
		avgTokensPerSession = totalTokens / int64(totalSessions)
	}

	return map[string]any{
		"days":                   days,
		"total_sessions":         totalSessions,
		"total_messages":         totalMessages,
		"total_input_tokens":     totalInputTokens,
		"total_output_tokens":    totalOutputTokens,
		"total_tokens":           totalTokens,
		"avg_tokens_per_session": avgTokensPerSession,
		"estimated_cost_usd":     totalCost,
		"top_models":             topModelsMap,
		"sessions_per_day":       sessionsPerDayMap,
	}
}
