package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// CronJob represents a scheduled job.
type CronJob struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastRun   *time.Time `json:"last_run,omitempty"`
	RunCount  int       `json:"run_count"`
}

func init() {
	Register(&ToolEntry{
		Name:    "cronjob",
		Toolset: "cronjob",
		Schema: map[string]any{
			"name":        "cronjob",
			"description": "Manage scheduled tasks (cron jobs). Create, list, update, or delete recurring jobs.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "Action to perform",
						"enum":        []string{"create", "list", "get", "update", "delete", "enable", "disable"},
					},
					"id": map[string]any{
						"type":        "string",
						"description": "Job ID (for get, update, delete, enable, disable)",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Human-readable job name",
					},
					"schedule": map[string]any{
						"type":        "string",
						"description": "Cron expression (e.g., '0 9 * * *' for daily at 9am, '*/5 * * * *' for every 5 min)",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "Command or task to execute on schedule",
					},
				},
				"required": []string{"action"},
			},
		},
		Handler: handleCronjob,
		Emoji:   "\u23f0",
	})
}

func getCronDir() string {
	return filepath.Join(config.HermesHome(), "cron")
}

func getCronJobPath(id string) string {
	return filepath.Join(getCronDir(), id+".json")
}

func loadCronJob(id string) (*CronJob, error) {
	data, err := os.ReadFile(getCronJobPath(id))
	if err != nil {
		return nil, err
	}
	var job CronJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

func saveCronJob(job *CronJob) error {
	cronDir := getCronDir()
	os.MkdirAll(cronDir, 0755)

	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getCronJobPath(job.ID), data, 0644)
}

func listCronJobs() ([]CronJob, error) {
	cronDir := getCronDir()
	os.MkdirAll(cronDir, 0755)

	entries, err := os.ReadDir(cronDir)
	if err != nil {
		return nil, err
	}

	var jobs []CronJob
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cronDir, entry.Name()))
		if err != nil {
			continue
		}
		var job CronJob
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func handleCronjob(args map[string]any, ctx *ToolContext) string {
	action, _ := args["action"].(string)

	switch action {
	case "create":
		return createCronJob(args)
	case "list":
		return listCronJobsAction()
	case "get":
		return getCronJobAction(args)
	case "update":
		return updateCronJob(args)
	case "delete":
		return deleteCronJob(args)
	case "enable":
		return toggleCronJob(args, true)
	case "disable":
		return toggleCronJob(args, false)
	default:
		return `{"error":"Invalid action. Use: create, list, get, update, delete, enable, disable"}`
	}
}

func createCronJob(args map[string]any) string {
	name, _ := args["name"].(string)
	schedule, _ := args["schedule"].(string)
	command, _ := args["command"].(string)

	if name == "" || schedule == "" || command == "" {
		return `{"error":"name, schedule, and command are required for create"}`
	}

	id := fmt.Sprintf("job_%d", time.Now().UnixMilli())
	now := time.Now()

	job := &CronJob{
		ID:        id,
		Name:      name,
		Schedule:  schedule,
		Command:   command,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := saveCronJob(job); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to save job: %v", err)})
	}

	return toJSON(map[string]any{
		"success": true,
		"job":     job,
		"message": fmt.Sprintf("Cron job '%s' created with ID %s", name, id),
	})
}

func listCronJobsAction() string {
	jobs, err := listCronJobs()
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to list jobs: %v", err)})
	}

	if jobs == nil {
		jobs = []CronJob{}
	}

	return toJSON(map[string]any{
		"jobs":  jobs,
		"count": len(jobs),
	})
}

func getCronJobAction(args map[string]any) string {
	id, _ := args["id"].(string)
	if id == "" {
		return `{"error":"id is required for get"}`
	}

	job, err := loadCronJob(id)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Job not found: %s", id)})
	}

	return toJSON(map[string]any{"job": job})
}

func updateCronJob(args map[string]any) string {
	id, _ := args["id"].(string)
	if id == "" {
		return `{"error":"id is required for update"}`
	}

	job, err := loadCronJob(id)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Job not found: %s", id)})
	}

	if name, ok := args["name"].(string); ok && name != "" {
		job.Name = name
	}
	if schedule, ok := args["schedule"].(string); ok && schedule != "" {
		job.Schedule = schedule
	}
	if command, ok := args["command"].(string); ok && command != "" {
		job.Command = command
	}
	job.UpdatedAt = time.Now()

	if err := saveCronJob(job); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to update job: %v", err)})
	}

	return toJSON(map[string]any{
		"success": true,
		"job":     job,
		"message": fmt.Sprintf("Job '%s' updated", job.Name),
	})
}

func deleteCronJob(args map[string]any) string {
	id, _ := args["id"].(string)
	if id == "" {
		return `{"error":"id is required for delete"}`
	}

	path := getCronJobPath(id)
	if !fileExists(path) {
		return toJSON(map[string]any{"error": fmt.Sprintf("Job not found: %s", id)})
	}

	if err := os.Remove(path); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to delete job: %v", err)})
	}

	return toJSON(map[string]any{
		"success": true,
		"id":      id,
		"message": fmt.Sprintf("Job %s deleted", id),
	})
}

func toggleCronJob(args map[string]any, enabled bool) string {
	id, _ := args["id"].(string)
	if id == "" {
		return `{"error":"id is required"}`
	}

	job, err := loadCronJob(id)
	if err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Job not found: %s", id)})
	}

	job.Enabled = enabled
	job.UpdatedAt = time.Now()

	if err := saveCronJob(job); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to update job: %v", err)})
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}

	return toJSON(map[string]any{
		"success": true,
		"job":     job,
		"message": fmt.Sprintf("Job '%s' %s", job.Name, status),
	})
}
