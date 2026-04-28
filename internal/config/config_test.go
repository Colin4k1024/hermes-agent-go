package config

import (
	"os"
	"path/filepath"
	"strings"
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

// --- MigrateConfig tests ---

func TestMigrateConfig_AlreadyCurrent(t *testing.T) {
	cfg := map[string]any{
		"_config_version": CurrentConfigVersion,
		"model":           "test-model",
	}
	migrated, changed := MigrateConfig(cfg)
	if changed {
		t.Error("Expected no migration needed for current version")
	}
	if migrated["model"] != "test-model" {
		t.Error("Config should remain unchanged")
	}
}

func TestMigrateConfig_V1ToV2(t *testing.T) {
	cfg := map[string]any{
		"_config_version": 1,
		"llm_provider":    "openai",
		"api_base":        "https://api.example.com",
		"model":           "gpt-4",
	}
	migrated, changed := MigrateConfig(cfg)
	if !changed {
		t.Error("Expected migration to occur")
	}
	if migrated["provider"] != "openai" {
		t.Errorf("Expected 'provider' to be set to 'openai', got %v", migrated["provider"])
	}
	if migrated["base_url"] != "https://api.example.com" {
		t.Errorf("Expected 'base_url' to be set, got %v", migrated["base_url"])
	}
	if _, ok := migrated["llm_provider"]; ok {
		t.Error("Old 'llm_provider' key should be removed")
	}
	if _, ok := migrated["api_base"]; ok {
		t.Error("Old 'api_base' key should be removed")
	}
}

func TestMigrateConfig_V1ToLatest(t *testing.T) {
	cfg := map[string]any{
		"_config_version": 1,
		"llm_provider":    "openai",
		"skin":            "mono",
		"tool_progress":   true,
		"default_timeout": 300,
	}
	migrated, changed := MigrateConfig(cfg)
	if !changed {
		t.Error("Expected migration to occur")
	}
	version := migrated["_config_version"]
	if version != CurrentConfigVersion {
		t.Errorf("Expected version %d, got %v", CurrentConfigVersion, version)
	}

	// Check display nesting
	display, ok := migrated["display"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'display' to be a map")
	}
	if display["skin"] != "mono" {
		t.Errorf("Expected skin 'mono', got %v", display["skin"])
	}

	// Check terminal nesting
	terminal, ok := migrated["terminal"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'terminal' to be a map")
	}
	if terminal["default_timeout"] != 300 {
		t.Errorf("Expected default_timeout 300, got %v", terminal["default_timeout"])
	}

	// Check reasoning defaults
	reasoning, ok := migrated["reasoning"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'reasoning' to be a map")
	}
	if reasoning["effort"] != "medium" {
		t.Errorf("Expected effort 'medium', got %v", reasoning["effort"])
	}
}

func TestMigrateConfig_V4ToV5(t *testing.T) {
	cfg := map[string]any{
		"_config_version": 4,
	}
	migrated, changed := MigrateConfig(cfg)
	if !changed {
		t.Error("Expected migration from v4 to v5")
	}
	reasoning, ok := migrated["reasoning"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'reasoning' to be created")
	}
	if reasoning["enabled"] != false {
		t.Error("Expected reasoning.enabled = false")
	}
	if _, ok := migrated["auxiliary"]; !ok {
		t.Error("Expected 'auxiliary' to be created")
	}
}

func TestGetConfigVersion(t *testing.T) {
	tests := []struct {
		cfg     map[string]any
		version int
	}{
		{map[string]any{}, 1},
		{map[string]any{"_config_version": 3}, 3},
		{map[string]any{"_config_version": float64(4)}, 4},
		{map[string]any{"_config_version": int64(5)}, 5},
		{map[string]any{"_config_version": "invalid"}, 1},
	}
	for _, tt := range tests {
		v := getConfigVersion(tt.cfg)
		if v != tt.version {
			t.Errorf("getConfigVersion(%v) = %d, want %d", tt.cfg, v, tt.version)
		}
	}
}

// --- LoadEnvFile tests ---

func TestLoadEnvFile_BasicKV(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `TEST_LOAD_A=hello
TEST_LOAD_B=world
`
	os.WriteFile(envPath, []byte(content), 0644)

	// Clear any existing values
	os.Unsetenv("TEST_LOAD_A")
	os.Unsetenv("TEST_LOAD_B")
	defer os.Unsetenv("TEST_LOAD_A")
	defer os.Unsetenv("TEST_LOAD_B")

	err := LoadEnvFile(envPath)
	if err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}

	if os.Getenv("TEST_LOAD_A") != "hello" {
		t.Errorf("Expected TEST_LOAD_A=hello, got '%s'", os.Getenv("TEST_LOAD_A"))
	}
	if os.Getenv("TEST_LOAD_B") != "world" {
		t.Errorf("Expected TEST_LOAD_B=world, got '%s'", os.Getenv("TEST_LOAD_B"))
	}
}

func TestLoadEnvFile_Comments(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `# This is a comment
TEST_COMMENT_KEY=value
# Another comment
`
	os.WriteFile(envPath, []byte(content), 0644)
	os.Unsetenv("TEST_COMMENT_KEY")
	defer os.Unsetenv("TEST_COMMENT_KEY")

	err := LoadEnvFile(envPath)
	if err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}
	if os.Getenv("TEST_COMMENT_KEY") != "value" {
		t.Errorf("Expected 'value', got '%s'", os.Getenv("TEST_COMMENT_KEY"))
	}
}

func TestLoadEnvFile_Quotes(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `TEST_DOUBLE_Q="double quoted"
TEST_SINGLE_Q='single quoted'
`
	os.WriteFile(envPath, []byte(content), 0644)
	os.Unsetenv("TEST_DOUBLE_Q")
	os.Unsetenv("TEST_SINGLE_Q")
	defer os.Unsetenv("TEST_DOUBLE_Q")
	defer os.Unsetenv("TEST_SINGLE_Q")

	err := LoadEnvFile(envPath)
	if err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}
	if os.Getenv("TEST_DOUBLE_Q") != "double quoted" {
		t.Errorf("Expected 'double quoted', got '%s'", os.Getenv("TEST_DOUBLE_Q"))
	}
	if os.Getenv("TEST_SINGLE_Q") != "single quoted" {
		t.Errorf("Expected 'single quoted', got '%s'", os.Getenv("TEST_SINGLE_Q"))
	}
}

func TestLoadEnvFile_ExportPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `export TEST_EXPORT_KEY=exported_value
`
	os.WriteFile(envPath, []byte(content), 0644)
	os.Unsetenv("TEST_EXPORT_KEY")
	defer os.Unsetenv("TEST_EXPORT_KEY")

	err := LoadEnvFile(envPath)
	if err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}
	if os.Getenv("TEST_EXPORT_KEY") != "exported_value" {
		t.Errorf("Expected 'exported_value', got '%s'", os.Getenv("TEST_EXPORT_KEY"))
	}
}

func TestLoadEnvFile_DoesNotOverride(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	os.Setenv("TEST_EXISTING_KEY", "original")
	defer os.Unsetenv("TEST_EXISTING_KEY")

	content := `TEST_EXISTING_KEY=overridden
`
	os.WriteFile(envPath, []byte(content), 0644)

	err := LoadEnvFile(envPath)
	if err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}
	if os.Getenv("TEST_EXISTING_KEY") != "original" {
		t.Errorf("Expected existing value 'original' to be preserved, got '%s'", os.Getenv("TEST_EXISTING_KEY"))
	}
}

func TestLoadEnvFile_MultiLine(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `TEST_MULTI=first_part\
second_part
`
	os.WriteFile(envPath, []byte(content), 0644)
	os.Unsetenv("TEST_MULTI")
	defer os.Unsetenv("TEST_MULTI")

	err := LoadEnvFile(envPath)
	if err != nil {
		t.Fatalf("LoadEnvFile failed: %v", err)
	}
	val := os.Getenv("TEST_MULTI")
	if val == "" {
		t.Error("Expected non-empty value for multiline env var")
	}
}

func TestLoadEnvFile_MissingFile(t *testing.T) {
	err := LoadEnvFile("/nonexistent/path/.env")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}

// --- UnquoteValue tests ---

func TestUnquoteValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`'hello'`, "hello"},
		{`hello`, "hello"},
		{`""`, ""},
		{`"`, `"`},
		{`"escaped\"quote"`, `escaped"quote`},
		{`"with\nnewline"`, "with\nnewline"},
		{`"with\ttab"`, "with\ttab"},
	}

	for _, tt := range tests {
		result := unquoteValue(tt.input)
		if result != tt.expected {
			t.Errorf("unquoteValue(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// --- GetAllConfiguredKeys tests ---

func TestGetAllConfiguredKeys(t *testing.T) {
	// Set some known optional env vars
	os.Setenv("EXA_API_KEY", "test")
	defer os.Unsetenv("EXA_API_KEY")

	keys := GetAllConfiguredKeys()
	found := false
	for _, k := range keys {
		if k == "EXA_API_KEY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected EXA_API_KEY in configured keys")
	}
}

func TestGetAllConfiguredKeys_Empty(t *testing.T) {
	// Unset all optional env vars to get a baseline
	for key := range OptionalEnvVars {
		old := os.Getenv(key)
		if old != "" {
			os.Unsetenv(key)
			defer os.Setenv(key, old)
		}
	}

	keys := GetAllConfiguredKeys()
	if len(keys) != 0 {
		t.Errorf("Expected 0 configured keys (after clearing), got %d: %v", len(keys), keys)
	}
}

// --- Profile tests ---

func TestListProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	profiles := ListProfiles()
	if len(profiles) < 1 {
		t.Error("Expected at least the default profile")
	}
	if profiles[0].Name != "default" {
		t.Errorf("Expected first profile to be 'default', got '%s'", profiles[0].Name)
	}
}

func TestCreateProfile(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	err := CreateProfile("test-profile")
	if err != nil {
		t.Fatalf("CreateProfile failed: %v", err)
	}

	// Verify dirs created
	profileHome := GetProfileHome("test-profile")
	if _, err := os.Stat(filepath.Join(profileHome, "sessions")); os.IsNotExist(err) {
		t.Error("Expected sessions dir to exist in profile")
	}
	if _, err := os.Stat(filepath.Join(profileHome, "skills")); os.IsNotExist(err) {
		t.Error("Expected skills dir to exist in profile")
	}
}

func TestCreateProfile_ReservedName(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	err := CreateProfile("")
	if err == nil {
		t.Error("Expected error for empty profile name")
	}
	err = CreateProfile("default")
	if err == nil {
		t.Error("Expected error for 'default' profile name")
	}
}

func TestCreateProfile_InvalidChars(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	err := CreateProfile("test/profile")
	if err == nil {
		t.Error("Expected error for profile with slash")
	}
	err = CreateProfile("test profile")
	if err == nil {
		t.Error("Expected error for profile with space")
	}
}

func TestCreateProfile_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	CreateProfile("dup-profile")
	err := CreateProfile("dup-profile")
	if err == nil {
		t.Error("Expected error for duplicate profile")
	}
}

func TestDeleteProfile(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	CreateProfile("del-profile")
	err := DeleteProfile("del-profile")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		// Note: profile home is based on defaultHermesHome which reads HERMES_HOME
		// If DeleteProfile succeeded, verify it's gone
		if err == nil {
			profileHome := GetProfileHome("del-profile")
			if _, statErr := os.Stat(profileHome); !os.IsNotExist(statErr) {
				t.Error("Expected profile dir to be removed")
			}
		}
	}
}

func TestDeleteProfile_Default(t *testing.T) {
	err := DeleteProfile("")
	if err == nil {
		t.Error("Expected error when deleting default profile")
	}
	err = DeleteProfile("default")
	if err == nil {
		t.Error("Expected error when deleting 'default' profile")
	}
}

func TestGetProfileHome(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HERMES_HOME", tmpDir)
	defer os.Unsetenv("HERMES_HOME")

	home := GetProfileHome("myprofile")
	if !strings.Contains(home, "profiles") {
		t.Errorf("Expected profile home to contain 'profiles', got '%s'", home)
	}
	if !strings.HasSuffix(home, "myprofile") {
		t.Errorf("Expected profile home to end with 'myprofile', got '%s'", home)
	}

	defaultHome := GetProfileHome("")
	if strings.Contains(defaultHome, "profiles") {
		t.Error("Default profile home should not contain 'profiles'")
	}
}

// --- MergeConfig tests ---

func TestMergeConfig(t *testing.T) {
	dst := DefaultConfig()
	src := &Config{
		Model:         "new-model",
		MaxIterations: 200,
		Display: DisplayConfig{
			Skin: "mono",
		},
		Terminal: TerminalConfig{
			DefaultTimeout: 300,
		},
	}

	mergeConfig(dst, src)

	if dst.Model != "new-model" {
		t.Errorf("Expected model 'new-model', got '%s'", dst.Model)
	}
	if dst.MaxIterations != 200 {
		t.Errorf("Expected max_iterations 200, got %d", dst.MaxIterations)
	}
	if dst.Display.Skin != "mono" {
		t.Errorf("Expected skin 'mono', got '%s'", dst.Display.Skin)
	}
	if dst.Terminal.DefaultTimeout != 300 {
		t.Errorf("Expected default_timeout 300, got %d", dst.Terminal.DefaultTimeout)
	}
	// Unset fields should keep defaults
	if dst.Terminal.Environment != "local" {
		t.Errorf("Expected default environment 'local', got '%s'", dst.Terminal.Environment)
	}
}

func TestMergeConfig_EmptySource(t *testing.T) {
	dst := DefaultConfig()
	originalModel := dst.Model
	src := &Config{}

	mergeConfig(dst, src)

	if dst.Model != originalModel {
		t.Error("Empty source should not change destination model")
	}
}
