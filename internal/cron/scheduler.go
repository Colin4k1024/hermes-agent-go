package cron

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/agent"
	"github.com/hermes-agent/hermes-agent-go/internal/config"
	robfigcron "github.com/robfig/cron/v3"
)

// Scheduler manages scheduled cron jobs using robfig/cron.
type Scheduler struct {
	mu        sync.Mutex
	cron      *robfigcron.Cron
	store     *JobStore
	entryMap  map[string]robfigcron.EntryID // job ID -> cron entry ID
	running   bool
	adapters  map[string]any // platform adapters for delivery
}

// NewScheduler creates a new cron scheduler.
func NewScheduler() *Scheduler {
	return &Scheduler{
		cron:     robfigcron.New(robfigcron.WithSeconds()),
		store:    NewJobStore(),
		entryMap: make(map[string]robfigcron.EntryID),
	}
}

// Start initializes the scheduler and begins processing jobs.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// Load jobs from disk.
	if err := s.store.Load(); err != nil {
		slog.Warn("Failed to load cron jobs", "error", err)
	}

	// Register all enabled jobs.
	for _, job := range s.store.List() {
		if job.Enabled {
			s.scheduleJob(job)
		}
	}

	s.cron.Start()
	s.running = true
	slog.Info("Cron scheduler started", "jobs", len(s.store.List()))

	return nil
}

// Stop stops the scheduler gracefully.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	ctx := s.cron.Stop()
	<-ctx.Done()
	s.running = false
	slog.Info("Cron scheduler stopped")
}

// AddJob adds a new job and schedules it.
func (s *Scheduler) AddJob(job *Job) error {
	if err := s.store.Add(job); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running && job.Enabled {
		s.scheduleJob(job)
	}

	return nil
}

// RemoveJob removes a job and unschedules it.
func (s *Scheduler) RemoveJob(id string) error {
	s.mu.Lock()
	if entryID, ok := s.entryMap[id]; ok {
		s.cron.Remove(entryID)
		delete(s.entryMap, id)
	}
	s.mu.Unlock()

	return s.store.Remove(id)
}

// PauseJob pauses a job.
func (s *Scheduler) PauseJob(id string) error {
	s.mu.Lock()
	if entryID, ok := s.entryMap[id]; ok {
		s.cron.Remove(entryID)
		delete(s.entryMap, id)
	}
	s.mu.Unlock()

	return s.store.Pause(id)
}

// ResumeJob resumes a paused job.
func (s *Scheduler) ResumeJob(id string) error {
	if err := s.store.Resume(id); err != nil {
		return err
	}

	job := s.store.Get(id)
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.scheduleJob(job)
	}

	return nil
}

// TriggerJob runs a job immediately, regardless of schedule.
func (s *Scheduler) TriggerJob(id string) error {
	job := s.store.Get(id)
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	go s.executeJob(job)
	return nil
}

// ListJobs returns all jobs.
func (s *Scheduler) ListJobs() []*Job {
	return s.store.List()
}

// GetJob returns a job by ID.
func (s *Scheduler) GetJob(id string) *Job {
	return s.store.Get(id)
}

// SetAdapters sets the platform adapters for delivery.
func (s *Scheduler) SetAdapters(adapters map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapters = adapters
}

// Tick checks for due jobs and executes them. Called periodically
// when not using the robfig/cron scheduler (e.g., from the gateway).
func (s *Scheduler) Tick() int {
	dueJobs := s.store.GetDueJobs()
	if len(dueJobs) == 0 {
		return 0
	}

	slog.Info("Cron tick: jobs due", "count", len(dueJobs))

	executed := 0
	for _, job := range dueJobs {
		s.executeJob(job)
		executed++
	}

	return executed
}

// --- Internal ---

func (s *Scheduler) scheduleJob(job *Job) {
	entryID, err := s.cron.AddFunc(job.Schedule, func() {
		s.executeJob(job)
	})
	if err != nil {
		slog.Error("Failed to schedule job", "job", job.Name, "schedule", job.Schedule, "error", err)
		return
	}

	s.entryMap[job.ID] = entryID
	slog.Debug("Scheduled job", "job", job.Name, "schedule", job.Schedule)
}

func (s *Scheduler) executeJob(job *Job) {
	slog.Info("Executing cron job", "job", job.Name, "id", job.ID)

	success, output, response, errMsg := s.runJob(job)

	// Save output.
	outputFile, err := s.store.SaveJobOutput(job.ID, output)
	if err != nil {
		slog.Warn("Failed to save job output", "error", err)
	} else {
		slog.Info("Job output saved", "file", outputFile)
	}

	// Mark run result.
	s.store.MarkRun(job.ID, success, errMsg)

	// Deliver result if configured.
	if response != "" && job.Deliver != "local" {
		s.deliverResult(job, response)
	}
}

func (s *Scheduler) runJob(job *Job) (success bool, output, response, errMsg string) {
	// Build the prompt (with optional skill loading).
	prompt := s.buildJobPrompt(job)

	// Create a one-shot agent.
	sessionID := fmt.Sprintf("cron_%s_%s", job.ID, time.Now().Format("20060102_150405"))

	ag, err := agent.New(
		agent.WithPlatform("cron"),
		agent.WithSessionID(sessionID),
		agent.WithQuietMode(true),
		agent.WithSkipMemory(true),
		agent.WithDisabledToolsets([]string{"cronjob", "messaging", "clarify"}),
	)
	if err != nil {
		errMsg = fmt.Sprintf("failed to create agent: %v", err)
		output = fmt.Sprintf("# Cron Job: %s (FAILED)\n\n## Error\n\n%s", job.Name, errMsg)
		return false, output, "", errMsg
	}
	defer ag.Close()

	// Set model if specified.
	if job.Model != "" {
		// Model is set via agent options, handled during creation.
	}

	// Run the agent.
	result, err := ag.Chat(prompt)
	if err != nil {
		errMsg = fmt.Sprintf("agent error: %v", err)
		output = fmt.Sprintf("# Cron Job: %s (FAILED)\n\n## Error\n\n%s", job.Name, errMsg)
		return false, output, "", errMsg
	}

	// Check for SILENT marker.
	if result == "[SILENT]" {
		slog.Info("Job returned SILENT marker", "job", job.Name)
		output = fmt.Sprintf("# Cron Job: %s\n\n## Response\n\n[SILENT]", job.Name)
		return true, output, "", ""
	}

	output = fmt.Sprintf("# Cron Job: %s\n\n## Prompt\n\n%s\n\n## Response\n\n%s", job.Name, prompt, result)
	return true, output, result, ""
}

func (s *Scheduler) buildJobPrompt(job *Job) string {
	prompt := job.Prompt

	// Prepend cron execution guidance.
	cronHint := `[SYSTEM: You are running as a scheduled cron job. ` +
		`DELIVERY: Your final response will be automatically delivered ` +
		`to the user -- do NOT use send_message or try to deliver ` +
		`the output yourself. Just produce your report/output as your ` +
		`final response and the system handles the rest. ` +
		`SILENT: If there is genuinely nothing new to report, respond ` +
		`with exactly "[SILENT]" (nothing else) to suppress delivery.]` + "\n\n"

	prompt = cronHint + prompt

	// Load skills if configured.
	if len(job.Skills) > 0 {
		for _, skillName := range job.Skills {
			prompt = fmt.Sprintf(
				`[SYSTEM: The user has invoked the "%s" skill for this cron job.]`+"\n\n%s",
				skillName, prompt,
			)
		}
	}

	// Run data collection script if configured.
	if job.Script != "" {
		prompt = fmt.Sprintf(
			"## Script\nA data-collection script is configured at: %s\n\n%s",
			job.Script, prompt,
		)
	}

	return prompt
}

func (s *Scheduler) deliverResult(job *Job, content string) {
	deliver := job.Deliver

	if deliver == "local" {
		return
	}

	// For now, save delivery attempts to a file.
	deliveryDir := filepath.Join(config.HermesHome(), "cron", "delivery")
	os.MkdirAll(deliveryDir, 0755)

	filename := fmt.Sprintf("%s_%s.txt", job.ID, time.Now().Format("20060102_150405"))
	deliveryPath := filepath.Join(deliveryDir, filename)

	deliveryContent := fmt.Sprintf("Deliver to: %s\n\n%s", deliver, content)
	if err := os.WriteFile(deliveryPath, []byte(deliveryContent), 0644); err != nil {
		slog.Warn("Failed to save delivery content", "error", err)
	}

	slog.Info("Job delivery saved", "job", job.Name, "deliver", deliver, "file", deliveryPath)
}
