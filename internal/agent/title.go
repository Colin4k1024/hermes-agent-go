package agent

import (
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// GenerateSessionTitle derives a short title from the first user message.
// It does not call the LLM — it uses a fast heuristic so that title
// generation never adds latency.
func GenerateSessionTitle(messages []llm.Message) string {
	// Find the first user message.
	var firstUser string
	for _, m := range messages {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			firstUser = strings.TrimSpace(m.Content)
			break
		}
	}

	if firstUser == "" {
		return "Untitled session"
	}

	// Use the first line, capped at 80 runes.
	title := firstUser
	if idx := strings.IndexAny(title, "\n\r"); idx > 0 {
		title = title[:idx]
	}
	title = strings.TrimSpace(title)

	if utf8.RuneCountInString(title) > 80 {
		runes := []rune(title)
		title = string(runes[:77]) + "..."
	}

	return title
}

// autoGenerateTitle generates and persists a session title on the first
// conversation turn.  Call this after the first user message has been
// appended.
func (a *AIAgent) autoGenerateTitle(messages []llm.Message) {
	if a.sessionDB == nil {
		return
	}

	// Only set title if it is currently empty.
	existing := a.sessionDB.GetSessionTitle(a.sessionID)
	if existing != "" {
		return
	}

	title := GenerateSessionTitle(messages)
	if err := a.sessionDB.SetSessionTitle(a.sessionID, title); err != nil {
		slog.Warn("Failed to set session title", "error", err)
	}
}
