package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model == "" {
		t.Error("Expected non-empty default model")
	}
	if cfg.MaxIterations <= 0 {
		t.Error("Expected positive max iterations")
	}
	if cfg.Display.Skin != "default" {
		t.Errorf("Expected default skin, got %s", cfg.Display.Skin)
	}
	if cfg.Terminal.DefaultTimeout <= 0 {
		t.Error("Expected positive default timeout")
	}
}

func TestLoadWithEnv(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	// Reset global config
	Reload()

	cfg := Load()
	if cfg == nil {
		t.Fatal("Load returned nil")
	}
	if cfg.Model == "" {
		t.Error("Expected default model to be set")
	}
}

func TestLoadWithConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	configContent := `model: "test-model"
max_iterations: 50
display:
  skin: "mono"
`
	os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0644)

	cfg := Reload()
	if cfg.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got '%s'", cfg.Model)
	}
	if cfg.MaxIterations != 50 {
		t.Errorf("Expected max_iterations 50, got %d", cfg.MaxIterations)
	}
	if cfg.Display.Skin != "mono" {
		t.Errorf("Expected skin 'mono', got '%s'", cfg.Display.Skin)
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	cfg := DefaultConfig()
	cfg.Model = "saved-model"

	err := Save(cfg)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(tmpDir, "config.yaml"))
	if err != nil {
		t.Fatalf("Read saved config: %v", err)
	}
	if len(data) == 0 {
		t.Error("Saved config file is empty")
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_HERMES_VAR", "hello")
	defer os.Unsetenv("TEST_HERMES_VAR")

	if GetEnv("TEST_HERMES_VAR", "default") != "hello" {
		t.Error("Expected 'hello' from env")
	}
	if GetEnv("NONEXISTENT_VAR_XYZ", "fallback") != "fallback" {
		t.Error("Expected fallback value")
	}
}

func TestHasEnv(t *testing.T) {
	os.Setenv("TEST_HERMES_EXISTS", "1")
	defer os.Unsetenv("TEST_HERMES_EXISTS")

	if !HasEnv("TEST_HERMES_EXISTS") {
		t.Error("Expected HasEnv to return true")
	}
	if HasEnv("NONEXISTENT_VAR_XYZ") {
		t.Error("Expected HasEnv to return false")
	}
}

func TestEnsureHermesHome(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", filepath.Join(tmpDir, "hermes-test"))
	defer os.Unsetenv("HERMES_HOME")

	err := EnsureHermesHome()
	if err != nil {
		t.Fatalf("EnsureHermesHome failed: %v", err)
	}

	// Check directories created
	expectedDirs := []string{"sessions", "logs", "memories", "skills", "cron", "cache"}
	for _, dir := range expectedDirs {
		path := filepath.Join(tmpDir, "hermes-test", dir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
	}
}
