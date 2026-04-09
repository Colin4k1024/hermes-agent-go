package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllSkillsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	skillsDir := filepath.Join(tmpDir, "skills")
	os.MkdirAll(skillsDir, 0755)

	skills, err := LoadAllSkills()
	if err != nil {
		t.Fatalf("LoadAllSkills failed: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("Expected 0 skills in empty dir, got %d", len(skills))
	}
}

func TestLoadAllSkillsWithSkill(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	skillsDir := filepath.Join(tmpDir, "skills", "test-skill")
	os.MkdirAll(skillsDir, 0755)

	skillContent := `---
name: test-skill
description: A test skill
version: 1.0.0
---

# Test Skill
Instructions here.
`
	os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(skillContent), 0644)

	skills, err := LoadAllSkills()
	if err != nil {
		t.Fatalf("LoadAllSkills failed: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("Expected 1 skill, got %d", len(skills))
	}
}

func TestBuildSkillsIndex(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	skillsDir := filepath.Join(tmpDir, "skills", "my-skill")
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(`---
name: my-skill
description: Does things
---
# My Skill
`), 0644)

	skills, _ := LoadAllSkills()
	index := BuildSkillsIndex(skills)
	if len(index) != 1 {
		t.Errorf("Expected 1 skill in index, got %d", len(index))
	}
}

func TestFindSkill(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	skillsDir := filepath.Join(tmpDir, "skills", "finder-test")
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(`---
name: finder-test
description: Test for FindSkill
---
# Finder Test
`), 0644)

	entry, err := FindSkill("finder-test")
	if err != nil {
		t.Fatalf("FindSkill failed: %v", err)
	}
	if entry == nil {
		t.Error("Expected to find skill 'finder-test'")
	}

	entry, _ = FindSkill("nonexistent-skill")
	if entry != nil {
		t.Error("Expected nil for nonexistent skill")
	}
}
