package config

import (
	"log/slog"
)

// CurrentConfigVersion is the latest config format version.
const CurrentConfigVersion = 5

// Migration describes a single config version upgrade step.
type Migration struct {
	FromVersion int
	ToVersion   int
	Migrate     func(cfg map[string]any) map[string]any
}

// Migrations is the ordered list of config migrations.
var Migrations = []Migration{
	{
		FromVersion: 1,
		ToVersion:   2,
		Migrate: func(cfg map[string]any) map[string]any {
			// v1 -> v2: Rename "llm_provider" to "provider".
			if v, ok := cfg["llm_provider"]; ok {
				cfg["provider"] = v
				delete(cfg, "llm_provider")
			}
			// Move "api_base" to "base_url".
			if v, ok := cfg["api_base"]; ok {
				cfg["base_url"] = v
				delete(cfg, "api_base")
			}
			return cfg
		},
	},
	{
		FromVersion: 2,
		ToVersion:   3,
		Migrate: func(cfg map[string]any) map[string]any {
			// v2 -> v3: Nest display-related keys under "display".
			display := getOrCreateMap(cfg, "display")
			for _, key := range []string{"skin", "tool_progress", "streaming_enabled"} {
				if v, ok := cfg[key]; ok {
					display[key] = v
					delete(cfg, key)
				}
			}
			cfg["display"] = display
			return cfg
		},
	},
	{
		FromVersion: 3,
		ToVersion:   4,
		Migrate: func(cfg map[string]any) map[string]any {
			// v3 -> v4: Nest terminal-related keys under "terminal".
			terminal := getOrCreateMap(cfg, "terminal")
			for _, key := range []string{"default_timeout", "max_timeout", "environment", "docker_image", "ssh_host"} {
				if v, ok := cfg[key]; ok {
					terminal[key] = v
					delete(cfg, key)
				}
			}
			cfg["terminal"] = terminal

			// Also nest memory keys.
			memory := getOrCreateMap(cfg, "memory")
			if v, ok := cfg["memory_enabled"]; ok {
				memory["enabled"] = v
				delete(cfg, "memory_enabled")
			}
			if v, ok := cfg["memory_provider"]; ok {
				memory["provider"] = v
				delete(cfg, "memory_provider")
			}
			cfg["memory"] = memory

			return cfg
		},
	},
	{
		FromVersion: 4,
		ToVersion:   5,
		Migrate: func(cfg map[string]any) map[string]any {
			// v4 -> v5: Add reasoning config defaults if missing.
			reasoning := getOrCreateMap(cfg, "reasoning")
			if _, ok := reasoning["enabled"]; !ok {
				reasoning["enabled"] = false
			}
			if _, ok := reasoning["effort"]; !ok {
				reasoning["effort"] = "medium"
			}
			cfg["reasoning"] = reasoning

			// Ensure auxiliary config exists.
			if _, ok := cfg["auxiliary"]; !ok {
				cfg["auxiliary"] = map[string]any{}
			}

			return cfg
		},
	},
}

// MigrateConfig applies all necessary migrations to bring a config map
// up to CurrentConfigVersion. Returns the migrated config and whether
// any migration was actually applied.
func MigrateConfig(cfg map[string]any) (map[string]any, bool) {
	version := getConfigVersion(cfg)
	if version >= CurrentConfigVersion {
		return cfg, false
	}

	migrated := false
	for _, m := range Migrations {
		if version < m.FromVersion {
			// Skip migrations below the current version -- should not happen
			// with a well-ordered list, but guard anyway.
			continue
		}
		if version == m.FromVersion {
			slog.Info("Migrating config", "from", m.FromVersion, "to", m.ToVersion)
			cfg = m.Migrate(cfg)
			version = m.ToVersion
			migrated = true
		}
	}

	if migrated {
		cfg["_config_version"] = CurrentConfigVersion
	}

	return cfg, migrated
}

// getConfigVersion extracts the config version number from a config map.
func getConfigVersion(cfg map[string]any) int {
	v, ok := cfg["_config_version"]
	if !ok {
		return 1 // Assume version 1 for configs without a version.
	}

	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case int64:
		return int(val)
	default:
		return 1
	}
}

// getOrCreateMap returns an existing nested map or creates a new one.
func getOrCreateMap(cfg map[string]any, key string) map[string]any {
	if existing, ok := cfg[key].(map[string]any); ok {
		return existing
	}
	m := make(map[string]any)
	cfg[key] = m
	return m
}
