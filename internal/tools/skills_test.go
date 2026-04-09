package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsList(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	// Create a skill
	skillDir := filepath.Join(tmpDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test-skill
description: A test
---
# Test
`), 0644)

	result := handleSkillsList(map[string]any{}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)

	skills, _ := m["skills"].([]any)
	if len(skills) == 0 {
		t.Error("Expected at least 1 skill")
	}
}

func TestSkillsListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "skills"), 0755)

	result := handleSkillsList(map[string]any{}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	// Should not error
	if m["error"] != nil {
		t.Errorf("Unexpected error: %v", m["error"])
	}
}

func TestSkillView(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	skillDir := filepath.Join(tmpDir, "skills", "viewable")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: viewable
description: Viewable skill
---
# Viewable Skill
Instructions here.
`), 0644)

	result := handleSkillView(map[string]any{"name": "viewable"}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["error"] != nil {
		t.Errorf("Unexpected error: %v", m["error"])
	}
}

func TestSkillViewNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "skills"), 0755)

	result := handleSkillView(map[string]any{"name": "nonexistent"}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["error"] == nil {
		t.Error("Expected error for nonexistent skill")
	}
}

func TestSkillManageCreate(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "skills"), 0755)

	result := handleSkillManage(map[string]any{
		"action":      "create",
		"name":  "new-skill",
		"description": "A brand new skill",
		"content":     "# New Skill\nDo stuff.",
	}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected create success, got: %s", result)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, "skills", "new-skill", "SKILL.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Expected SKILL.md to exist after create")
	}
}

func TestSkillManageDelete(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	skillDir := filepath.Join(tmpDir, "skills", "to-delete")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Delete me"), 0644)

	result := handleSkillManage(map[string]any{
		"action":     "delete",
		"name": "to-delete",
	}, nil)
	var m map[string]any
	json.Unmarshal([]byte(result), &m)
	if m["success"] != true {
		t.Errorf("Expected delete success, got: %s", result)
	}
}
