// Package cron implements the scheduled job system for Hermes Agent.
package cron

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// Job represents a scheduled cron job.
type Job struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Schedule        string         `json:"schedule"`         // Cron expression or interval
	ScheduleDisplay string         `json:"schedule_display"` // Human-readable schedule
	Prompt          string         `json:"prompt"`
	Model           string         `json:"model,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	BaseURL         string         `json:"base_url,omitempty"`
	Skills          []string       `json:"skills,omitempty"`
	Script          string         `json:"script,omitempty"` // Pre-run data collection script
	Deliver         string         `json:"deliver"`          // "local", "origin", "platform:chat_id"
	Origin          *JobOrigin     `json:"origin,omitempty"` // Where the job was created
	Enabled         bool           `json:"enabled"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	NextRunAt       *time.Time     `json:"next_run_at,omitempty"`
	LastRunAt       *time.Time     `json:"last_run_at,omitempty"`
	LastRunSuccess  *bool          `json:"last_run_success,omitempty"`
	LastRunError    string         `json:"last_run_error,omitempty"`
	RunCount        int            `json:"run_count"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// JobOrigin records where a cron job was created.
type JobOrigin struct {
	Platform string `json:"platform"`
	ChatID   string `json:"chat_id"`
	ChatName string `json:"chat_name,omitempty"`
	ThreadID string `json:"thread_id,omitempty"`
}

// JobStore manages persistence of cron jobs.
type JobStore struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	jobsDir string
	loaded  bool
}

// NewJobStore creates a new job store.
func NewJobStore() *JobStore {
	jobsDir := filepath.Join(config.HermesHome(), "cron")
	os.MkdirAll(jobsDir, 0755)

	return &JobStore{
		jobs:    make(map[string]*Job),
		jobsDir: jobsDir,
	}
}

// Load reads jobs from disk.
func (s *JobStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.loaded {
		return nil
	}

	jobsFile := filepath.Join(s.jobsDir, "jobs.json")
	data, err := os.ReadFile(jobsFile)
	if err != nil {
		if os.IsNotExist(err) {
			s.loaded = true
			return nil
		}
		return fmt.Errorf("read jobs file: %w", err)
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return fmt.Errorf("parse jobs file: %w", err)
	}

	for _, job := range jobs {
		s.jobs[job.ID] = job
	}

	s.loaded = true
	slog.Info("Loaded cron jobs", "count", len(s.jobs))
	return nil
}

// Save writes all jobs to disk.
func (s *JobStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var jobs []*Job
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal jobs: %w", err)
	}

	jobsFile := filepath.Join(s.jobsDir, "jobs.json")
	return os.WriteFile(jobsFile, data, 0644)
}

// Add creates a new job.
func (s *JobStore) Add(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job.ID == "" {
		job.ID = uuid.New().String()[:8]
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	job.UpdatedAt = time.Now()
	job.Enabled = true

	if job.Deliver == "" {
		job.Deliver = "local"
	}

	s.jobs[job.ID] = job
	return s.saveUnlocked()
}

// Remove deletes a job.
func (s *JobStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[id]; !ok {
		return fmt.Errorf("job not found: %s", id)
	}

	delete(s.jobs, id)
	return s.saveUnlocked()
}

// Update modifies an existing job.
func (s *JobStore) Update(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[job.ID]; !ok {
		return fmt.Errorf("job not found: %s", job.ID)
	}

	job.UpdatedAt = time.Now()
	s.jobs[job.ID] = job
	return s.saveUnlocked()
}

// Get returns a job by ID.
func (s *JobStore) Get(id string) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[id]
}

// List returns all jobs.
func (s *JobStore) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var jobs []*Job
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// GetDueJobs returns jobs that are due to run.
func (s *JobStore) GetDueJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var due []*Job
	for _, job := range s.jobs {
		if !job.Enabled {
			continue
		}
		if job.NextRunAt != nil && !now.Before(*job.NextRunAt) {
			due = append(due, job)
		}
	}
	return due
}

// MarkRun records a job execution result.
func (s *JobStore) MarkRun(id string, success bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return
	}

	now := time.Now()
	job.LastRunAt = &now
	job.LastRunSuccess = &success
	job.LastRunError = errMsg
	job.RunCount++
	job.UpdatedAt = now

	s.saveUnlocked()
}

// Pause disables a job.
func (s *JobStore) Pause(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job not found: %s", id)
	}

	job.Enabled = false
	job.UpdatedAt = time.Now()
	return s.saveUnlocked()
}

// Resume enables a job.
func (s *JobStore) Resume(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job not found: %s", id)
	}

	job.Enabled = true
	job.UpdatedAt = time.Now()
	return s.saveUnlocked()
}

// SaveJobOutput saves the output of a job run to a file.
func (s *JobStore) SaveJobOutput(jobID, output string) (string, error) {
	outputDir := filepath.Join(s.jobsDir, "output")
	os.MkdirAll(outputDir, 0755)

	filename := fmt.Sprintf("%s_%s.md", jobID, time.Now().Format("20060102_150405"))
	outputPath := filepath.Join(outputDir, filename)

	if err := os.WriteFile(outputPath, []byte(output), 0644); err != nil {
		return "", fmt.Errorf("write output file: %w", err)
	}

	return outputPath, nil
}

func (s *JobStore) saveUnlocked() error {
	var jobs []*Job
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal jobs: %w", err)
	}

	jobsFile := filepath.Join(s.jobsDir, "jobs.json")
	return os.WriteFile(jobsFile, data, 0644)
}
