package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

func init() {
	Register(&ToolEntry{
		Name:    "skills_list",
		Toolset: "skills",
		Schema: map[string]any{
			"name":        "skills_list",
			"description": "List all available skills from the skills directory (~/.hermes/skills/). Shows skill name, description, and status.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler: handleSkillsList,
		Emoji:   "\U0001f4da",
	})

	Register(&ToolEntry{
		Name:    "skill_view",
		Toolset: "skills",
		Schema: map[string]any{
			"name":        "skill_view",
			"description": "Read the contents of a specific skill's SKILL.md file.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Name of the skill to view",
					},
				},
				"required": []string{"name"},
			},
		},
		Handler: handleSkillView,
		Emoji:   "\U0001f4d6",
	})

	Register(&ToolEntry{
		Name:    "skill_manage",
		Toolset: "skills",
		Schema: map[string]any{
			"name":        "skill_manage",
			"description": "Create, edit, or delete a skill. Skills are stored as directories with a SKILL.md file.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "Action to perform",
						"enum":        []string{"create", "edit", "delete"},
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Skill name (used as directory name)",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "SKILL.md content (for create/edit)",
					},
				},
				"required": []string{"action", "name"},
			},
		},
		Handler: handleSkillManage,
		Emoji:   "\u2699\ufe0f",
	})
}

func getSkillsDir() string {
	return filepath.Join(config.HermesHome(), "skills")
}

func handleSkillsList(args map[string]any, ctx *ToolContext) string {
	skillsDir := getSkillsDir()
	os.MkdirAll(skillsDir, 0755)

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Cannot read skills directory: %v", err)})
	}

	var skills []map[string]any
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillMD := filepath.Join(skillsDir, skillName, "SKILL.md")

		skill := map[string]any{
			"name":   skillName,
			"path":   filepath.Join(skillsDir, skillName),
			"has_md": fileExists(skillMD),
		}

		// Try to read first line as description
		if data, err := os.ReadFile(skillMD); err == nil {
			lines := strings.SplitN(string(data), "\n", 3)
			for _, line := range lines {
				trimmed := strings.TrimSpace(strings.TrimPrefix(line, "#"))
				if trimmed != "" && !strings.HasPrefix(line, "#") {
					skill["description"] = truncateOutput(trimmed, 200)
					break
				}
			}
			skill["size"] = len(data)
		}

		skills = append(skills, skill)
	}

	return toJSON(map[string]any{
		"skills":     skills,
		"count":      len(skills),
		"skills_dir": skillsDir,
	})
}

func handleSkillView(args map[string]any, ctx *ToolContext) string {
	name, _ := args["name"].(string)
	if name == "" {
		return `{"error":"name is required"}`
	}

	skillMD := filepath.Join(getSkillsDir(), name, "SKILL.md")
	data, err := os.ReadFile(skillMD)
	if err != nil {
		return toJSON(map[string]any{
			"error": fmt.Sprintf("Cannot read skill '%s': %v", name, err),
			"hint":  "Use skills_list to see available skills",
		})
	}

	return toJSON(map[string]any{
		"name":    name,
		"content": string(data),
		"path":    skillMD,
	})
}

func handleSkillManage(args map[string]any, ctx *ToolContext) string {
	action, _ := args["action"].(string)
	name, _ := args["name"].(string)
	content, _ := args["content"].(string)

	if name == "" {
		return `{"error":"name is required"}`
	}

	// Sanitize skill name
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "..", "")

	skillDir := filepath.Join(getSkillsDir(), name)
	skillMD := filepath.Join(skillDir, "SKILL.md")

	switch action {
	case "create":
		if content == "" {
			return `{"error":"content is required for create"}`
		}
		if fileExists(skillDir) {
			return toJSON(map[string]any{
				"error": fmt.Sprintf("Skill '%s' already exists. Use 'edit' to modify.", name),
			})
		}
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return toJSON(map[string]any{"error": fmt.Sprintf("Cannot create skill directory: %v", err)})
		}
		if err := os.WriteFile(skillMD, []byte(content), 0644); err != nil {
			return toJSON(map[string]any{"error": fmt.Sprintf("Cannot write SKILL.md: %v", err)})
		}
		return toJSON(map[string]any{
			"success": true,
			"name":    name,
			"path":    skillMD,
			"message": fmt.Sprintf("Skill '%s' created successfully", name),
		})

	case "edit":
		if content == "" {
			return `{"error":"content is required for edit"}`
		}
		if !fileExists(skillDir) {
			return toJSON(map[string]any{
				"error": fmt.Sprintf("Skill '%s' does not exist. Use 'create' first.", name),
			})
		}
		if err := os.WriteFile(skillMD, []byte(content), 0644); err != nil {
			return toJSON(map[string]any{"error": fmt.Sprintf("Cannot write SKILL.md: %v", err)})
		}
		return toJSON(map[string]any{
			"success": true,
			"name":    name,
			"path":    skillMD,
			"message": fmt.Sprintf("Skill '%s' updated successfully", name),
		})

	case "delete":
		if !fileExists(skillDir) {
			return toJSON(map[string]any{
				"error": fmt.Sprintf("Skill '%s' does not exist", name),
			})
		}
		if err := os.RemoveAll(skillDir); err != nil {
			return toJSON(map[string]any{"error": fmt.Sprintf("Cannot delete skill: %v", err)})
		}
		return toJSON(map[string]any{
			"success": true,
			"name":    name,
			"message": fmt.Sprintf("Skill '%s' deleted successfully", name),
		})

	default:
		return `{"error":"Invalid action. Use: create, edit, delete"}`
	}
}
