package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// RunSetupWizard runs an interactive setup wizard that walks the user through
// initial configuration of Hermes Agent.
func RunSetupWizard() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("=================================")
	fmt.Println("   Hermes Agent Setup Wizard")
	fmt.Println("=================================")
	fmt.Println()

	// Ensure home directory exists.
	config.EnsureHermesHome()
	fmt.Printf("Home directory: %s\n\n", config.DisplayHermesHome())

	// --- Step 1: Choose provider ---
	fmt.Println("Step 1: Choose your API provider")
	fmt.Println()

	providers := ListProviders()
	for i, p := range providers {
		status := ""
		if p.Configured {
			status = " [configured]"
		}
		fmt.Printf("  %d. %s%s\n", i+1, p.Name, status)
	}
	fmt.Printf("  %d. Custom endpoint\n", len(providers)+1)
	fmt.Println()

	choice := promptInput(scanner, "Select provider (number)", "1")
	choiceIdx := 0
	if _, err := fmt.Sscanf(choice, "%d", &choiceIdx); err != nil || choiceIdx < 1 || choiceIdx > len(providers)+1 {
		choiceIdx = 1
	}

	var selectedProvider string
	var selectedBaseURL string
	var envKeyName string

	if choiceIdx <= len(providers) {
		p := providers[choiceIdx-1]
		selectedProvider = p.ID
		selectedBaseURL = p.BaseURL
		envKeyName = p.EnvKey
		fmt.Printf("\nSelected: %s (%s)\n", p.Name, p.BaseURL)
	} else {
		selectedProvider = "custom"
		selectedBaseURL = promptInput(scanner, "Enter base URL", "https://api.example.com/v1")
		envKeyName = "CUSTOM_API_KEY"
		fmt.Printf("\nCustom endpoint: %s\n", selectedBaseURL)
	}

	// --- Step 2: Enter API key ---
	fmt.Println()
	fmt.Println("Step 2: Enter your API key")

	var apiKey string
	if os.Getenv(envKeyName) != "" {
		fmt.Printf("  %s is already set in your environment.\n", envKeyName)
		keepExisting := promptInput(scanner, "Keep existing key? (y/n)", "y")
		if strings.ToLower(keepExisting) == "y" || keepExisting == "" {
			apiKey = os.Getenv(envKeyName)
		}
	}

	if apiKey == "" {
		if selectedProvider != "custom" {
			p := GetProvider(selectedProvider)
			if p != nil {
				for _, def := range config.OptionalEnvVars {
					if def.Category == "provider" && def.URL != "" {
						if strings.Contains(strings.ToLower(def.Description), strings.ToLower(p.Name)) {
							fmt.Printf("  Get your key at: %s\n", def.URL)
							break
						}
					}
				}
			}
		}
		apiKey = promptInput(scanner, fmt.Sprintf("Enter API key (%s)", envKeyName), "")
		if apiKey == "" {
			fmt.Println("  Skipping API key (you can set it later in ~/.hermes/.env)")
		}
	}

	// --- Step 3: Choose default model ---
	fmt.Println()
	fmt.Println("Step 3: Choose your default model")

	models := ListModelsByProvider(selectedProvider)
	if len(models) == 0 {
		// Show all models if provider has no catalog entries.
		models = ModelCatalog
	}

	maxShow := 8
	if len(models) < maxShow {
		maxShow = len(models)
	}
	for i := 0; i < maxShow; i++ {
		m := models[i]
		fmt.Printf("  %d. %s (%s) - $%.2f/$%.2f per 1M tokens\n",
			i+1, m.Name, m.ShortName, m.InputPrice, m.OutputPrice)
	}
	if len(models) > maxShow {
		fmt.Printf("  ... and %d more\n", len(models)-maxShow)
	}
	fmt.Println()

	defaultModel := "anthropic/claude-sonnet-4-20250514"
	if len(models) > 0 {
		defaultModel = models[0].Name
	}

	modelChoice := promptInput(scanner, "Select model (number or name)", "1")
	selectedModel := defaultModel

	if modelIdx := 0; func() bool {
		_, err := fmt.Sscanf(modelChoice, "%d", &modelIdx)
		return err == nil && modelIdx >= 1 && modelIdx <= len(models)
	}() {
		selectedModel = models[modelIdx-1].Name
	} else if modelChoice != "" && modelChoice != "1" {
		selectedModel = ResolveModelName(modelChoice)
	}

	fmt.Printf("  Model: %s\n", selectedModel)

	// --- Step 4: Optional messaging setup ---
	fmt.Println()
	fmt.Println("Step 4: Configure messaging platforms (optional)")

	messagingTokens := map[string]string{}

	setupMessaging := promptInput(scanner, "Configure messaging? (y/n)", "n")
	if strings.ToLower(setupMessaging) == "y" {
		platforms := []struct {
			name   string
			envKey string
			url    string
		}{
			{"Telegram", "TELEGRAM_BOT_TOKEN", "https://t.me/BotFather"},
			{"Discord", "DISCORD_BOT_TOKEN", "https://discord.com/developers/applications"},
			{"Slack", "SLACK_BOT_TOKEN", ""},
		}

		for _, plat := range platforms {
			fmt.Println()
			setup := promptInput(scanner, fmt.Sprintf("Configure %s? (y/n)", plat.name), "n")
			if strings.ToLower(setup) != "y" {
				continue
			}
			if plat.url != "" {
				fmt.Printf("  Get your token at: %s\n", plat.url)
			}
			token := promptInput(scanner, fmt.Sprintf("  Enter %s token", plat.name), "")
			if token != "" {
				messagingTokens[plat.envKey] = token
			}

			// Slack also needs an app token.
			if plat.name == "Slack" && token != "" {
				appToken := promptInput(scanner, "  Enter Slack App Token (SLACK_APP_TOKEN)", "")
				if appToken != "" {
					messagingTokens["SLACK_APP_TOKEN"] = appToken
				}
			}
		}
	}

	// --- Step 5: Save configuration ---
	fmt.Println()
	fmt.Println("Step 5: Saving configuration")

	// Save config.yaml.
	cfg := config.Load()
	cfg.Model = selectedModel
	cfg.Provider = selectedProvider
	if selectedProvider == "custom" {
		cfg.BaseURL = selectedBaseURL
	}

	if err := config.Save(cfg); err != nil {
		fmt.Printf("  Error saving config.yaml: %v\n", err)
	} else {
		fmt.Printf("  Saved config.yaml\n")
	}

	// Save .env file (append, do not overwrite existing keys).
	envPath := filepath.Join(config.HermesHome(), ".env")
	envLines := loadExistingEnvLines(envPath)

	if apiKey != "" {
		envLines = setEnvLine(envLines, envKeyName, apiKey)
	}
	for key, value := range messagingTokens {
		envLines = setEnvLine(envLines, key, value)
	}

	if err := os.WriteFile(envPath, []byte(strings.Join(envLines, "\n")+"\n"), 0600); err != nil {
		fmt.Printf("  Error saving .env: %v\n", err)
	} else {
		fmt.Printf("  Saved .env\n")
	}

	// Create default SOUL.md.
	if err := EnsureDefaultSoul(); err != nil {
		fmt.Printf("  Error creating SOUL.md: %v\n", err)
	} else {
		fmt.Printf("  Ensured SOUL.md\n")
	}

	// Done.
	fmt.Println()
	fmt.Println("Setup complete! Run 'hermes' to start.")
	fmt.Println("Run 'hermes doctor' to verify your configuration.")
}

// promptInput displays a prompt and reads a line of input.
// Returns the default value if the user enters an empty string.
func promptInput(scanner *bufio.Scanner, prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("  %s: ", prompt)
	}

	if !scanner.Scan() {
		return defaultVal
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return defaultVal
	}
	return input
}

// loadExistingEnvLines reads an .env file into lines, preserving comments.
func loadExistingEnvLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	// Remove trailing empty line if present.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// setEnvLine updates or appends a key=value pair in .env lines.
func setEnvLine(lines []string, key, value string) []string {
	prefix := key + "="
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comments.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) || strings.HasPrefix(trimmed, "export "+prefix) {
			lines[i] = key + "=" + value
			return lines
		}
	}
	// Not found -- append.
	return append(lines, key+"="+value)
}
