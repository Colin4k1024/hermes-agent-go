package cron

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJobStore(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	os.MkdirAll(filepath.Join(tmpDir, "cron"), 0755)

	store := NewJobStore()

	// Add job
	job := &Job{
		Schedule: "*/5 * * * *",
		Prompt:   "Check server status",
		Enabled:  true,
	}
	err := store.Add(job)
	if err != nil {
		t.Fatalf("Add job failed: %v", err)
	}
	if job.ID == "" {
		t.Error("Expected non-empty job ID")
	}

	// List jobs
	jobs := store.List()
	if len(jobs) != 1 {
		t.Errorf("Expected 1 job, got %d", len(jobs))
	}

	// Get job
	got := store.Get(job.ID)
	if got == nil {
		t.Fatal("Expected to find job by ID")
	}
	if got.Prompt != "Check server status" {
		t.Errorf("Expected prompt match, got '%s'", got.Prompt)
	}

	// Pause
	err = store.Pause(job.ID)
	if err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	got = store.Get(job.ID)
	if got.Enabled {
		t.Error("Expected job to be disabled after pause")
	}

	// Resume
	err = store.Resume(job.ID)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	got = store.Get(job.ID)
	if !got.Enabled {
		t.Error("Expected job to be enabled after resume")
	}

	// Remove
	err = store.Remove(job.ID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	jobs = store.List()
	if len(jobs) != 0 {
		t.Errorf("Expected 0 jobs after remove, got %d", len(jobs))
	}
}

func TestJobStoreNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "cron"), 0755)

	store := NewJobStore()

	got := store.Get("nonexistent-id")
	if got != nil {
		t.Error("Expected nil for nonexistent job")
	}

	err := store.Pause("nonexistent-id")
	if err == nil {
		t.Error("Expected error for pausing nonexistent job")
	}

	err = store.Remove("nonexistent-id")
	if err == nil {
		t.Error("Expected error for removing nonexistent job")
	}
}
