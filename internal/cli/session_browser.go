package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hermes-agent/hermes-agent-go/internal/state"
)

// SessionBrowser provides a simple session history browser.
type SessionBrowser struct {
	db   *state.SessionDB
	skin *SkinConfig
}

// NewSessionBrowser creates a new session browser.
func NewSessionBrowser(db *state.SessionDB, skin *SkinConfig) *SessionBrowser {
	return &SessionBrowser{
		db:   db,
		skin: skin,
	}
}

// BrowseSessionHistory lists past sessions with details.
// Returns a formatted string for display and the list of session IDs.
func BrowseSessionHistory(db *state.SessionDB, skin *SkinConfig, limit int) (string, []string) {
	if db == nil {
		return "Session database not available.", nil
	}

	if limit <= 0 {
		limit = 20
	}

	sessions, err := db.ListSessions("", limit, 0)
	if err != nil {
		return fmt.Sprintf("Error loading sessions: %v", err), nil
	}

	if len(sessions) == 0 {
		return "No past sessions found.", nil
	}

	// Styles.
	accentColor := "#FFBF00"
	dimColor := "#888888"
	textColor := "#FFF8DC"
	if skin != nil {
		accentColor = skin.GetColor("banner_accent", accentColor)
		dimColor = skin.GetColor("banner_dim", dimColor)
		textColor = skin.GetColor("banner_text", textColor)
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Bold(true)
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(dimColor))
	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(textColor))

	var sb strings.Builder
	sb.WriteString(headerStyle.Render("Session History"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(strings.Repeat("-", 70)))
	sb.WriteString("\n\n")

	var sessionIDs []string

	for i, sess := range sessions {
		id, _ := sess["id"].(string)
		title, _ := sess["title"].(string)
		model, _ := sess["model"].(string)
		startedAt, _ := sess["started_at"].(float64)
		msgCount, _ := sess["message_count"].(int64)
		preview, _ := sess["preview"].(string)

		sessionIDs = append(sessionIDs, id)

		// Format date.
		dateStr := "unknown"
		if startedAt > 0 {
			t := time.Unix(int64(startedAt), 0)
			dateStr = t.Format("2006-01-02 15:04")
		}

		// Format title.
		if title == "" {
			title = "Untitled"
		}
		if len(title) > 50 {
			title = title[:47] + "..."
		}

		// Format model.
		modelShort := model
		if idx := strings.LastIndex(model, "/"); idx >= 0 {
			modelShort = model[idx+1:]
		}
		if len(modelShort) > 20 {
			modelShort = modelShort[:17] + "..."
		}

		// Format preview.
		if preview != "" && len(preview) > 60 {
			preview = preview[:57] + "..."
		}

		// Print session entry.
		indexStr := fmt.Sprintf("[%d]", i+1)
		sb.WriteString(textStyle.Render(fmt.Sprintf("%-5s ", indexStr)))
		sb.WriteString(headerStyle.Render(title))
		sb.WriteString("\n")

		sb.WriteString(dimStyle.Render(fmt.Sprintf("      %s  |  %s  |  %d msgs  |  %s",
			dateStr, modelShort, msgCount, id)))
		sb.WriteString("\n")

		if preview != "" {
			sb.WriteString(dimStyle.Render(fmt.Sprintf("      > %s", preview)))
			sb.WriteString("\n")
		}

		sb.WriteString("\n")
	}

	sb.WriteString(dimStyle.Render(fmt.Sprintf("Showing %d sessions. Use /resume <id> to resume a session.", len(sessions))))
	sb.WriteString("\n")

	return sb.String(), sessionIDs
}
