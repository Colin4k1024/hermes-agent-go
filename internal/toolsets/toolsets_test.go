package toolsets

import (
	"testing"
)

func TestResolveToolset(t *testing.T) {
	tools := ResolveToolset("web")
	if len(tools) == 0 {
		t.Error("Expected web toolset to have tools")
	}

	found := false
	for _, tool := range tools {
		if tool == "web_search" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected web toolset to contain web_search")
	}
}

func TestResolveToolsetTerminal(t *testing.T) {
	tools := ResolveToolset("terminal")
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools in terminal toolset, got %d", len(tools))
	}
}

func TestResolveToolsetHermesCLI(t *testing.T) {
	tools := ResolveToolset("hermes-cli")
	if len(tools) < 20 {
		t.Errorf("Expected hermes-cli to have 20+ tools, got %d", len(tools))
	}
}

func TestResolveToolsetNonExistent(t *testing.T) {
	tools := ResolveToolset("nonexistent-toolset")
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools for nonexistent toolset, got %d", len(tools))
	}
}

func TestResolveMultipleToolsets(t *testing.T) {
	tools := ResolveMultipleToolsets([]string{"web", "terminal"})
	if len(tools) < 3 {
		t.Errorf("Expected at least 3 tools, got %d", len(tools))
	}
}

func TestValidateToolset(t *testing.T) {
	if !ValidateToolset("web") {
		t.Error("Expected 'web' to be valid")
	}
	if !ValidateToolset("hermes-cli") {
		t.Error("Expected 'hermes-cli' to be valid")
	}
	if !ValidateToolset("all") {
		t.Error("Expected 'all' to be valid")
	}
	if ValidateToolset("nonexistent") {
		t.Error("Expected 'nonexistent' to be invalid")
	}
}

func TestGetAllToolsets(t *testing.T) {
	all := GetAllToolsets()
	if len(all) < 10 {
		t.Errorf("Expected at least 10 toolsets, got %d", len(all))
	}

	// Check that hermes-cli exists
	if _, ok := all["hermes-cli"]; !ok {
		t.Error("Expected hermes-cli in all toolsets")
	}
}

func TestGetToolsetNames(t *testing.T) {
	names := GetToolsetNames()
	if len(names) < 10 {
		t.Errorf("Expected at least 10 toolset names, got %d", len(names))
	}
}

func TestResolveToolsetCycleDetection(t *testing.T) {
	// hermes-gateway includes hermes-telegram which doesn't include hermes-gateway
	// so no cycle, but test that resolution completes
	tools := ResolveToolset("hermes-gateway")
	// Should not panic or hang
	_ = tools
}

func TestResolveToolsetAll(t *testing.T) {
	tools := ResolveToolset("all")
	if len(tools) < 30 {
		t.Errorf("Expected 30+ tools for 'all', got %d", len(tools))
	}
}

func TestCoreToolsNotEmpty(t *testing.T) {
	if len(CoreTools) == 0 {
		t.Error("CoreTools should not be empty")
	}
	if len(CoreTools) < 20 {
		t.Errorf("Expected 20+ core tools, got %d", len(CoreTools))
	}
}
