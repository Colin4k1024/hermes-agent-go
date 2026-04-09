package cli

import (
	"os"
	"path/filepath"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"gopkg.in/yaml.v3"
)

// SkinConfig holds the complete skin/theme configuration.
type SkinConfig struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Colors      map[string]string `yaml:"colors"`
	Spinner     SpinnerSkin       `yaml:"spinner"`
	Branding    map[string]string `yaml:"branding"`
	ToolPrefix  string            `yaml:"tool_prefix"`
	ToolEmojis  map[string]string `yaml:"tool_emojis"`
	BannerLogo  string            `yaml:"banner_logo"`
	BannerHero  string            `yaml:"banner_hero"`
}

// SpinnerSkin holds spinner customization for a skin.
type SpinnerSkin struct {
	WaitingFaces  []string   `yaml:"waiting_faces"`
	ThinkingFaces []string   `yaml:"thinking_faces"`
	ThinkingVerbs []string   `yaml:"thinking_verbs"`
	Wings         [][]string `yaml:"wings"`
}

// GetColor returns a color value with fallback.
func (s *SkinConfig) GetColor(key, fallback string) string {
	if v, ok := s.Colors[key]; ok {
		return v
	}
	return fallback
}

// GetBranding returns a branding value with fallback.
func (s *SkinConfig) GetBranding(key, fallback string) string {
	if v, ok := s.Branding[key]; ok {
		return v
	}
	return fallback
}

// GetWings returns spinner wing pairs as [left, right] tuples.
func (s *SkinConfig) GetWings() [][2]string {
	var result [][2]string
	for _, pair := range s.Spinner.Wings {
		if len(pair) == 2 {
			result = append(result, [2]string{pair[0], pair[1]})
		}
	}
	return result
}

// --- Built-in skin definitions ---

var defaultColors = map[string]string{
	"banner_border":   "#CD7F32",
	"banner_title":    "#FFD700",
	"banner_accent":   "#FFBF00",
	"banner_dim":      "#B8860B",
	"banner_text":     "#FFF8DC",
	"ui_accent":       "#FFBF00",
	"ui_label":        "#4dd0e1",
	"ui_ok":           "#4caf50",
	"ui_error":        "#ef5350",
	"ui_warn":         "#ffa726",
	"prompt":          "#FFF8DC",
	"input_rule":      "#CD7F32",
	"response_border": "#FFD700",
	"session_label":   "#DAA520",
	"session_border":  "#8B8682",
}

var defaultBranding = map[string]string{
	"agent_name":     "Hermes Agent",
	"welcome":        "Welcome to Hermes Agent! Type your message or /help for commands.",
	"goodbye":        "Goodbye!",
	"response_label": " Hermes ",
	"prompt_symbol":  "> ",
	"help_header":    "(^_^)? Available Commands",
}

var builtinSkins = map[string]*SkinConfig{
	"default": {
		Name:        "default",
		Description: "Classic Hermes -- gold and kawaii",
		Colors:      defaultColors,
		Branding:    defaultBranding,
		ToolPrefix:  "|",
	},
	"ares": {
		Name:        "ares",
		Description: "War-god theme -- crimson and bronze",
		Colors: map[string]string{
			"banner_border":   "#9F1C1C",
			"banner_title":    "#C7A96B",
			"banner_accent":   "#DD4A3A",
			"banner_dim":      "#6B1717",
			"banner_text":     "#F1E6CF",
			"ui_accent":       "#DD4A3A",
			"ui_label":        "#C7A96B",
			"ui_ok":           "#4caf50",
			"ui_error":        "#ef5350",
			"ui_warn":         "#ffa726",
			"prompt":          "#F1E6CF",
			"input_rule":      "#9F1C1C",
			"response_border": "#C7A96B",
			"session_label":   "#C7A96B",
			"session_border":  "#6E584B",
		},
		Spinner: SpinnerSkin{
			WaitingFaces:  []string{"(x)", "(+)", "(/\\)", "(<>)", "(/)"},
			ThinkingFaces: []string{"(x)", "(+)", "(/\\)", "(-)", "(<>)"},
			ThinkingVerbs: []string{
				"forging", "marching", "sizing the field", "holding the line",
				"hammering plans", "tempering steel", "plotting impact", "raising the shield",
			},
			Wings: [][]string{
				{"<<x", "x>>"},
				{"<</\\", "/\\>>"},
				{"<<-", "->>"},
				{"<<+", "+>>"},
			},
		},
		Branding: map[string]string{
			"agent_name":     "Ares Agent",
			"welcome":        "Welcome to Ares Agent! Type your message or /help for commands.",
			"goodbye":        "Farewell, warrior!",
			"response_label": " Ares ",
			"prompt_symbol":  "x > ",
			"help_header":    "(x) Available Commands",
		},
		ToolPrefix: "|",
	},
	"mono": {
		Name:        "mono",
		Description: "Monochrome -- clean grayscale",
		Colors: map[string]string{
			"banner_border":   "#555555",
			"banner_title":    "#e6edf3",
			"banner_accent":   "#aaaaaa",
			"banner_dim":      "#444444",
			"banner_text":     "#c9d1d9",
			"ui_accent":       "#aaaaaa",
			"ui_label":        "#888888",
			"ui_ok":           "#888888",
			"ui_error":        "#cccccc",
			"ui_warn":         "#999999",
			"prompt":          "#c9d1d9",
			"input_rule":      "#444444",
			"response_border": "#aaaaaa",
			"session_label":   "#888888",
			"session_border":  "#555555",
		},
		Branding: map[string]string{
			"agent_name":     "Hermes Agent",
			"welcome":        "Welcome to Hermes Agent! Type your message or /help for commands.",
			"goodbye":        "Goodbye!",
			"response_label": " Hermes ",
			"prompt_symbol":  "> ",
			"help_header":    "[?] Available Commands",
		},
		ToolPrefix: "|",
	},
	"slate": {
		Name:        "slate",
		Description: "Cool blue -- developer-focused",
		Colors: map[string]string{
			"banner_border":   "#4169e1",
			"banner_title":    "#7eb8f6",
			"banner_accent":   "#8EA8FF",
			"banner_dim":      "#4b5563",
			"banner_text":     "#c9d1d9",
			"ui_accent":       "#7eb8f6",
			"ui_label":        "#8EA8FF",
			"ui_ok":           "#63D0A6",
			"ui_error":        "#F7A072",
			"ui_warn":         "#e6a855",
			"prompt":          "#c9d1d9",
			"input_rule":      "#4169e1",
			"response_border": "#7eb8f6",
			"session_label":   "#7eb8f6",
			"session_border":  "#4b5563",
		},
		Branding: map[string]string{
			"agent_name":     "Hermes Agent",
			"welcome":        "Welcome to Hermes Agent! Type your message or /help for commands.",
			"goodbye":        "Goodbye!",
			"response_label": " Hermes ",
			"prompt_symbol":  "> ",
			"help_header":    "(^_^)? Available Commands",
		},
		ToolPrefix: "|",
	},
}

// --- Skin management ---

var activeSkin *SkinConfig
var activeSkinName = "default"

// LoadSkin loads a skin by name. Checks user skins first, then built-in.
func LoadSkin(name string) *SkinConfig {
	// Check user skins directory.
	skinsDir := filepath.Join(config.HermesHome(), "skins")
	userFile := filepath.Join(skinsDir, name+".yaml")
	if data, err := os.ReadFile(userFile); err == nil {
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err == nil {
			return buildSkinConfig(raw)
		}
	}

	// Check built-in skins.
	if skin, ok := builtinSkins[name]; ok {
		return mergeSkinWithDefaults(skin)
	}

	// Fallback to default.
	return mergeSkinWithDefaults(builtinSkins["default"])
}

// GetActiveSkin returns the currently active skin config.
func GetActiveSkin() *SkinConfig {
	if activeSkin == nil {
		activeSkin = LoadSkin(activeSkinName)
	}
	return activeSkin
}

// SetActiveSkin switches the active skin. Returns the new SkinConfig.
func SetActiveSkin(name string) *SkinConfig {
	activeSkinName = name
	activeSkin = LoadSkin(name)
	return activeSkin
}

// GetActiveSkinName returns the name of the currently active skin.
func GetActiveSkinName() string {
	return activeSkinName
}

// InitSkinFromConfig initializes the active skin from CLI config at startup.
func InitSkinFromConfig(cfg *config.Config) {
	skinName := cfg.Display.Skin
	if skinName == "" {
		skinName = "default"
	}
	SetActiveSkin(skinName)
}

// ListSkins returns all available skins (built-in + user-installed).
func ListSkins() []map[string]string {
	var result []map[string]string
	for name, skin := range builtinSkins {
		result = append(result, map[string]string{
			"name":        name,
			"description": skin.Description,
			"source":      "builtin",
		})
	}

	skinsDir := filepath.Join(config.HermesHome(), "skins")
	entries, err := os.ReadDir(skinsDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(skinsDir, entry.Name()))
			if err != nil {
				continue
			}
			var raw map[string]any
			if err := yaml.Unmarshal(data, &raw); err != nil {
				continue
			}
			skinName, _ := raw["name"].(string)
			if skinName == "" {
				skinName = entry.Name()[:len(entry.Name())-5] // strip .yaml
			}
			// Skip if shadows a built-in.
			if _, exists := builtinSkins[skinName]; exists {
				continue
			}
			desc, _ := raw["description"].(string)
			result = append(result, map[string]string{
				"name":        skinName,
				"description": desc,
				"source":      "user",
			})
		}
	}

	return result
}

// --- Helpers ---

func mergeSkinWithDefaults(skin *SkinConfig) *SkinConfig {
	merged := &SkinConfig{
		Name:        skin.Name,
		Description: skin.Description,
		Colors:      make(map[string]string),
		Branding:    make(map[string]string),
		ToolPrefix:  skin.ToolPrefix,
		Spinner:     skin.Spinner,
		ToolEmojis:  skin.ToolEmojis,
		BannerLogo:  skin.BannerLogo,
		BannerHero:  skin.BannerHero,
	}

	// Merge colors with defaults.
	for k, v := range defaultColors {
		merged.Colors[k] = v
	}
	for k, v := range skin.Colors {
		merged.Colors[k] = v
	}

	// Merge branding with defaults.
	for k, v := range defaultBranding {
		merged.Branding[k] = v
	}
	for k, v := range skin.Branding {
		merged.Branding[k] = v
	}

	if merged.ToolPrefix == "" {
		merged.ToolPrefix = "|"
	}

	return merged
}

func buildSkinConfig(raw map[string]any) *SkinConfig {
	data, _ := yaml.Marshal(raw)
	var skin SkinConfig
	yaml.Unmarshal(data, &skin)
	return mergeSkinWithDefaults(&skin)
}
