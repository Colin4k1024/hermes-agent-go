package agent

import (
	"fmt"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
	"github.com/hermes-agent/hermes-agent-go/internal/state"
)

// Checkpoint represents a saved point in a session's conversation history.
type Checkpoint struct {
	SessionID    string    `json:"session_id"`
	MessageCount int       `json:"message_count"`
	Timestamp    time.Time `json:"timestamp"`
	Summary      string    `json:"summary"`
	Model        string    `json:"model"`
}

// SaveCheckpoint creates a checkpoint of the current session state.
// It records the session's message count, timestamp, model, and generates
// a brief summary from the conversation.
func SaveCheckpoint(sessionDB *state.SessionDB, sessionID string) (*Checkpoint, error) {
	if sessionDB == nil {
		return nil, fmt.Errorf("session database not available")
	}

	// Get session metadata.
	sess, err := sessionDB.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Get messages to build a summary.
	messages, err := sessionDB.GetMessages(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	msgCount := len(messages)
	model, _ := sess["model"].(string)

	// Build a brief summary from the first user message and last assistant message.
	summary := buildCheckpointSummary(messages)

	checkpoint := &Checkpoint{
		SessionID:    sessionID,
		MessageCount: msgCount,
		Timestamp:    time.Now(),
		Summary:      summary,
		Model:        model,
	}

	// Persist the checkpoint as a title annotation on the session.
	// This uses the existing title mechanism with a checkpoint prefix.
	existingTitle := sessionDB.GetSessionTitle(sessionID)
	checkpointTitle := existingTitle
	if checkpointTitle == "" {
		checkpointTitle = summary
	}

	// Record the checkpoint in the session title with a marker.
	markerTitle := fmt.Sprintf("[CP:%d] %s", msgCount, checkpointTitle)
	if err := sessionDB.SetSessionTitle(sessionID, markerTitle); err != nil {
		return nil, fmt.Errorf("save checkpoint title: %w", err)
	}

	return checkpoint, nil
}

// ListCheckpoints lists all sessions that have checkpoint markers.
func ListCheckpoints(sessionDB *state.SessionDB) ([]Checkpoint, error) {
	if sessionDB == nil {
		return nil, fmt.Errorf("session database not available")
	}

	// List recent sessions.
	sessions, err := sessionDB.ListSessions("", 100, 0)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var checkpoints []Checkpoint
	for _, sess := range sessions {
		title, _ := sess["title"].(string)
		if len(title) < 4 || title[:3] != "[CP" {
			continue
		}

		sessionID, _ := sess["id"].(string)
		model, _ := sess["model"].(string)
		startedAt, _ := sess["started_at"].(float64)
		msgCount, _ := sess["message_count"].(int64)

		// Parse the checkpoint summary from the title.
		summary := title
		if idx := findByte(title, ']'); idx >= 0 && idx+2 < len(title) {
			summary = title[idx+2:]
		}

		cp := Checkpoint{
			SessionID:    sessionID,
			MessageCount: int(msgCount),
			Timestamp:    time.Unix(int64(startedAt), 0),
			Summary:      summary,
			Model:        model,
		}
		checkpoints = append(checkpoints, cp)
	}

	return checkpoints, nil
}

// RestoreCheckpoint loads the conversation messages from a checkpointed session.
func RestoreCheckpoint(sessionDB *state.SessionDB, sessionID string) ([]llm.Message, error) {
	if sessionDB == nil {
		return nil, fmt.Errorf("session database not available")
	}

	// Verify the session exists.
	sess, err := sessionDB.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if sess == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Load all messages.
	rawMsgs, err := sessionDB.GetMessages(sessionID)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	var messages []llm.Message
	for _, raw := range rawMsgs {
		role, _ := raw["role"].(string)
		content, _ := raw["content"].(string)
		toolCallID, _ := raw["tool_call_id"].(string)
		toolName, _ := raw["tool_name"].(string)
		reasoning, _ := raw["reasoning"].(string)

		msg := llm.Message{
			Role:       role,
			Content:    content,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Reasoning:  reasoning,
		}

		messages = append(messages, msg)
	}

	return messages, nil
}

// buildCheckpointSummary creates a short summary from conversation messages.
func buildCheckpointSummary(messages []map[string]any) string {
	// Find first user message for context.
	firstUser := ""
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role == "user" && content != "" {
			firstUser = content
			break
		}
	}

	if firstUser == "" {
		return "Empty session"
	}

	// Truncate to a reasonable summary length.
	if len(firstUser) > 80 {
		firstUser = firstUser[:77] + "..."
	}

	return firstUser
}

func findByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
