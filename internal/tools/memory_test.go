package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMemorySaveAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	// Save
	result := handleMemory(map[string]any{
		"action":  "save",
		"key":     "test-key",
		"content": "test content for memory",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected save success, got: %s", result)
	}

	// Read
	result = handleMemory(map[string]any{"action": "read"}, nil)
	json.Unmarshal([]byte(result), &m)
	content, _ := m["content"].(string)
	if content == "" {
		t.Error("Expected non-empty memory content")
	}
}

func TestMemoryDelete(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "memories"), 0755)

	// Save first
	handleMemory(map[string]any{"action": "save", "key": "to-delete", "content": "will be deleted"}, nil)

	// Delete
	result := handleMemory(map[string]any{"action": "delete", "key": "to-delete"}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected delete success, got: %s", result)
	}
}

func TestMemoryUserProfile(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	// Save profile
	result := handleMemory(map[string]any{
		"action":  "save_user",
		"content": "Name: Test User\nRole: Developer",
	}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected save_user success, got: %s", result)
	}

	// Read profile
	result = handleMemory(map[string]any{"action": "read_user"}, nil)
	json.Unmarshal([]byte(result), &m)
	if m["content"] == nil || m["content"] == "" {
		t.Error("Expected non-empty user profile")
	}
}

func TestMemoryInvalidAction(t *testing.T) {
	result := handleMemory(map[string]any{"action": "invalid"}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["error"] == nil {
		t.Error("Expected error for invalid action")
	}
}
