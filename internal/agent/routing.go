package agent

import (
	"strings"
	"unicode/utf8"
)

// SmartRouter decides whether a message is simple enough to route to a cheaper model.
type SmartRouter struct {
	CheapModel string // e.g. "openai/gpt-4o-mini"
	Threshold  int    // max character count for "simple" messages (default 200)
	Enabled    bool
}

// DefaultSmartRouter returns a SmartRouter with sensible defaults.
func DefaultSmartRouter() *SmartRouter {
	return &SmartRouter{
		CheapModel: "openai/gpt-4o-mini",
		Threshold:  200,
		Enabled:    false,
	}
}

// ShouldUseSmartModel returns true if the message is short/simple enough
// to route to the cheaper model.
func (r *SmartRouter) ShouldUseSmartModel(message string) bool {
	if !r.Enabled || r.CheapModel == "" {
		return false
	}

	msg := strings.TrimSpace(message)
	charCount := utf8.RuneCountInString(msg)

	// Length gate: must be under threshold.
	if charCount > r.Threshold {
		return false
	}

	// Heuristic: messages with code fences, bullet lists, or multi-line
	// structure are complex enough to keep on the primary model.
	if strings.Contains(msg, "```") {
		return false
	}
	if strings.Count(msg, "\n") > 3 {
		return false
	}

	// Messages that explicitly request tools or deep work stay on the
	// primary model.
	lower := strings.ToLower(msg)
	complexKeywords := []string{
		"write code", "implement", "refactor", "debug", "analyze",
		"create a file", "run the command", "execute", "deploy",
		"search the codebase", "investigate",
	}
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			return false
		}
	}

	return true
}

// SmartModelRoutingConfig mirrors the config.yaml section.
type SmartModelRoutingConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CheapModel string `yaml:"cheap_model"`
	Threshold  int    `yaml:"threshold"`
}
