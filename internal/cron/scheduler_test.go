package cron

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSchedulerStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "cron"), 0755)

	s := NewScheduler()
	err := s.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer s.Stop()
}

func TestSchedulerAddRemoveJob(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "cron"), 0755)

	s := NewScheduler()
	s.Start()
	defer s.Stop()

	job := &Job{
		Schedule: "*/5 * * * *",
		Prompt:   "Test job",
		Enabled:  true,
	}

	err := s.AddJob(job)
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	jobs := s.ListJobs()
	if len(jobs) != 1 {
		t.Errorf("Expected 1 job, got %d", len(jobs))
	}

	err = s.RemoveJob(job.ID)
	if err != nil {
		t.Fatalf("RemoveJob failed: %v", err)
	}

	jobs = s.ListJobs()
	if len(jobs) != 0 {
		t.Errorf("Expected 0 jobs after remove, got %d", len(jobs))
	}
}

func TestSchedulerPauseResume(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")
	os.MkdirAll(filepath.Join(tmpDir, "cron"), 0755)

	s := NewScheduler()
	s.Start()
	defer s.Stop()

	job := &Job{
		Schedule: "0 9 * * *",
		Prompt:   "Morning check",
		Enabled:  true,
	}
	s.AddJob(job)

	err := s.PauseJob(job.ID)
	if err != nil {
		t.Fatalf("PauseJob failed: %v", err)
	}

	j := s.store.Get(job.ID)
	if j.Enabled {
		t.Error("Expected job to be disabled after pause")
	}

	err = s.ResumeJob(job.ID)
	if err != nil {
		t.Fatalf("ResumeJob failed: %v", err)
	}

	j = s.store.Get(job.ID)
	if !j.Enabled {
		t.Error("Expected job to be enabled after resume")
	}
}
