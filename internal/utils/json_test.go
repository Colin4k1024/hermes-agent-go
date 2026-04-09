package utils

import (
	"encoding/json"
	"testing"
)

func TestToolError(t *testing.T) {
	result := ToolError("something failed")
	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
	if m["error"] != "something failed" {
		t.Errorf("Expected error message, got %v", m["error"])
	}
}

func TestToolErrorWithExtra(t *testing.T) {
	result := ToolError("bad input", map[string]any{"code": 400})
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["error"] != "bad input" {
		t.Errorf("Expected 'bad input', got %v", m["error"])
	}
	if m["code"] != float64(400) {
		t.Errorf("Expected code 400, got %v", m["code"])
	}
}

func TestToolResult(t *testing.T) {
	result := ToolResult(map[string]any{"success": true, "count": 42})
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected success=true, got %v", m["success"])
	}
	if m["count"] != float64(42) {
		t.Errorf("Expected count=42, got %v", m["count"])
	}
}

func TestToJSON(t *testing.T) {
	result := ToJSON(map[string]any{"key": "value"})
	if result == "{}" {
		t.Error("Expected non-empty JSON")
	}
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["key"] != "value" {
		t.Errorf("Expected key=value, got %v", m["key"])
	}
}

func TestFromJSON(t *testing.T) {
	m := FromJSON(`{"name":"test","num":123}`)
	if m == nil {
		t.Fatal("Expected non-nil map")
	}
	if m["name"] != "test" {
		t.Errorf("Expected name=test, got %v", m["name"])
	}
	if m["num"] != float64(123) {
		t.Errorf("Expected num=123, got %v", m["num"])
	}

	// Invalid JSON
	m = FromJSON("not json")
	if m != nil {
		t.Error("Expected nil for invalid JSON")
	}
}
