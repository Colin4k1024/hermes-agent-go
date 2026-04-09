package tools

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestTerminalEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	result := handleTerminal(map[string]any{
		"command": "echo hello-test",
	}, nil)

	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("Invalid JSON result: %v", err)
	}

	stdout, _ := m["stdout"].(string)
	if stdout == "" {
		t.Error("Expected non-empty stdout")
	}
	exitCode, _ := m["exit_code"].(float64)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %v", exitCode)
	}
}

func TestTerminalTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	result := handleTerminal(map[string]any{
		"command": "sleep 10",
		"timeout": float64(1),
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	// Should either have "error" key or non-zero exit code
	if m["error"] == nil {
		exitCode, _ := m["exit_code"].(float64)
		if exitCode == 0 {
			t.Error("Expected timeout error or non-zero exit code")
		}
	}
}

func TestTerminalMissingCommand(t *testing.T) {
	result := handleTerminal(map[string]any{}, nil)
	if result != `{"error":"command is required"}` {
		t.Errorf("Expected error, got: %s", result)
	}
}

func TestTerminalBackground(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	result := handleTerminal(map[string]any{
		"command":    "sleep 0.1",
		"background": true,
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if m["process_id"] == nil {
		t.Error("Expected process_id for background command")
	}
	if m["status"] != "running" {
		t.Errorf("Expected status 'running', got %v", m["status"])
	}
}

func TestProcessList(t *testing.T) {
	result := handleProcess(map[string]any{
		"action": "list",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	if _, ok := m["processes"]; !ok {
		t.Error("Expected 'processes' key in list result")
	}
}

func TestProcessInvalidAction(t *testing.T) {
	result := handleProcess(map[string]any{
		"action": "invalid",
	}, nil)

	if result == "" {
		t.Error("Expected error message")
	}
}
