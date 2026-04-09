package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionDB(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := NewSessionDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	// Create session
	err = db.CreateSession("test-session-1", "cli", "gpt-4", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Append messages
	_, err = db.AppendMessage("test-session-1", "user", "Hello", "", "", nil, "")
	if err != nil {
		t.Fatalf("Failed to append user message: %v", err)
	}

	_, err = db.AppendMessage("test-session-1", "assistant", "Hi there!", "", "", nil, "")
	if err != nil {
		t.Fatalf("Failed to append assistant message: %v", err)
	}

	// Get messages
	msgs, err := db.GetMessages("test-session-1")
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}

	if msgs[0]["role"] != "user" || msgs[0]["content"] != "Hello" {
		t.Errorf("First message mismatch: %v", msgs[0])
	}

	// Set title
	err = db.SetSessionTitle("test-session-1", "Test Session")
	if err != nil {
		t.Fatalf("Failed to set title: %v", err)
	}

	title := db.GetSessionTitle("test-session-1")
	if title != "Test Session" {
		t.Errorf("Expected 'Test Session', got '%s'", title)
	}

	// Get session
	session, err := db.GetSession("test-session-1")
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}
	if session["source"] != "cli" {
		t.Errorf("Expected source 'cli', got '%s'", session["source"])
	}

	// List sessions
	sessions, err := db.ListSessions("", 10, 0)
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	// End session
	err = db.EndSession("test-session-1", "completed")
	if err != nil {
		t.Fatalf("Failed to end session: %v", err)
	}

	// Delete session
	err = db.DeleteSession("test-session-1")
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	sessions, _ = db.ListSessions("", 10, 0)
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after delete, got %d", len(sessions))
	}
}

func TestTokenCounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	db, err := NewSessionDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	db.CreateSession("tok-session", "cli", "gpt-4", "")

	err = db.UpdateTokenCounts("tok-session", 100, 50, 10, 5, 20)
	if err != nil {
		t.Fatalf("Failed to update tokens: %v", err)
	}

	session, _ := db.GetSession("tok-session")
	if session["input_tokens"].(int64) != 100 {
		t.Errorf("Expected 100 input tokens, got %v", session["input_tokens"])
	}

	// Increment
	db.UpdateTokenCounts("tok-session", 200, 100, 0, 0, 0)
	session, _ = db.GetSession("tok-session")
	if session["input_tokens"].(int64) != 300 {
		t.Errorf("Expected 300 input tokens after increment, got %v", session["input_tokens"])
	}
}
