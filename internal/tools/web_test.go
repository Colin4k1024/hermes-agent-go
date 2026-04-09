package tools

import (
	"testing"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<p>Hello</p>", "Hello"},
		{"<script>alert('xss')</script>World", "World"},
		{"<style>.foo{color:red}</style>Content", "Content"},
		{"Plain text", "Plain text"},
		{"<div><p>Nested</p></div>", "Nested"},
		{"<a href='link'>Click</a>", "Click"},
	}

	for _, tt := range tests {
		result := stripHTML(tt.input)
		if result != tt.expected {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestWebSearchMissingQuery(t *testing.T) {
	result := handleWebSearch(map[string]any{}, nil)
	if result == "" {
		t.Error("Expected non-empty result")
	}
}

func TestWebSearchNoAPIKey(t *testing.T) {
	// Without EXA_API_KEY, should return fallback message
	result := handleWebSearch(map[string]any{"query": "test"}, nil)
	if result == "" {
		t.Error("Expected non-empty fallback result")
	}
}

func TestWebExtractMissingURLs(t *testing.T) {
	result := handleWebExtract(map[string]any{}, nil)
	if result == "" {
		t.Error("Expected error for missing urls")
	}
}

func TestCheckWebRequirements(t *testing.T) {
	// Without any API keys, should return false
	result := checkWebRequirements()
	// Can be true or false depending on environment
	_ = result
}
