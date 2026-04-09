package tools

import (
	"strings"
	"testing"
)

func TestRegisterAndDispatch(t *testing.T) {
	r := &ToolRegistry{
		tools:         make(map[string]*ToolEntry),
		toolsetChecks: make(map[string]func() bool),
	}

	r.Register(&ToolEntry{
		Name:    "test_tool",
		Toolset: "test",
		Schema: map[string]any{
			"name":        "test_tool",
			"description": "A test tool",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler: func(args map[string]any, ctx *ToolContext) string {
			return `{"result":"hello"}`
		},
	})

	if !r.HasTool("test_tool") {
		t.Error("Expected tool to be registered")
	}

	if r.ToolCount() != 1 {
		t.Errorf("Expected 1 tool, got %d", r.ToolCount())
	}

	result := r.Dispatch("test_tool", map[string]any{}, nil)
	if result != `{"result":"hello"}` {
		t.Errorf("Unexpected result: %s", result)
	}
}

func TestDispatchUnknownTool(t *testing.T) {
	r := &ToolRegistry{
		tools:         make(map[string]*ToolEntry),
		toolsetChecks: make(map[string]func() bool),
	}

	result := r.Dispatch("nonexistent", map[string]any{}, nil)
	if !strings.Contains(result, "Unknown tool") {
		t.Errorf("Expected unknown tool error, got: %s", result)
	}
}

func TestGetDefinitions(t *testing.T) {
	r := &ToolRegistry{
		tools:         make(map[string]*ToolEntry),
		toolsetChecks: make(map[string]func() bool),
	}

	r.Register(&ToolEntry{
		Name:    "available_tool",
		Toolset: "test",
		Schema: map[string]any{
			"name":        "available_tool",
			"description": "Available",
		},
		Handler: func(args map[string]any, ctx *ToolContext) string { return "{}" },
	})

	r.Register(&ToolEntry{
		Name:    "unavailable_tool",
		Toolset: "test2",
		Schema: map[string]any{
			"name":        "unavailable_tool",
			"description": "Not available",
		},
		Handler: func(args map[string]any, ctx *ToolContext) string { return "{}" },
		CheckFn: func() bool { return false },
	})

	defs := r.GetDefinitions(map[string]bool{"available_tool": true, "unavailable_tool": true}, true)

	if len(defs) != 1 {
		t.Errorf("Expected 1 definition (unavailable filtered), got %d", len(defs))
	}
}

func TestDeregister(t *testing.T) {
	r := &ToolRegistry{
		tools:         make(map[string]*ToolEntry),
		toolsetChecks: make(map[string]func() bool),
	}

	r.Register(&ToolEntry{
		Name:    "temp_tool",
		Toolset: "temp",
		Schema:  map[string]any{"name": "temp_tool"},
		Handler: func(args map[string]any, ctx *ToolContext) string { return "{}" },
		CheckFn: func() bool { return true },
	})

	if !r.HasTool("temp_tool") {
		t.Error("Tool should exist before deregister")
	}

	r.Deregister("temp_tool")

	if r.HasTool("temp_tool") {
		t.Error("Tool should not exist after deregister")
	}
}

func TestGetToolsetForTool(t *testing.T) {
	r := &ToolRegistry{
		tools:         make(map[string]*ToolEntry),
		toolsetChecks: make(map[string]func() bool),
	}

	r.Register(&ToolEntry{
		Name:    "my_tool",
		Toolset: "my_toolset",
		Schema:  map[string]any{"name": "my_tool"},
		Handler: func(args map[string]any, ctx *ToolContext) string { return "{}" },
	})

	ts := r.GetToolsetForTool("my_tool")
	if ts != "my_toolset" {
		t.Errorf("Expected 'my_toolset', got '%s'", ts)
	}

	ts = r.GetToolsetForTool("nonexistent")
	if ts != "" {
		t.Errorf("Expected empty string, got '%s'", ts)
	}
}
