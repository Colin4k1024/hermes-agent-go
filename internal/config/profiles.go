package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Profile represents an isolated Hermes instance with its own config dir.
type Profile struct {
	Name string
	Home string // e.g. ~/.hermes/profiles/coder
}

var (
	activeProfile       string
	activeProfileLoaded bool
	profileMu           sync.RWMutex
)

// ListProfiles returns all available profiles.
// The default profile (unnamed, using ~/.hermes/) is always included.
func ListProfiles() []Profile {
	profiles := []Profile{
		{Name: "default", Home: defaultHermesHome()},
	}

	profilesDir := filepath.Join(defaultHermesHome(), "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return profiles
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		profiles = append(profiles, Profile{
			Name: name,
			Home: filepath.Join(profilesDir, name),
		})
	}

	return profiles
}

// GetActiveProfile returns the name of the currently active profile.
// Returns "" for the default profile.
func GetActiveProfile() string {
	profileMu.RLock()
	if activeProfileLoaded {
		p := activeProfile
		profileMu.RUnlock()
		return p
	}
	profileMu.RUnlock()

	profileMu.Lock()
	defer profileMu.Unlock()
	if activeProfileLoaded {
		return activeProfile
	}
	activeProfile = loadActiveProfile()
	activeProfileLoaded = true
	return activeProfile
}

// SetActiveProfile changes the active profile by writing to the marker file.
// Pass "" to revert to the default profile.
func SetActiveProfile(name string) error {
	if name != "" && name != "default" {
		home := GetProfileHome(name)
		if _, err := os.Stat(home); os.IsNotExist(err) {
			return fmt.Errorf("profile %q does not exist", name)
		}
	}

	markerPath := filepath.Join(defaultHermesHome(), "active_profile")

	if name == "" || name == "default" {
		os.Remove(markerPath)
		profileMu.Lock()
		activeProfile = ""
		activeProfileLoaded = true
		profileMu.Unlock()
		InvalidateConfig()
		return nil
	}

	if err := os.WriteFile(markerPath, []byte(name), 0644); err != nil {
		return fmt.Errorf("write active_profile: %w", err)
	}

	profileMu.Lock()
	activeProfile = name
	activeProfileLoaded = true
	profileMu.Unlock()
	InvalidateConfig()
	return nil
}

// CreateProfile creates a new named profile directory with standard sub-dirs.
func CreateProfile(name string) error {
	if name == "" || name == "default" {
		return fmt.Errorf("cannot create profile with reserved name %q", name)
	}
	if strings.ContainsAny(name, "/\\. ") {
		return fmt.Errorf("profile name must not contain path separators, dots, or spaces")
	}

	home := GetProfileHome(name)
	if _, err := os.Stat(home); err == nil {
		return fmt.Errorf("profile %q already exists", name)
	}

	// Create directory structure matching EnsureHermesHome.
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
			return fmt.Errorf("create profile dir %s: %w", d, err)
		}
	}

	return nil
}

// DeleteProfile removes a named profile directory.
// It refuses to delete the default profile.
func DeleteProfile(name string) error {
	if name == "" || name == "default" {
		return fmt.Errorf("cannot delete the default profile")
	}

	home := GetProfileHome(name)
	if _, err := os.Stat(home); os.IsNotExist(err) {
		return fmt.Errorf("profile %q does not exist", name)
	}

	// If this profile is currently active, revert to default first.
	if GetActiveProfile() == name {
		if err := SetActiveProfile(""); err != nil {
			return fmt.Errorf("revert active profile: %w", err)
		}
	}

	return os.RemoveAll(home)
}

// GetProfileHome returns the home directory for a named profile.
func GetProfileHome(name string) string {
	if name == "" || name == "default" {
		return defaultHermesHome()
	}
	return filepath.Join(defaultHermesHome(), "profiles", name)
}

// defaultHermesHome returns the base hermes home (ignoring profiles).
func defaultHermesHome() string {
	if h := os.Getenv("HERMES_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".hermes")
	}
	return filepath.Join(home, ".hermes")
}

// loadActiveProfile reads the active profile from the marker file or --profile flag.
func loadActiveProfile() string {
	// CLI flag takes priority — checked via package-level variable set by
	// the caller before any config access (see OverrideActiveProfile).
	if profileOverride != "" {
		return profileOverride
	}

	markerPath := filepath.Join(defaultHermesHome(), "active_profile")
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// profileOverride is set via OverrideActiveProfile, typically from --profile flag.
var profileOverride string

// OverrideActiveProfile sets the profile for this process, overriding the
// marker file. Call this early, before any HermesHome() / Load() calls.
func OverrideActiveProfile(name string) {
	profileMu.Lock()
	profileOverride = name
	activeProfile = ""
	activeProfileLoaded = false
	profileMu.Unlock()
	InvalidateConfig()
}

// init patches HermesHome to be profile-aware.
// The original HermesHome is simple; the profile layer wraps it.
func init() {
	// We replace the package-level HermesHome function via an internal hook.
	hermesHomeHook = func() string {
		profile := GetActiveProfile()
		if profile == "" || profile == "default" {
			return defaultHermesHome()
		}
		return GetProfileHome(profile)
	}
}

// hermesHomeHook is an internal hook that HermesHome delegates to when set.
// It's set in profiles.go init() to make HermesHome profile-aware.
var hermesHomeHook func() string
