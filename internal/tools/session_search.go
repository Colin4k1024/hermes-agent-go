package tools

import (
	"fmt"
	"log/slog"

	"github.com/hermes-agent/hermes-agent-go/internal/state"
)

func init() {
	Register(&ToolEntry{
		Name:    "session_search",
		Toolset: "session_search",
		Schema: map[string]any{
			"name":        "session_search",
			"description": "Search past conversation sessions using full-text search. Returns matching messages with session context.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query (supports FTS5 syntax: AND, OR, NOT, phrases in quotes)",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of results (default: 20, max: 50)",
						"default":     20,
					},
				},
				"required": []string{"query"},
			},
		},
		Handler: handleSessionSearch,
		Emoji:   "\U0001f50d",
	})
}

func handleSessionSearch(args map[string]any, ctx *ToolContext) string {
	query, _ := args["query"].(string)
	if query == "" {
		return `{"error":"query is required"}`
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	if limit > 50 {
		limit = 50
	}

	// Open the session database
	db, err := state.NewSessionDB("")
	if err != nil {
		slog.Warn("Cannot open session database for search", "error", err)
		return toJSON(map[string]any{
			"error":   "Cannot access session database",
			"details": err.Error(),
		})
	}
	defer db.Close()

	// Perform full-text search
	results, err := db.SearchMessages(query, limit)
	if err != nil {
		return toJSON(map[string]any{
			"error": fmt.Sprintf("Search failed: %v", err),
			"hint":  "FTS5 may not be available. Try a simpler query.",
		})
	}

	if results == nil {
		results = []map[string]any{}
	}

	// Truncate long content in results
	for i, r := range results {
		if content, ok := r["content"].(string); ok {
			results[i]["content"] = truncateOutput(content, 500)
		}
	}

	return toJSON(map[string]any{
		"query":   query,
		"results": results,
		"count":   len(results),
	})
}
