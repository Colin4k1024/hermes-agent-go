package config

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// Config represents the full Hermes configuration.
type Config struct {
	Model    string `yaml:"model"`
	Provider string `yaml:"provider"`
	BaseURL  string `yaml:"base_url"`
	APIKey   string `yaml:"api_key"`
	APIMode  string `yaml:"api_mode"`

	MaxIterations int     `yaml:"max_iterations"`
	ToolDelay     float64 `yaml:"tool_delay"`
	MaxTokens     int     `yaml:"max_tokens"`

	Display    DisplayConfig    `yaml:"display"`
	Terminal   TerminalConfig   `yaml:"terminal"`
	Memory     MemoryConfig     `yaml:"memory"`
	Toolsets   ToolsetsConfig   `yaml:"toolsets"`
	Reasoning  ReasoningConfig  `yaml:"reasoning"`
	Delegation DelegationConfig `yaml:"delegation"`
	Auxiliary  AuxiliaryConfig  `yaml:"auxiliary"`
	Plugins    PluginsConfig    `yaml:"plugins"`

	ProviderRouting map[string]any `yaml:"provider_routing"`

	// SaaS / Stateless
	Database DatabaseConfig `yaml:"database"`
	Redis    RedisConfig    `yaml:"redis"`

	// Internal
	configVersion int `yaml:"_config_version"`
}

// DatabaseConfig controls the state store backend.
type DatabaseConfig struct {
	Driver string `yaml:"driver"` // "postgres" or "sqlite" (default)
	URL    string `yaml:"url"`
}

// RedisConfig controls the Redis connection for distributed state.
type RedisConfig struct {
	URL string `yaml:"url"`
}

// DisplayConfig controls CLI display options.
type DisplayConfig struct {
	Skin                           string `yaml:"skin"`
	ToolProgress                   bool   `yaml:"tool_progress"`
	ToolProgressCommand            bool   `yaml:"tool_progress_command"`
	BackgroundProcessNotifications string `yaml:"background_process_notifications"`
	StreamingEnabled               bool   `yaml:"streaming_enabled"`
}

// TerminalConfig controls terminal tool behavior.
type TerminalConfig struct {
	DefaultTimeout int    `yaml:"default_timeout"`
	MaxTimeout     int    `yaml:"max_timeout"`
	Environment    string `yaml:"environment"`
	DockerImage    string `yaml:"docker_image"`
	SSHHost        string `yaml:"ssh_host"`
}

// MemoryConfig controls the memory system.
type MemoryConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Provider string `yaml:"provider"`
}

// ToolsetsConfig controls which toolsets are enabled/disabled.
type ToolsetsConfig struct {
	Enabled  []string `yaml:"enabled"`
	Disabled []string `yaml:"disabled"`
}

// ReasoningConfig controls extended thinking.
type ReasoningConfig struct {
	Enabled bool   `yaml:"enabled"`
	Effort  string `yaml:"effort"`
}

// DelegationConfig controls subagent delegation.
type DelegationConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// AuxiliaryConfig controls auxiliary LLM clients.
type AuxiliaryConfig struct {
	WebExtract   map[string]any `yaml:"web_extract"`
	SummaryModel string         `yaml:"summary_model"`
}

// PluginsConfig controls plugin discovery and loading.
type PluginsConfig struct {
	Disabled []string `yaml:"disabled"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Model:         "anthropic/claude-sonnet-4-20250514",
		MaxIterations: 90,
		ToolDelay:     1.0,
		Display: DisplayConfig{
			Skin:                           "default",
			ToolProgress:                   true,
			StreamingEnabled:               true,
			BackgroundProcessNotifications: "all",
		},
		Terminal: TerminalConfig{
			DefaultTimeout: 120,
			MaxTimeout:     600,
			Environment:    "local",
		},
		Memory: MemoryConfig{
			Enabled: true,
		},
		Reasoning: ReasoningConfig{
			Enabled: false,
			Effort:  "medium",
		},
	}
}

var (
	globalConfig *Config
	configOnce   sync.Once
)

// Load reads the configuration from disk, merging with defaults.
func Load() *Config {
	configOnce.Do(func() {
		globalConfig = DefaultConfig()

		// Load .env file (optional)
		_ = godotenv.Load(filepath.Join(HermesHome(), ".env"))

		// Load config.yaml (optional) — merge over defaults
		if data, err := os.ReadFile(filepath.Join(HermesHome(), "config.yaml")); err == nil {
			var fileConfig Config
			if err := yaml.Unmarshal(data, &fileConfig); err == nil {
				mergeConfig(globalConfig, &fileConfig)
			}
		}

		// Env var overrides (highest priority)
		applyEnvOverrides(globalConfig)
	})
	return globalConfig
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("DATABASE_DRIVER"); v != "" {
		cfg.Database.Driver = v
	}
	if v := os.Getenv("REDIS_URL"); v != "" {
		cfg.Redis.URL = v
	}
	// LLM configuration
	if v := os.Getenv("HERMES_DEFAULT_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("HERMES_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("HERMES_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv("HERMES_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("HERMES_API_KEY_LLM"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("HERMES_API_MODE"); v != "" {
		cfg.APIMode = v
	}
}

// Reload forces a config reload.
func Reload() *Config {
	configOnce = sync.Once{}
	globalConfig = nil
	return Load()
}

// Save writes the current configuration to disk.
func Save(cfg *Config) error {
	configPath := filepath.Join(HermesHome(), "config.yaml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	// Atomic write: write to temp file then rename to prevent corruption
	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, configPath)
}

func mergeConfig(dst, src *Config) {
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Provider != "" {
		dst.Provider = src.Provider
	}
	if src.BaseURL != "" {
		dst.BaseURL = src.BaseURL
	}
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
	}
	if src.APIMode != "" {
		dst.APIMode = src.APIMode
	}
	if src.MaxIterations > 0 {
		dst.MaxIterations = src.MaxIterations
	}
	if src.ToolDelay > 0 {
		dst.ToolDelay = src.ToolDelay
	}
	if src.MaxTokens > 0 {
		dst.MaxTokens = src.MaxTokens
	}
	if src.Display.Skin != "" {
		dst.Display.Skin = src.Display.Skin
	}
	if src.Terminal.DefaultTimeout > 0 {
		dst.Terminal.DefaultTimeout = src.Terminal.DefaultTimeout
	}
	if src.Terminal.Environment != "" {
		dst.Terminal.Environment = src.Terminal.Environment
	}
	if src.Reasoning.Effort != "" {
		dst.Reasoning.Effort = src.Reasoning.Effort
	}
	if len(src.Toolsets.Enabled) > 0 {
		dst.Toolsets.Enabled = src.Toolsets.Enabled
	}
	if len(src.Toolsets.Disabled) > 0 {
		dst.Toolsets.Disabled = src.Toolsets.Disabled
	}
	if src.Auxiliary.SummaryModel != "" {
		dst.Auxiliary.SummaryModel = src.Auxiliary.SummaryModel
	}
	if src.ProviderRouting != nil {
		dst.ProviderRouting = src.ProviderRouting
	}
	// Database & Redis
	if src.Database.Driver != "" {
		dst.Database.Driver = src.Database.Driver
	}
	if src.Database.URL != "" {
		dst.Database.URL = src.Database.URL
	}
	if src.Redis.URL != "" {
		dst.Redis.URL = src.Redis.URL
	}
	if src.Memory.Provider != "" {
		dst.Memory.Provider = src.Memory.Provider
	}
}
