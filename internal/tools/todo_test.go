package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTodoEnv(t *testing.T) func() {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, "cache"), 0755)
	return func() { os.Unsetenv("HERMES_HOME") }
}

func TestTodoAddAndList(t *testing.T) {
	cleanup := setupTodoEnv(t)
	defer cleanup()

	// Add
	result := handleTodo(map[string]any{"action": "add", "task": "Write tests", "priority": "high"}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Fatalf("Expected add success, got: %s", result)
	}

	// List
	result = handleTodo(map[string]any{"action": "list"}, nil)
	json.Unmarshal([]byte(result), &m)
	todos, _ := m["todos"].([]any)
	if len(todos) != 1 {
		t.Errorf("Expected 1 todo, got %d", len(todos))
	}
}

func TestTodoUpdate(t *testing.T) {
	cleanup := setupTodoEnv(t)
	defer cleanup()

	handleTodo(map[string]any{"action": "add", "task": "Update me"}, nil)

	result := handleTodo(map[string]any{
		"action":  "update",
		"task_id": float64(1),
		"status":  "done",
	}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected update success, got: %s", result)
	}
}

func TestTodoRemove(t *testing.T) {
	cleanup := setupTodoEnv(t)
	defer cleanup()

	handleTodo(map[string]any{"action": "add", "task": "Remove me"}, nil)

	result := handleTodo(map[string]any{"action": "remove", "task_id": float64(1)}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected remove success, got: %s", result)
	}

	// Verify empty
	result = handleTodo(map[string]any{"action": "list"}, nil)
	json.Unmarshal([]byte(result), &m)
	count, _ := m["count"].(float64)
	if count != 0 {
		t.Errorf("Expected 0 todos after remove, got %v", count)
	}
}

func TestTodoClear(t *testing.T) {
	cleanup := setupTodoEnv(t)
	defer cleanup()

	handleTodo(map[string]any{"action": "add", "task": "A"}, nil)
	handleTodo(map[string]any{"action": "add", "task": "B"}, nil)

	result := handleTodo(map[string]any{"action": "clear"}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Error("Expected clear success")
	}
}

func TestTodoMissingTask(t *testing.T) {
	result := handleTodo(map[string]any{"action": "add"}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["error"] == nil {
		t.Error("Expected error for add without task")
	}
}
