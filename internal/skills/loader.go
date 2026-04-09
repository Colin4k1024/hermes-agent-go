package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// ExcludedDirs contains directory names to skip during skill scanning.
var ExcludedDirs = map[string]bool{
	".git":    true,
	".github": true,
	".hub":    true,
}

// SkillEntry represents a loaded skill with its metadata and content.
type SkillEntry struct {
	Meta    *SkillMeta
	Body    string
	DirName string // Directory name (used as the command name)
}

// LoadAllSkills scans the skills directory and loads all SKILL.md files.
// Filters by platform compatibility.
func LoadAllSkills() ([]*SkillEntry, error) {
	skillsDir := filepath.Join(config.HermesHome(), "skills")

	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	var skills []*SkillEntry
	err := scanSkillsDir(skillsDir, "", &skills)
	if err != nil {
		return nil, fmt.Errorf("scan skills directory: %w", err)
	}

	// Sort by name.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Meta.Name < skills[j].Meta.Name
	})

	return skills, nil
}

// scanSkillsDir recursively scans a directory for SKILL.md files.
func scanSkillsDir(dir, prefix string, skills *[]*SkillEntry) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if ExcludedDirs[name] {
			continue
		}

		// Check for SKILL.md in this directory.
		skillMDPath := filepath.Join(dir, name, "SKILL.md")
		if _, err := os.Stat(skillMDPath); err == nil {
			meta, body, err := ParseSkillMD(skillMDPath)
			if err != nil {
				slog.Debug("Failed to parse skill", "path", skillMDPath, "error", err)
				continue
			}

			// Set name from directory if not in frontmatter.
			if meta.Name == "" {
				meta.Name = name
			}

			// Check platform compatibility.
			if !SkillMatchesPlatform(meta) {
				slog.Debug("Skill skipped (platform mismatch)", "skill", meta.Name)
				continue
			}

			dirName := name
			if prefix != "" {
				dirName = prefix + "/" + name
			}

			*skills = append(*skills, &SkillEntry{
				Meta:    meta,
				Body:    body,
				DirName: dirName,
			})
		}

		// Scan subdirectories (one level deep).
		subPrefix := name
		if prefix != "" {
			subPrefix = prefix + "/" + name
		}
		subDir := filepath.Join(dir, name)
		subEntries, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		for _, subEntry := range subEntries {
			if !subEntry.IsDir() || ExcludedDirs[subEntry.Name()] {
				continue
			}
			subSkillMD := filepath.Join(subDir, subEntry.Name(), "SKILL.md")
			if _, err := os.Stat(subSkillMD); err == nil {
				meta, body, err := ParseSkillMD(subSkillMD)
				if err != nil {
					slog.Debug("Failed to parse sub-skill", "path", subSkillMD, "error", err)
					continue
				}
				if meta.Name == "" {
					meta.Name = subEntry.Name()
				}
				if !SkillMatchesPlatform(meta) {
					continue
				}
				*skills = append(*skills, &SkillEntry{
					Meta:    meta,
					Body:    body,
					DirName: subPrefix + "/" + subEntry.Name(),
				})
			}
		}
	}

	return nil
}

// BuildSkillsIndex builds a map of skill name -> SkillEntry for quick lookup.
func BuildSkillsIndex(skills []*SkillEntry) map[string]*SkillEntry {
	index := make(map[string]*SkillEntry, len(skills))
	for _, skill := range skills {
		index[skill.Meta.Name] = skill
		// Also index by directory name.
		if skill.DirName != "" {
			index[skill.DirName] = skill
		}
	}
	return index
}

// BuildSkillsPrompt builds a system prompt section listing available skills.
func BuildSkillsPrompt(skills []*SkillEntry) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("The user can load skills with slash commands. Available skills:\n")

	for _, skill := range skills {
		desc := skill.Meta.Description
		if desc == "" {
			desc = "No description"
		}
		sb.WriteString(fmt.Sprintf("- /%s: %s\n", skill.Meta.Name, desc))
	}

	sb.WriteString("\nWhen the user types a skill command, load and follow its instructions.")
	return sb.String()
}

// GetSkillsByCategory groups skills by their category.
func GetSkillsByCategory(skills []*SkillEntry) map[string][]*SkillEntry {
	result := make(map[string][]*SkillEntry)
	for _, skill := range skills {
		category := skill.Meta.Category
		if category == "" {
			category = "general"
		}
		result[category] = append(result[category], skill)
	}
	return result
}

// FindSkill looks up a skill by name in the skills directory.
func FindSkill(name string) (*SkillEntry, error) {
	skills, err := LoadAllSkills()
	if err != nil {
		return nil, err
	}

	index := BuildSkillsIndex(skills)
	skill, ok := index[name]
	if !ok {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return skill, nil
}
