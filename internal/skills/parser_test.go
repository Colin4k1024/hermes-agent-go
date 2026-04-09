package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, "SKILL.md")

	content := `---
name: test-skill
description: A test skill for unit testing
version: 1.0.0
author: Test Author
metadata:
  hermes:
    tags: [testing, unit-test]
---

# Test Skill

This is the body of the test skill.

## When to Use
Use this for testing.
`
	os.WriteFile(skillPath, []byte(content), 0644)

	meta, body, err := ParseSkillMD(skillPath)
	if err != nil {
		t.Fatalf("ParseSkillMD failed: %v", err)
	}

	if meta.Name != "test-skill" {
		t.Errorf("Expected name 'test-skill', got '%s'", meta.Name)
	}
	if meta.Description != "A test skill for unit testing" {
		t.Errorf("Expected description mismatch, got '%s'", meta.Description)
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", meta.Version)
	}
	if meta.Author != "Test Author" {
		t.Errorf("Expected author 'Test Author', got '%s'", meta.Author)
	}
	if body == "" {
		t.Error("Expected non-empty body")
	}
}

func TestParseSkillMDNoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, "SKILL.md")

	content := `# Plain Skill

No frontmatter here, just plain markdown.
`
	os.WriteFile(skillPath, []byte(content), 0644)

	meta, body, err := ParseSkillMD(skillPath)
	if err != nil {
		t.Fatalf("ParseSkillMD failed: %v", err)
	}

	if meta.Name != "" {
		t.Errorf("Expected empty name, got '%s'", meta.Name)
	}
	if body == "" {
		t.Error("Expected non-empty body")
	}
}

func TestParseSkillMDMissing(t *testing.T) {
	_, _, err := ParseSkillMD("/nonexistent/path/SKILL.md")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}

func TestSkillMatchesPlatform(t *testing.T) {
	// No platform restriction
	meta := &SkillMeta{}
	if !SkillMatchesPlatform(meta) {
		t.Error("Expected match when no platforms specified")
	}

	// Empty platforms list
	meta = &SkillMeta{Platforms: []string{}}
	if !SkillMatchesPlatform(meta) {
		t.Error("Expected match when platforms list is empty")
	}
}
