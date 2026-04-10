package agent

import (
	"testing"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

func TestRepairToolCall(t *testing.T) {
	validNames := map[string]bool{
		"terminal":     true,
		"read_file":    true,
		"write_file":   true,
		"web_search":   true,
		"vision_analyze": true,
	}

	tests := []struct {
		name      string
		input     string
		wantName  string
		wantFound bool
	}{
		{"exact match", "terminal", "terminal", true},
		{"lowercase", "Terminal", "terminal", true},
		{"uppercase", "READ_FILE", "read_file", true},
		{"hyphens", "read-file", "read_file", true},
		{"spaces", "web search", "web_search", true},
		{"typo close match", "terminl", "terminal", true},
		{"typo read_fle", "read_fle", "read_file", true},
		{"completely wrong", "xyzabc123", "", false},
		{"empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := RepairToolCall(tt.input, validNames)
			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if got != tt.wantName {
				t.Errorf("name = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestRepairToolCalls(t *testing.T) {
	validNames := map[string]bool{
		"terminal":  true,
		"read_file": true,
	}

	calls := []llm.ToolCall{
		{Function: llm.FunctionCall{Name: "Terminal", Arguments: "{}"}},
		{Function: llm.FunctionCall{Name: "read_file", Arguments: "{}"}},
		{Function: llm.FunctionCall{Name: "reed_file", Arguments: "{}"}},
	}

	repaired, count := RepairToolCalls(calls, validNames)
	if count != 2 {
		t.Errorf("repair count = %d, want 2", count)
	}
	if repaired[0].Function.Name != "terminal" {
		t.Errorf("repaired[0] = %q, want %q", repaired[0].Function.Name, "terminal")
	}
	if repaired[2].Function.Name != "read_file" {
		t.Errorf("repaired[2] = %q, want %q", repaired[2].Function.Name, "read_file")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"terminal", "terminl", 1},
		{"read_file", "reed_file", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := levenshteinDistance(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestDeduplicateToolCalls(t *testing.T) {
	calls := []llm.ToolCall{
		{ID: "1", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"/a.txt"}`}},
		{ID: "2", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"/a.txt"}`}},
		{ID: "3", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"/b.txt"}`}},
	}

	result := DeduplicateToolCalls(calls)
	if len(result) != 2 {
		t.Errorf("got %d calls, want 2", len(result))
	}
	if result[0].ID != "1" || result[1].ID != "3" {
		t.Errorf("wrong calls kept: %v", result)
	}
}

func TestDeduplicateToolCalls_NoDupes(t *testing.T) {
	calls := []llm.ToolCall{
		{ID: "1", Function: llm.FunctionCall{Name: "a", Arguments: "{}"}},
		{ID: "2", Function: llm.FunctionCall{Name: "b", Arguments: "{}"}},
	}

	result := DeduplicateToolCalls(calls)
	// Should return original slice when no dupes
	if len(result) != 2 {
		t.Errorf("got %d calls, want 2", len(result))
	}
}

func TestShouldParallelizeToolBatch(t *testing.T) {
	tests := []struct {
		name  string
		calls []llm.ToolCall
		want  bool
	}{
		{
			name:  "single call",
			calls: []llm.ToolCall{{Function: llm.FunctionCall{Name: "read_file"}}},
			want:  false,
		},
		{
			name: "two safe tools",
			calls: []llm.ToolCall{
				{Function: llm.FunctionCall{Name: "web_search", Arguments: `{"query":"go"}`}},
				{Function: llm.FunctionCall{Name: "web_search", Arguments: `{"query":"rust"}`}},
			},
			want: true,
		},
		{
			name: "never parallel tool",
			calls: []llm.ToolCall{
				{Function: llm.FunctionCall{Name: "clarify", Arguments: "{}"}},
				{Function: llm.FunctionCall{Name: "web_search", Arguments: "{}"}},
			},
			want: false,
		},
		{
			name: "path scoped non-overlapping",
			calls: []llm.ToolCall{
				{Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"/tmp/a.txt"}`}},
				{Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"/tmp/b.txt"}`}},
			},
			want: true,
		},
		{
			name: "path scoped overlapping",
			calls: []llm.ToolCall{
				{Function: llm.FunctionCall{Name: "write_file", Arguments: `{"path":"/tmp/dir/a.txt"}`}},
				{Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"/tmp/dir"}`}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldParallelizeToolBatch(tt.calls)
			if got != tt.want {
				t.Errorf("ShouldParallelizeToolBatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathsOverlap(t *testing.T) {
	tests := []struct {
		left, right string
		want        bool
	}{
		{"/tmp/a.txt", "/tmp/b.txt", false},
		{"/tmp/dir", "/tmp/dir/file.txt", true},
		{"/tmp/dir/file.txt", "/tmp/dir", true},
		{"/a", "/b", false},
		{"/a/b/c", "/a/b/c", true},
		{"", "/tmp", false},
	}

	for _, tt := range tests {
		t.Run(tt.left+"_"+tt.right, func(t *testing.T) {
			got := pathsOverlap(tt.left, tt.right)
			if got != tt.want {
				t.Errorf("pathsOverlap(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}
