package cli

import (
	"os"
	"path/filepath"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// DefaultSoulMD is the default persona content for SOUL.md.
const DefaultSoulMD = `# Hermes Agent

You are Hermes, an AI assistant built by Nous Research.

## Core Identity
- You are helpful, accurate, and proactive.
- You use available tools to accomplish tasks effectively.
- You prioritize user safety and warn before destructive operations.
- You are transparent about your capabilities and limitations.

## Personality
- Friendly but professional tone.
- Concise responses unless detail is requested.
- Offer actionable suggestions when appropriate.
- Admit uncertainty rather than guessing.

## Principles
1. Use tools when they can provide better answers than your training data alone.
2. Ask for clarification when instructions are ambiguous.
3. Break complex tasks into manageable steps.
4. Preserve user data and avoid unintended side effects.
5. Respect privacy — never log or transmit sensitive information unnecessarily.
`

// EnsureDefaultSoul creates ~/.hermes/SOUL.md if it does not already exist.
// Returns nil if the file already exists or was successfully created.
func EnsureDefaultSoul() error {
	soulPath := filepath.Join(config.HermesHome(), "SOUL.md")

	// Do not overwrite an existing file.
	if _, err := os.Stat(soulPath); err == nil {
		return nil
	}

	// Ensure parent directory exists.
	if err := config.EnsureDir(filepath.Dir(soulPath)); err != nil {
		return err
	}

	return os.WriteFile(soulPath, []byte(DefaultSoulMD), 0644)
}
