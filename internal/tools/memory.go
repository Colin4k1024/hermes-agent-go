package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

func init() {
	Register(&ToolEntry{
		Name:    "memory",
		Toolset: "memory",
		Schema: map[string]any{
			"name":        "memory",
			"description": "Manage persistent memory across sessions. Read, save, or update memory notes and user profile.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "Action to perform",
						"enum":        []string{"read", "save", "delete", "read_user", "save_user"},
					},
					"key": map[string]any{
						"type":        "string",
						"description": "Memory key/title for save or delete",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Content to save",
					},
				},
				"required": []string{"action"},
			},
		},
		Handler: handleMemory,
		Emoji:   "🧠",
	})
}

func handleMemory(args map[string]any, ctx *ToolContext) string {
	action, _ := args["action"].(string)
	key, _ := args["key"].(string)
	content, _ := args["content"].(string)

	memoriesDir := filepath.Join(config.HermesHome(), "memories")
	os.MkdirAll(memoriesDir, 0755)

	switch action {
	case "read":
		return readMemory(memoriesDir)
	case "save":
		return saveMemory(memoriesDir, key, content)
	case "delete":
		return deleteMemory(memoriesDir, key)
	case "read_user":
		return readUserProfile(memoriesDir)
	case "save_user":
		return saveUserProfile(memoriesDir, content)
	default:
		return `{"error":"Invalid action. Use: read, save, delete, read_user, save_user"}`
	}
}

func readMemory(dir string) string {
	memoryPath := filepath.Join(dir, "MEMORY.md")
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		return toJSON(map[string]any{
			"content": "",
			"message": "No memory file found. Use save to create one.",
		})
	}
	return toJSON(map[string]any{
		"content": string(data),
		"path":    memoryPath,
	})
}

func saveMemory(dir, key, content string) string {
	if key == "" || content == "" {
		return `{"error":"Both key and content are required for save"}`
	}

	memoryPath := filepath.Join(dir, "MEMORY.md")
	existing, _ := os.ReadFile(memoryPath)

	timestamp := time.Now().Format("2006-01-02 15:04")
	entry := fmt.Sprintf("\n## %s\n*Saved: %s*\n\n%s\n", key, timestamp, content)

	// Check if key exists and update
	existingStr := string(existing)
	marker := fmt.Sprintf("## %s\n", key)
	if idx := strings.Index(existingStr, marker); idx != -1 {
		// Find next section or end
		nextIdx := strings.Index(existingStr[idx+len(marker):], "\n## ")
		if nextIdx != -1 {
			existingStr = existingStr[:idx] + entry[1:] + existingStr[idx+len(marker)+nextIdx:]
		} else {
			existingStr = existingStr[:idx] + entry[1:]
		}
	} else {
		existingStr += entry
	}

	if err := os.WriteFile(memoryPath, []byte(existingStr), 0644); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to save: %v", err)})
	}

	return toJSON(map[string]any{
		"success": true,
		"key":     key,
		"message": "Memory saved successfully",
	})
}

func deleteMemory(dir, key string) string {
	if key == "" {
		return `{"error":"key is required for delete"}`
	}

	memoryPath := filepath.Join(dir, "MEMORY.md")
	data, err := os.ReadFile(memoryPath)
	if err != nil {
		return `{"error":"No memory file found"}`
	}

	content := string(data)
	marker := fmt.Sprintf("## %s\n", key)
	idx := strings.Index(content, marker)
	if idx == -1 {
		return toJSON(map[string]any{"error": fmt.Sprintf("Memory key '%s' not found", key)})
	}

	nextIdx := strings.Index(content[idx+len(marker):], "\n## ")
	if nextIdx != -1 {
		content = content[:idx] + content[idx+len(marker)+nextIdx+1:]
	} else {
		content = content[:idx]
	}

	os.WriteFile(memoryPath, []byte(content), 0644)
	return toJSON(map[string]any{
		"success": true,
		"key":     key,
		"message": "Memory deleted",
	})
}

func readUserProfile(dir string) string {
	userPath := filepath.Join(dir, "USER.md")
	data, err := os.ReadFile(userPath)
	if err != nil {
		return toJSON(map[string]any{
			"content": "",
			"message": "No user profile found.",
		})
	}
	return toJSON(map[string]any{
		"content": string(data),
		"path":    userPath,
	})
}

func saveUserProfile(dir, content string) string {
	if content == "" {
		return `{"error":"content is required"}`
	}

	userPath := filepath.Join(dir, "USER.md")
	if err := os.WriteFile(userPath, []byte(content), 0644); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to save: %v", err)})
	}

	return toJSON(map[string]any{
		"success": true,
		"message": "User profile saved",
	})
}
