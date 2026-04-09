package config

import "os"

// EnvVarDef describes an optional environment variable.
type EnvVarDef struct {
	Description string
	Prompt      string
	URL         string
	Password    bool
	Category    string // "provider", "tool", "messaging", "setting"
}

// OptionalEnvVars defines all optional environment variables.
var OptionalEnvVars = map[string]EnvVarDef{
	// Provider keys
	"OPENROUTER_API_KEY": {
		Description: "OpenRouter API key for 200+ models",
		Prompt:      "OpenRouter API Key",
		URL:         "https://openrouter.ai/keys",
		Password:    true,
		Category:    "provider",
	},
	"OPENAI_API_KEY": {
		Description: "OpenAI API key",
		Prompt:      "OpenAI API Key",
		URL:         "https://platform.openai.com/api-keys",
		Password:    true,
		Category:    "provider",
	},
	"ANTHROPIC_API_KEY": {
		Description: "Anthropic API key",
		Prompt:      "Anthropic API Key",
		URL:         "https://console.anthropic.com/",
		Password:    true,
		Category:    "provider",
	},

	// Tool keys
	"EXA_API_KEY": {
		Description: "Exa search API key",
		Prompt:      "Exa API Key",
		URL:         "https://exa.ai",
		Password:    true,
		Category:    "tool",
	},
	"FIRECRAWL_API_KEY": {
		Description: "Firecrawl web scraping API key",
		Prompt:      "Firecrawl API Key",
		URL:         "https://firecrawl.dev",
		Password:    true,
		Category:    "tool",
	},
	"BROWSERBASE_API_KEY": {
		Description: "Browserbase browser automation",
		Prompt:      "Browserbase API Key",
		URL:         "https://browserbase.com",
		Password:    true,
		Category:    "tool",
	},
	"BROWSERBASE_PROJECT_ID": {
		Description: "Browserbase project ID",
		Prompt:      "Browserbase Project ID",
		Category:    "tool",
	},
	"FAL_KEY": {
		Description: "fal.ai image generation API key",
		Prompt:      "fal.ai API Key",
		URL:         "https://fal.ai",
		Password:    true,
		Category:    "tool",
	},
	"HASS_TOKEN": {
		Description: "Home Assistant long-lived access token",
		Prompt:      "Home Assistant Token",
		Password:    true,
		Category:    "tool",
	},
	"HASS_URL": {
		Description: "Home Assistant URL",
		Prompt:      "Home Assistant URL",
		Category:    "tool",
	},

	// Messaging
	"TELEGRAM_BOT_TOKEN": {
		Description: "Telegram bot token",
		Prompt:      "Telegram Bot Token",
		URL:         "https://t.me/BotFather",
		Password:    true,
		Category:    "messaging",
	},
	"DISCORD_BOT_TOKEN": {
		Description: "Discord bot token",
		Prompt:      "Discord Bot Token",
		URL:         "https://discord.com/developers/applications",
		Password:    true,
		Category:    "messaging",
	},
	"SLACK_BOT_TOKEN": {
		Description: "Slack bot token",
		Prompt:      "Slack Bot Token",
		Password:    true,
		Category:    "messaging",
	},
	"SLACK_APP_TOKEN": {
		Description: "Slack app-level token",
		Prompt:      "Slack App Token",
		Password:    true,
		Category:    "messaging",
	},
}

// GetEnv returns the value of an env var, with fallback.
func GetEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// HasEnv returns true if an env var is set and non-empty.
func HasEnv(key string) bool {
	return os.Getenv(key) != ""
}
