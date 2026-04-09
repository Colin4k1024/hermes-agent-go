package tools

import (
	"encoding/json"
	"testing"
)

func TestClarify(t *testing.T) {
	result := handleClarify(map[string]any{
		"question": "What color do you prefer?",
		"choices":  []any{"Red", "Blue", "Green"},
	}, nil)

	var m map[string]any
	if err := json.Unmarshal([]byte(result), &m); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if m["question"] != "What color do you prefer?" {
		t.Errorf("Expected question back, got %v", m["question"])
	}

	choices, ok := m["choices"].([]any)
	if !ok || len(choices) != 3 {
		t.Errorf("Expected 3 choices, got %v", m["choices"])
	}
}

func TestClarifyNoChoices(t *testing.T) {
	result := handleClarify(map[string]any{
		"question": "What should I do next?",
	}, nil)

	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["question"] == nil {
		t.Error("Expected question in result")
	}
}

func TestClarifyMissingQuestion(t *testing.T) {
	result := handleClarify(map[string]any{}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["error"] == nil {
		t.Error("Expected error for missing question")
	}
}
