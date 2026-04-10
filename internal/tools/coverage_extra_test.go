package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleReadFile_TableDriven(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		wantSub string
	}{
		{"read existing file", map[string]any{"file_path": testFile}, false, "line1"},
		{"read with limit", map[string]any{"file_path": testFile, "limit": float64(1)}, false, "line1"},
		{"read nonexistent", map[string]any{"file_path": filepath.Join(dir, "nope.txt")}, true, ""},
		{"empty path", map[string]any{}, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleReadFile(tt.args, nil)
			if tt.wantErr && !strings.Contains(result, "error") {
				t.Errorf("expected error, got: %s", result)
			}
			if !tt.wantErr && tt.wantSub != "" && !strings.Contains(result, tt.wantSub) {
				t.Errorf("result = %q, want to contain %q", result, tt.wantSub)
			}
		})
	}
}

func TestHandleWriteFile_TableDriven(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
	}{
		{"write new file", map[string]any{"file_path": filepath.Join(dir, "new.txt"), "content": "hello"}, false},
		{"write no path", map[string]any{"content": "data"}, true},
		{"write empty content", map[string]any{"file_path": filepath.Join(dir, "e.txt")}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleWriteFile(tt.args, nil)
			if tt.wantErr && !strings.Contains(result, "error") {
				t.Errorf("expected error, got: %s", result)
			}
		})
	}
}

func TestHandleDelegateTask_Validation(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		ctx  *ToolContext
		want string
	}{
		{"empty args", map[string]any{}, nil, "error"},
		{"too many tasks", map[string]any{"tasks": make([]any, 6)}, nil, "Maximum 5"},
		{"no valid goals", map[string]any{"tasks": []any{map[string]any{"x": "y"}}}, nil, "error"},
		{"depth exceeded", map[string]any{"tasks": []any{map[string]any{"goal": "test"}}},
			&ToolContext{Extra: map[string]any{depthContextKey: maxDelegationDepth}}, "depth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleDelegateTask(tt.args, tt.ctx)
			if !strings.Contains(result, tt.want) {
				t.Errorf("got %q, want to contain %q", result, tt.want)
			}
		})
	}
}

func TestHandleTerminal_Validation(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"empty command", map[string]any{}, "command"},
		{"sudo blocked", map[string]any{"command": "sudo rm -rf /"}, "sudo"},
		{"dangerous blocked", map[string]any{"command": "rm -rf /"}, "dangerous"},
		{"simple echo", map[string]any{"command": "echo test123"}, "test123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handleTerminal(tt.args, nil)
			if !strings.Contains(result, tt.want) {
				t.Errorf("got %q, want to contain %q", result, tt.want)
			}
		})
	}
}

func TestRegistryDispatch_Unknown(t *testing.T) {
	result := Registry().Dispatch("nonexistent_tool_xyz", map[string]any{}, nil)
	if !strings.Contains(result, "error") {
		t.Errorf("expected error for unknown tool, got: %s", result)
	}
}

func TestRegistryGetAllToolNames_NonEmpty(t *testing.T) {
	names := Registry().GetAllToolNames()
	if len(names) == 0 {
		t.Error("expected at least 1 registered tool")
	}
}
