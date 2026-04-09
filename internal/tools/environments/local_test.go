package environments

import (
	"runtime"
	"strings"
	"testing"
)

func TestLocalEnvironmentAvailable(t *testing.T) {
	env := &LocalEnvironment{}
	if !env.IsAvailable() {
		t.Error("Local environment should always be available")
	}
}

func TestLocalEnvironmentName(t *testing.T) {
	env := &LocalEnvironment{}
	if env.Name() != "local" {
		t.Errorf("Expected 'local', got '%s'", env.Name())
	}
}

func TestLocalEnvironmentExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	env := &LocalEnvironment{}
	stdout, stderr, exitCode, err := env.Execute("echo test-output", 10)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "test-output") {
		t.Errorf("Expected stdout to contain 'test-output', got '%s'", stdout)
	}
	_ = stderr
}

func TestLocalEnvironmentExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	env := &LocalEnvironment{}
	_, _, exitCode, _ := env.Execute("exit 42", 10)
	if exitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", exitCode)
	}
}

func TestGetEnvironmentLocal(t *testing.T) {
	env, err := GetEnvironment("local", nil)
	if err != nil {
		t.Fatalf("GetEnvironment failed: %v", err)
	}
	if env == nil {
		t.Error("Expected local environment to be registered")
	}
}

func TestGetEnvironmentUnknown(t *testing.T) {
	// Unknown environments fall back to local
	env, err := GetEnvironment("nonexistent", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if env == nil {
		t.Error("Expected fallback to local environment")
	}
	if env.Name() != "local" {
		t.Errorf("Expected fallback name 'local', got '%s'", env.Name())
	}
}
