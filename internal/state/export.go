package state

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ExportedSession is the JSON-serializable format for a full session export.
type ExportedSession struct {
	SessionID  string            `json:"session_id"`
	Source     string            `json:"source"`
	Model      string            `json:"model"`
	Title      string            `json:"title"`
	StartedAt  string            `json:"started_at"`
	EndedAt    string            `json:"ended_at,omitempty"`
	Tokens     ExportedTokens    `json:"tokens"`
	Messages   []ExportedMessage `json:"messages"`
	ExportedAt string            `json:"exported_at"`
}

// ExportedTokens holds token usage stats for the export.
type ExportedTokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

// ExportedMessage is a single message in the exported session.
type ExportedMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// ExportSessionJSON exports a full conversation to a JSON file.
func ExportSessionJSON(db *SessionDB, sessionID, outputPath string) error {
	exported, err := buildExportedSession(db, sessionID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// ExportSessionMarkdown exports a full conversation to a Markdown file.
func ExportSessionMarkdown(db *SessionDB, sessionID, outputPath string) error {
	exported, err := buildExportedSession(db, sessionID)
	if err != nil {
		return err
	}

	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Session: %s\n\n", exported.Title))
	sb.WriteString(fmt.Sprintf("- **Session ID:** %s\n", exported.SessionID))
	sb.WriteString(fmt.Sprintf("- **Model:** %s\n", exported.Model))
	sb.WriteString(fmt.Sprintf("- **Source:** %s\n", exported.Source))
	sb.WriteString(fmt.Sprintf("- **Started:** %s\n", exported.StartedAt))
	if exported.EndedAt != "" {
		sb.WriteString(fmt.Sprintf("- **Ended:** %s\n", exported.EndedAt))
	}
	sb.WriteString(fmt.Sprintf("- **Tokens:** %d input, %d output\n",
		exported.Tokens.Input, exported.Tokens.Output))
	sb.WriteString(fmt.Sprintf("- **Exported:** %s\n", exported.ExportedAt))
	sb.WriteString("\n---\n\n")

	// Messages
	for _, msg := range exported.Messages {
		switch msg.Role {
		case "user":
			sb.WriteString("## User\n\n")
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")

		case "assistant":
			sb.WriteString("## Assistant\n\n")
			if msg.Reasoning != "" {
				sb.WriteString("<details>\n<summary>Reasoning</summary>\n\n")
				sb.WriteString(msg.Reasoning)
				sb.WriteString("\n</details>\n\n")
			}
			sb.WriteString(msg.Content)
			sb.WriteString("\n\n")

		case "tool":
			toolLabel := msg.ToolName
			if toolLabel == "" {
				toolLabel = "tool"
			}
			sb.WriteString(fmt.Sprintf("### Tool Result (%s)\n\n", toolLabel))
			// Wrap tool output in a code block for readability.
			sb.WriteString("```\n")
			sb.WriteString(msg.Content)
			sb.WriteString("\n```\n\n")

		case "system":
			// Omit system prompts from markdown export.
			continue
		}
	}

	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// buildExportedSession loads session data from the DB and constructs an ExportedSession.
func buildExportedSession(db *SessionDB, sessionID string) (*ExportedSession, error) {
	// Load session metadata.
	sess, err := db.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Load messages.
	rawMsgs, err := db.GetMessages(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	// Build exported messages.
	var messages []ExportedMessage
	for _, raw := range rawMsgs {
		msg := ExportedMessage{
			Role: stringVal(raw, "role"),
		}

		if content, ok := raw["content"].(string); ok {
			msg.Content = content
		}
		if toolName, ok := raw["tool_name"].(string); ok {
			msg.ToolName = toolName
		}
		if reasoning, ok := raw["reasoning"].(string); ok {
			msg.Reasoning = reasoning
		}

		messages = append(messages, msg)
	}

	// Build timestamps.
	startedAt := ""
	if v, ok := sess["started_at"].(float64); ok && v > 0 {
		startedAt = time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
	}
	endedAt := ""
	if v, ok := sess["ended_at"].(float64); ok && v > 0 {
		endedAt = time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
	}

	exported := &ExportedSession{
		SessionID: stringVal(sess, "id"),
		Source:    stringVal(sess, "source"),
		Model:     stringVal(sess, "model"),
		Title:     stringVal(sess, "title"),
		StartedAt: startedAt,
		EndedAt:   endedAt,
		Tokens: ExportedTokens{
			Input:  int64Val(sess, "input_tokens"),
			Output: int64Val(sess, "output_tokens"),
		},
		Messages:   messages,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if exported.Title == "" {
		exported.Title = "Untitled Session"
	}

	return exported, nil
}

// stringVal safely extracts a string from a map.
func stringVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// int64Val safely extracts an int64 from a map.
func int64Val(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	default:
		return 0
	}
}
