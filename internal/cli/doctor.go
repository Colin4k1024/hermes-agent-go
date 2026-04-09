package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/cron"
	"github.com/hermes-agent/hermes-agent-go/internal/skills"
	"github.com/hermes-agent/hermes-agent-go/internal/state"
	"github.com/hermes-agent/hermes-agent-go/internal/tools"
)

// checkStatus represents the outcome of a single diagnostic check.
type checkStatus int

const (
	statusPass checkStatus = iota
	statusWarn
	statusFail
	statusInfo
)

// checkResult holds one diagnostic check result.
type checkResult struct {
	Name    string
	Status  checkStatus
	Message string
}

// statusIcon returns a terminal-friendly indicator for the check status.
func statusIcon(s checkStatus) string {
	switch s {
	case statusPass:
		return "[PASS]"
	case statusWarn:
		return "[WARN]"
	case statusFail:
		return "[FAIL]"
	case statusInfo:
		return "[INFO]"
	default:
		return "[????]"
	}
}

// RunDoctor performs a comprehensive set of diagnostic checks and prints results.
func RunDoctor() {
	fmt.Println("Hermes Agent Diagnostics")
	fmt.Println("========================")
	fmt.Println()

	var results []checkResult

	// --- Hermes home directory ---
	results = append(results, checkHermesHome()...)

	// --- Configuration file ---
	results = append(results, checkConfigFile()...)

	// --- Environment / .env file ---
	results = append(results, checkEnvFile()...)

	// --- Provider API keys ---
	results = append(results, checkAPIKeys()...)

	// --- Messaging tokens ---
	results = append(results, checkMessagingTokens()...)

	// --- Optional tools ---
	results = append(results, checkOptionalTools()...)

	// --- Skills ---
	results = append(results, checkSkills()...)

	// --- Cron jobs ---
	results = append(results, checkCronJobs()...)

	// --- Session database ---
	results = append(results, checkSessionDB()...)

	// --- MCP servers ---
	results = append(results, checkMCPServers()...)

	// --- System dependencies ---
	results = append(results, checkSystemDeps()...)

	// --- Print results ---
	fmt.Println()
	passCount, warnCount, failCount := 0, 0, 0
	for _, r := range results {
		fmt.Printf("  %-6s  %-30s  %s\n", statusIcon(r.Status), r.Name, r.Message)
		switch r.Status {
		case statusPass:
			passCount++
		case statusWarn:
			warnCount++
		case statusFail:
			failCount++
		}
	}

	// --- Summary ---
	fmt.Println()
	fmt.Printf("Summary: %d passed, %d warnings, %d failed\n", passCount, warnCount, failCount)

	if failCount > 0 {
		fmt.Println()
		fmt.Println("Run 'hermes setup' to configure missing items.")
	} else if warnCount > 0 {
		fmt.Println()
		fmt.Println("All critical checks passed. Warnings are informational.")
	} else {
		fmt.Println()
		fmt.Println("All checks passed. Hermes is ready to go!")
	}
}

func checkHermesHome() []checkResult {
	home := config.HermesHome()
	info, err := os.Stat(home)
	if err != nil {
		return []checkResult{{
			Name:    "Hermes home",
			Status:  statusFail,
			Message: fmt.Sprintf("%s does not exist (run 'hermes setup')", home),
		}}
	}
	if !info.IsDir() {
		return []checkResult{{
			Name:    "Hermes home",
			Status:  statusFail,
			Message: fmt.Sprintf("%s exists but is not a directory", home),
		}}
	}

	// Check writable by creating a temp file.
	tmpPath := filepath.Join(home, ".doctor_check")
	if err := os.WriteFile(tmpPath, []byte("ok"), 0644); err != nil {
		return []checkResult{{
			Name:    "Hermes home",
			Status:  statusFail,
			Message: fmt.Sprintf("%s is not writable", home),
		}}
	}
	os.Remove(tmpPath)

	return []checkResult{{
		Name:    "Hermes home",
		Status:  statusPass,
		Message: config.DisplayHermesHome(),
	}}
}

func checkConfigFile() []checkResult {
	configPath := filepath.Join(config.HermesHome(), "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return []checkResult{{
			Name:    "config.yaml",
			Status:  statusWarn,
			Message: "not found (using defaults)",
		}}
	}

	// Try loading it.
	cfg := config.Load()
	if cfg == nil {
		return []checkResult{{
			Name:    "config.yaml",
			Status:  statusFail,
			Message: "exists but failed to parse",
		}}
	}

	return []checkResult{{
		Name:    "config.yaml",
		Status:  statusPass,
		Message: fmt.Sprintf("loaded (model: %s)", cfg.Model),
	}}
}

func checkEnvFile() []checkResult {
	envPath := filepath.Join(config.HermesHome(), ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return []checkResult{{
			Name:    ".env file",
			Status:  statusWarn,
			Message: "not found (API keys must be set as env vars)",
		}}
	}

	// Count non-empty, non-comment lines.
	data, err := os.ReadFile(envPath)
	if err != nil {
		return []checkResult{{
			Name:    ".env file",
			Status:  statusFail,
			Message: fmt.Sprintf("cannot read: %v", err),
		}}
	}

	keyCount := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "=") {
			keyCount++
		}
	}

	if keyCount == 0 {
		return []checkResult{{
			Name:    ".env file",
			Status:  statusWarn,
			Message: "exists but contains no keys",
		}}
	}

	return []checkResult{{
		Name:    ".env file",
		Status:  statusPass,
		Message: fmt.Sprintf("%d key(s) defined", keyCount),
	}}
}

func checkAPIKeys() []checkResult {
	providers := []struct {
		name string
		env  string
	}{
		{"OpenRouter", "OPENROUTER_API_KEY"},
		{"OpenAI", "OPENAI_API_KEY"},
		{"Anthropic", "ANTHROPIC_API_KEY"},
		{"DeepSeek", "DEEPSEEK_API_KEY"},
		{"Google AI", "GOOGLE_API_KEY"},
		{"Nous Portal", "NOUS_API_KEY"},
		{"Exa", "EXA_API_KEY"},
		{"Firecrawl", "FIRECRAWL_API_KEY"},
	}

	var results []checkResult
	anySet := false
	for _, p := range providers {
		if config.HasEnv(p.env) {
			anySet = true
			results = append(results, checkResult{
				Name:    p.name + " API key",
				Status:  statusPass,
				Message: "set",
			})
		} else {
			results = append(results, checkResult{
				Name:    p.name + " API key",
				Status:  statusInfo,
				Message: "not set",
			})
		}
	}

	if !anySet {
		// Upgrade all to warnings if nothing is set.
		for i := range results {
			if results[i].Status == statusInfo {
				results[i].Status = statusWarn
			}
		}
	}

	return results
}

func checkMessagingTokens() []checkResult {
	tokens := []struct {
		name string
		env  string
	}{
		{"Telegram", "TELEGRAM_BOT_TOKEN"},
		{"Discord", "DISCORD_BOT_TOKEN"},
		{"Slack bot", "SLACK_BOT_TOKEN"},
		{"Slack app", "SLACK_APP_TOKEN"},
	}

	var results []checkResult
	for _, t := range tokens {
		if config.HasEnv(t.env) {
			results = append(results, checkResult{
				Name:    t.name + " token",
				Status:  statusPass,
				Message: "set",
			})
		} else {
			results = append(results, checkResult{
				Name:    t.name + " token",
				Status:  statusInfo,
				Message: "not set (optional)",
			})
		}
	}
	return results
}

func checkOptionalTools() []checkResult {
	binaries := []struct {
		name string
		bin  string
		hint string
	}{
		{"edge-tts", "edge-tts", "pip install edge-tts"},
		{"whisper", "whisper", "pip install openai-whisper"},
		{"docker", "docker", "https://docs.docker.com/get-docker/"},
		{"ssh", "ssh", "system package manager"},
		{"modal", "modal", "pip install modal"},
		{"pngpaste", "pngpaste", "brew install pngpaste (macOS)"},
		{"ffmpeg", "ffmpeg", "brew install ffmpeg / apt install ffmpeg"},
	}

	var results []checkResult
	for _, b := range binaries {
		if _, err := exec.LookPath(b.bin); err == nil {
			results = append(results, checkResult{
				Name:    b.name,
				Status:  statusPass,
				Message: "found in PATH",
			})
		} else {
			results = append(results, checkResult{
				Name:    b.name,
				Status:  statusInfo,
				Message: fmt.Sprintf("not found (%s)", b.hint),
			})
		}
	}
	return results
}

func checkSkills() []checkResult {
	allSkills, err := skills.LoadAllSkills()
	if err != nil {
		return []checkResult{{
			Name:    "Skills",
			Status:  statusWarn,
			Message: fmt.Sprintf("error loading: %v", err),
		}}
	}

	return []checkResult{{
		Name:    "Skills",
		Status:  statusPass,
		Message: fmt.Sprintf("%d installed", len(allSkills)),
	}}
}

func checkCronJobs() []checkResult {
	store := cron.NewJobStore()
	if err := store.Load(); err != nil {
		return []checkResult{{
			Name:    "Cron jobs",
			Status:  statusWarn,
			Message: fmt.Sprintf("error loading: %v", err),
		}}
	}
	return []checkResult{{
		Name:    "Cron jobs",
		Status:  statusPass,
		Message: fmt.Sprintf("%d configured", len(store.List())),
	}}
}

func checkSessionDB() []checkResult {
	dbPath := filepath.Join(config.HermesHome(), "state.db")

	info, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		return []checkResult{{
			Name:    "Session DB",
			Status:  statusInfo,
			Message: "not yet created (will be created on first use)",
		}}
	}
	if err != nil {
		return []checkResult{{
			Name:    "Session DB",
			Status:  statusFail,
			Message: fmt.Sprintf("cannot stat: %v", err),
		}}
	}

	// Try opening.
	db, dbErr := state.NewSessionDB(dbPath)
	if dbErr != nil {
		return []checkResult{{
			Name:    "Session DB",
			Status:  statusFail,
			Message: fmt.Sprintf("cannot open: %v", dbErr),
		}}
	}
	db.Close()

	sizeKB := info.Size() / 1024
	sizeMB := float64(info.Size()) / (1024 * 1024)
	sizeStr := fmt.Sprintf("%d KB", sizeKB)
	if sizeMB >= 1.0 {
		sizeStr = fmt.Sprintf("%.1f MB", sizeMB)
	}

	return []checkResult{{
		Name:    "Session DB",
		Status:  statusPass,
		Message: fmt.Sprintf("accessible (%s)", sizeStr),
	}}
}

func checkMCPServers() []checkResult {
	mcpCfg, err := tools.LoadMCPConfig()
	if err != nil {
		return []checkResult{{
			Name:    "MCP servers",
			Status:  statusInfo,
			Message: "no configuration found",
		}}
	}

	count := len(mcpCfg.Servers)
	if count == 0 {
		return []checkResult{{
			Name:    "MCP servers",
			Status:  statusInfo,
			Message: "none configured",
		}}
	}

	var names []string
	for name := range mcpCfg.Servers {
		names = append(names, name)
	}

	return []checkResult{{
		Name:    "MCP servers",
		Status:  statusPass,
		Message: fmt.Sprintf("%d configured (%s)", count, strings.Join(names, ", ")),
	}}
}

func checkSystemDeps() []checkResult {
	var results []checkResult

	// Node.js (needed for browser tools).
	if path, err := exec.LookPath("node"); err == nil {
		out, _ := exec.Command(path, "--version").Output()
		ver := strings.TrimSpace(string(out))
		results = append(results, checkResult{
			Name:    "Node.js",
			Status:  statusPass,
			Message: ver,
		})
	} else {
		results = append(results, checkResult{
			Name:    "Node.js",
			Status:  statusWarn,
			Message: "not found (needed for browser tools)",
		})
	}

	// Git.
	if path, err := exec.LookPath("git"); err == nil {
		out, _ := exec.Command(path, "--version").Output()
		ver := strings.TrimSpace(string(out))
		results = append(results, checkResult{
			Name:    "Git",
			Status:  statusPass,
			Message: ver,
		})
	} else {
		results = append(results, checkResult{
			Name:    "Git",
			Status:  statusWarn,
			Message: "not found",
		})
	}

	return results
}
