package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// TodoItem represents a single todo item.
type TodoItem struct {
	ID        int       `json:"id"`
	Task      string    `json:"task"`
	Status    string    `json:"status"` // "pending", "in_progress", "done"
	Priority  string    `json:"priority,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
}

func init() {
	Register(&ToolEntry{
		Name:    "todo",
		Toolset: "todo",
		Schema: map[string]any{
			"name":        "todo",
			"description": "Manage a task/todo list for planning and tracking multi-step work.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "Action to perform",
						"enum":        []string{"add", "list", "update", "remove", "clear"},
					},
					"task": map[string]any{
						"type":        "string",
						"description": "Task description (for add)",
					},
					"task_id": map[string]any{
						"type":        "integer",
						"description": "Task ID (for update, remove)",
					},
					"status": map[string]any{
						"type":        "string",
						"description": "New status (for update)",
						"enum":        []string{"pending", "in_progress", "done"},
					},
					"priority": map[string]any{
						"type":        "string",
						"description": "Priority level",
						"enum":        []string{"high", "medium", "low"},
					},
				},
				"required": []string{"action"},
			},
		},
		Handler: handleTodo,
		Emoji:   "📋",
	})
}

func getTodoPath() string {
	return filepath.Join(config.HermesHome(), "cache", "todos.json")
}

func loadTodos() []TodoItem {
	data, err := os.ReadFile(getTodoPath())
	if err != nil {
		return nil
	}
	var todos []TodoItem
	json.Unmarshal(data, &todos)
	return todos
}

func saveTodos(todos []TodoItem) error {
	dir := filepath.Dir(getTodoPath())
	os.MkdirAll(dir, 0755)
	data, _ := json.MarshalIndent(todos, "", "  ")
	return os.WriteFile(getTodoPath(), data, 0644)
}

func handleTodo(args map[string]any, ctx *ToolContext) string {
	action, _ := args["action"].(string)

	switch action {
	case "add":
		task, _ := args["task"].(string)
		if task == "" {
			return `{"error":"task is required for add"}`
		}
		priority, _ := args["priority"].(string)
		if priority == "" {
			priority = "medium"
		}

		todos := loadTodos()
		maxID := 0
		for _, t := range todos {
			if t.ID > maxID {
				maxID = t.ID
			}
		}

		newTodo := TodoItem{
			ID:        maxID + 1,
			Task:      task,
			Status:    "pending",
			Priority:  priority,
			CreatedAt: time.Now(),
		}
		todos = append(todos, newTodo)
		saveTodos(todos)

		return toJSON(map[string]any{
			"success": true,
			"todo":    newTodo,
			"message": fmt.Sprintf("Added task #%d: %s", newTodo.ID, task),
		})

	case "list":
		todos := loadTodos()
		return toJSON(map[string]any{
			"todos": todos,
			"count": len(todos),
		})

	case "update":
		taskID := 0
		if id, ok := args["task_id"].(float64); ok {
			taskID = int(id)
		}
		if taskID == 0 {
			return `{"error":"task_id is required for update"}`
		}

		status, _ := args["status"].(string)
		todos := loadTodos()
		for i, t := range todos {
			if t.ID == taskID {
				if status != "" {
					todos[i].Status = status
					if status == "done" {
						now := time.Now()
						todos[i].DoneAt = &now
					}
				}
				saveTodos(todos)
				return toJSON(map[string]any{
					"success": true,
					"todo":    todos[i],
					"message": fmt.Sprintf("Updated task #%d", taskID),
				})
			}
		}
		return toJSON(map[string]any{"error": fmt.Sprintf("Task #%d not found", taskID)})

	case "remove":
		taskID := 0
		if id, ok := args["task_id"].(float64); ok {
			taskID = int(id)
		}
		if taskID == 0 {
			return `{"error":"task_id is required for remove"}`
		}

		todos := loadTodos()
		var newTodos []TodoItem
		found := false
		for _, t := range todos {
			if t.ID == taskID {
				found = true
				continue
			}
			newTodos = append(newTodos, t)
		}
		if !found {
			return toJSON(map[string]any{"error": fmt.Sprintf("Task #%d not found", taskID)})
		}
		saveTodos(newTodos)
		return toJSON(map[string]any{
			"success": true,
			"message": fmt.Sprintf("Removed task #%d", taskID),
		})

	case "clear":
		saveTodos(nil)
		return toJSON(map[string]any{
			"success": true,
			"message": "All todos cleared",
		})

	default:
		return `{"error":"Invalid action. Use: add, list, update, remove, clear"}`
	}
}
