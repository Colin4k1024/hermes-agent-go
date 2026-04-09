package config

import (
	"os"
	"path/filepath"
	"strings"
)

// HermesHome returns the Hermes home directory (default: ~/.hermes).
// When a profile is active (see profiles.go), it returns the profile's directory.
// Reads HERMES_HOME env var, falls back to ~/.hermes.
func HermesHome() string {
	if hermesHomeHook != nil {
		return hermesHomeHook()
	}
	if h := os.Getenv("HERMES_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".hermes")
	}
	return filepath.Join(home, ".hermes")
}

// DisplayHermesHome returns a user-friendly display string for HermesHome.
// Uses ~/ shorthand for readability.
func DisplayHermesHome() string {
	h := HermesHome()
	home, err := os.UserHomeDir()
	if err != nil {
		return h
	}
	if strings.HasPrefix(h, home) {
		return "~" + h[len(home):]
	}
	return h
}

// GetHermesDir resolves a Hermes subdirectory with backward compatibility.
// New installs get the consolidated layout (e.g. cache/images).
// Existing installs that already have the old path keep using it.
func GetHermesDir(newSubpath, oldName string) string {
	home := HermesHome()
	oldPath := filepath.Join(home, oldName)
	if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
		return oldPath
	}
	return filepath.Join(home, newSubpath)
}

// EnsureDir creates a directory and all parents if they don't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// EnsureHermesHome creates the Hermes home directory structure.
func EnsureHermesHome() error {
	home := HermesHome()
	dirs := []string{
		home,
		filepath.Join(home, "sessions"),
		filepath.Join(home, "logs"),
		filepath.Join(home, "memories"),
		filepath.Join(home, "skills"),
		filepath.Join(home, "cron"),
		filepath.Join(home, "cache"),
		filepath.Join(home, "cache", "images"),
		filepath.Join(home, "cache", "audio"),
		filepath.Join(home, "cache", "documents"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}
