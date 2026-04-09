package gateway

import (
	"os"
	"path/filepath"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"gopkg.in/yaml.v3"
)

// SessionConfig holds session-related settings for the gateway.
type SessionConfig struct {
	ExpiryMinutes         int  `yaml:"expiry_minutes"`
	GroupSessionsPerUser  bool `yaml:"group_sessions_per_user"`
	ThreadSessionsPerUser bool `yaml:"thread_sessions_per_user"`
	MaxMessageLength      int  `yaml:"max_message_length"`
}

// GeneralConfig holds general gateway settings.
type GeneralConfig struct {
	Model         string `yaml:"model"`
	MaxIterations int    `yaml:"max_iterations"`
	AutoApprove   bool   `yaml:"auto_approve"`
	ToolDelay     float64 `yaml:"tool_delay"`
}

// GatewayConfigFile is the top-level structure loaded from config.yaml's
// messaging / gateway section.
type GatewayConfigFile struct {
	Platforms    map[string]*PlatformConfigEntry `yaml:"platforms"`
	Sessions     SessionConfig                   `yaml:"sessions"`
	General      GeneralConfig                   `yaml:"general"`
	AllowedUsers map[string]any                  `yaml:"allowed_users"`
}

// PlatformConfigEntry holds per-platform configuration from the config file.
type PlatformConfigEntry struct {
	Enabled bool           `yaml:"enabled"`
	Token   string         `yaml:"token"`
	Extra   map[string]any `yaml:"extra,omitempty"`
}

// DefaultGatewayConfigFile returns a GatewayConfigFile with sensible defaults.
func DefaultGatewayConfigFile() *GatewayConfigFile {
	return &GatewayConfigFile{
		Platforms: make(map[string]*PlatformConfigEntry),
		Sessions: SessionConfig{
			ExpiryMinutes:         60,
			GroupSessionsPerUser:  true,
			ThreadSessionsPerUser: false,
			MaxMessageLength:      4096,
		},
		General: GeneralConfig{
			MaxIterations: 90,
			ToolDelay:     1.0,
		},
	}
}

// LoadGatewayConfig loads the gateway/messaging section from config.yaml and
// merges environment variable tokens for known platforms.
func LoadGatewayConfig() (*GatewayConfigFile, error) {
	gcf := DefaultGatewayConfigFile()

	// Try to load from config.yaml.
	configPath := filepath.Join(config.HermesHome(), "config.yaml")
	data, err := os.ReadFile(configPath)
	if err == nil {
		// Parse only the gateway/messaging sub-key.
		var raw struct {
			Gateway  *GatewayConfigFile `yaml:"gateway"`
			Messaging *GatewayConfigFile `yaml:"messaging"` // alternate key
		}
		if err := yaml.Unmarshal(data, &raw); err == nil {
			if raw.Gateway != nil {
				gcf = mergeGatewayConfig(gcf, raw.Gateway)
			} else if raw.Messaging != nil {
				gcf = mergeGatewayConfig(gcf, raw.Messaging)
			}
		}
	}

	// Overlay tokens from environment variables.
	envTokens := map[string]string{
		"telegram":     "TELEGRAM_BOT_TOKEN",
		"discord":      "DISCORD_BOT_TOKEN",
		"slack":        "SLACK_BOT_TOKEN",
		"whatsapp":     "WHATSAPP_API_TOKEN",
		"signal":       "SIGNAL_CLI_PATH",
		"matrix":       "MATRIX_ACCESS_TOKEN",
		"mattermost":   "MATTERMOST_TOKEN",
		"dingtalk":     "DINGTALK_TOKEN",
		"feishu":       "FEISHU_APP_ID",
		"wecom":        "WECOM_CORP_ID",
		"email":        "EMAIL_IMAP_HOST",
		"sms":          "TWILIO_ACCOUNT_SID",
	}

	for platformName, envKey := range envTokens {
		token := os.Getenv(envKey)
		if token == "" {
			continue
		}
		entry, exists := gcf.Platforms[platformName]
		if !exists {
			entry = &PlatformConfigEntry{Enabled: true}
			gcf.Platforms[platformName] = entry
		}
		if entry.Token == "" {
			entry.Token = token
		}
		// Auto-enable platforms that have a token.
		entry.Enabled = true
	}

	return gcf, nil
}

// GetEnabledPlatforms returns the names of all enabled platforms.
func GetEnabledPlatforms() []string {
	gcf, err := LoadGatewayConfig()
	if err != nil {
		return nil
	}
	var enabled []string
	for name, entry := range gcf.Platforms {
		if entry.Enabled {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// mergeGatewayConfig merges src into dst, preferring non-zero src values.
func mergeGatewayConfig(dst, src *GatewayConfigFile) *GatewayConfigFile {
	if src.Platforms != nil {
		if dst.Platforms == nil {
			dst.Platforms = make(map[string]*PlatformConfigEntry)
		}
		for k, v := range src.Platforms {
			dst.Platforms[k] = v
		}
	}
	if src.Sessions.ExpiryMinutes > 0 {
		dst.Sessions.ExpiryMinutes = src.Sessions.ExpiryMinutes
	}
	if src.Sessions.MaxMessageLength > 0 {
		dst.Sessions.MaxMessageLength = src.Sessions.MaxMessageLength
	}
	dst.Sessions.GroupSessionsPerUser = src.Sessions.GroupSessionsPerUser
	dst.Sessions.ThreadSessionsPerUser = src.Sessions.ThreadSessionsPerUser

	if src.General.Model != "" {
		dst.General.Model = src.General.Model
	}
	if src.General.MaxIterations > 0 {
		dst.General.MaxIterations = src.General.MaxIterations
	}
	if src.General.ToolDelay > 0 {
		dst.General.ToolDelay = src.General.ToolDelay
	}
	dst.General.AutoApprove = src.General.AutoApprove

	if src.AllowedUsers != nil {
		dst.AllowedUsers = src.AllowedUsers
	}

	return dst
}
