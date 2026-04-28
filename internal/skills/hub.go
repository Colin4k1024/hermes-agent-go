package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// HubSource represents a skill source for discovery.
type HubSource struct {
	Name  string `json:"name"`
	Type  string `json:"type"` // "github", "url", "local"
	URL   string `json:"url"`
	Trust string `json:"trust"` // "builtin", "trusted", "community"
}

// HubSkill represents a skill available from a hub.
type HubSkill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Tags        []string `json:"tags"`
	Source      string   `json:"source"`
	URL         string   `json:"url"`
	Trust       string   `json:"trust"`
}

// DefaultSources returns the default skill sources.
func DefaultSources() []HubSource {
	return []HubSource{
		{
			Name:  "agentskills.io",
			Type:  "url",
			URL:   "https://agentskills.io/api/skills",
			Trust: "community",
		},
		{
			Name:  "hermes-official",
			Type:  "github",
			URL:   "https://api.github.com/repos/NousResearch/hermes-agent/contents/optional-skills",
			Trust: "trusted",
		},
	}
}

// SearchHub searches for skills across all configured sources.
func SearchHub(query string) ([]HubSkill, error) {
	var results []HubSkill

	for _, source := range DefaultSources() {
		skills, err := searchSource(source, query)
		if err != nil {
			slog.Debug("Hub source search failed", "source", source.Name, "error", err)
			continue
		}
		results = append(results, skills...)
	}

	return results, nil
}

// InstallFromHub downloads and installs a skill from a hub source.
func InstallFromHub(skillName, sourceURL string) error {
	skillsDir := filepath.Join(config.HermesHome(), "skills", skillName)
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("create skill directory: %w", err)
	}

	// Download SKILL.md
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return fmt.Errorf("download skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	skillPath := filepath.Join(skillsDir, "SKILL.md")
	if err := os.WriteFile(skillPath, data, 0644); err != nil {
		return fmt.Errorf("save skill: %w", err)
	}

	// Security scan before committing the install
	trust := TrustCommunity
	if isTrustedSource(sourceURL) {
		trust = TrustTrusted
	}
	scanResult, scanErr := ScanSkillWithTrust(skillsDir, trust)
	if scanErr == nil && scanResult != nil {
		decision := ShouldAllowInstall(trust, scanResult)
		if decision == InstallBlock {
			os.RemoveAll(skillsDir)
			return fmt.Errorf("skill blocked by security scan: %s", FormatIssues(scanResult.Findings))
		}
	}

	// Write lock entry
	writeLockEntry(skillName, sourceURL)

	slog.Info("Skill installed", "name", skillName, "path", skillsDir)
	return nil
}

func isTrustedSource(url string) bool {
	for _, src := range DefaultSources() {
		if strings.Contains(url, src.URL) {
			return true
		}
	}
	return false
}

// UninstallSkill removes an installed skill.
func UninstallSkill(skillName string) error {
	skillsDir := filepath.Join(config.HermesHome(), "skills", skillName)
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return fmt.Errorf("skill '%s' not found", skillName)
	}

	if err := os.RemoveAll(skillsDir); err != nil {
		return fmt.Errorf("remove skill: %w", err)
	}

	removeLockEntry(skillName)
	return nil
}

func searchSource(source HubSource, query string) ([]HubSkill, error) {
	switch source.Type {
	case "url":
		return searchURLSource(source, query)
	case "github":
		return searchGitHubSource(source, query)
	default:
		return nil, fmt.Errorf("unknown source type: %s", source.Type)
	}
}

func searchURLSource(source HubSource, query string) ([]HubSkill, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	url := source.URL
	if query != "" {
		url += "?q=" + query
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var skills []HubSkill
	if err := json.NewDecoder(resp.Body).Decode(&skills); err != nil {
		return nil, err
	}

	for i := range skills {
		skills[i].Source = source.Name
		skills[i].Trust = source.Trust
	}

	return skills, nil
}

func searchGitHubSource(source HubSource, query string) ([]HubSkill, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", source.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "HermesAgent/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	var skills []HubSkill
	for _, e := range entries {
		if e.Type != "dir" {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(query)) {
			continue
		}
		skills = append(skills, HubSkill{
			Name:   e.Name,
			Source: source.Name,
			Trust:  source.Trust,
			URL:    source.URL + "/" + e.Name,
		})
	}

	return skills, nil
}

// Lock file management
type lockEntry struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Installed string `json:"installed"`
}

func getLockPath() string {
	return filepath.Join(config.HermesHome(), "skills", ".hub", "lock.json")
}

func writeLockEntry(name, source string) {
	lockPath := getLockPath()
	os.MkdirAll(filepath.Dir(lockPath), 0755)

	var entries []lockEntry
	if data, err := os.ReadFile(lockPath); err == nil {
		json.Unmarshal(data, &entries)
	}

	// Update or add
	found := false
	for i, e := range entries {
		if e.Name == name {
			entries[i].Source = source
			entries[i].Installed = time.Now().Format(time.RFC3339)
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, lockEntry{
			Name:      name,
			Source:    source,
			Installed: time.Now().Format(time.RFC3339),
		})
	}

	data, _ := json.MarshalIndent(entries, "", "  ")
	os.WriteFile(lockPath, data, 0644)
}

func removeLockEntry(name string) {
	lockPath := getLockPath()
	var entries []lockEntry
	if data, err := os.ReadFile(lockPath); err == nil {
		json.Unmarshal(data, &entries)
	}

	var filtered []lockEntry
	for _, e := range entries {
		if e.Name != name {
			filtered = append(filtered, e)
		}
	}

	data, _ := json.MarshalIndent(filtered, "", "  ")
	os.WriteFile(lockPath, data, 0644)
}
