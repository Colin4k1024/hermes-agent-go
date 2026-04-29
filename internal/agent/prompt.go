package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/skills"
)

const defaultAgentIdentity = `You are Hermes, a helpful AI assistant built by Nous Research.
You have access to a set of tools that allow you to interact with the user's computer, browse the web, manage files, execute commands, and more.

Key behaviors:
- Be helpful, accurate, and concise
- Use tools when needed to accomplish tasks
- Ask for clarification when instructions are ambiguous
- Be transparent about your capabilities and limitations
- Prioritize user safety — warn before destructive operations

Current date: %s
Platform: %s
Working directory: %s`

const memoryGuidance = `

## Memory System
You have persistent memory across sessions. Use the memory tool to:
- Save important information the user tells you
- Recall past conversations and preferences
- Track ongoing projects and tasks
- Remember user preferences and working style

Memory files are stored in ~/.hermes/memories/:
- MEMORY.md — persistent notes and knowledge
- USER.md — user profile and preferences`

const sessionSearchGuidance = `

## Session Search
You can search past conversations using the session_search tool.
Use it when the user references previous work or asks "what did we do before?"`

var platformHints = map[string]string{
	"cli":      `You are running in an interactive CLI terminal. The user can see your responses in real-time with streaming. You can use rich formatting (markdown, code blocks). The user can interrupt you with Ctrl+C.`,
	"telegram": `You are running as a Telegram bot. Keep responses concise — long messages may be split. Use markdown formatting sparingly. The user can send photos and voice messages.`,
	"discord":  `You are running as a Discord bot. Use Discord-compatible markdown. Responses over 2000 characters will be split. The user can send images and files.`,
	"slack":    `You are running as a Slack bot. Use Slack mrkdwn formatting. Keep responses focused and well-structured.`,
}

func (a *AIAgent) buildSystemPrompt() string {
	if a.ephemeralSystemPrompt != "" {
		if a.skillLoader != nil {
			loaded, err := a.skillLoader.LoadAll(context.Background())
			if err != nil {
				slog.Debug("Failed to load skills from SkillLoader", "error", err)
			} else if len(loaded) > 0 {
				skillsText := skills.BuildSkillsPrompt(loaded)
				return a.ephemeralSystemPrompt + "\n\n## Available Skills\n" + skillsText
			}
		}
		return a.ephemeralSystemPrompt
	}

	var sb strings.Builder

	// Core identity
	cwd, _ := os.Getwd()
	sb.WriteString(fmt.Sprintf(defaultAgentIdentity,
		time.Now().Format("2006-01-02"),
		a.platform,
		cwd,
	))

	// Platform hints
	if hint, ok := platformHints[a.platform]; ok {
		sb.WriteString("\n\n")
		sb.WriteString(hint)
	}

	// Memory guidance
	if !a.skipMemory {
		sb.WriteString(memoryGuidance)
	}

	// Session search guidance
	sb.WriteString(sessionSearchGuidance)

	// Context files
	if !a.skipContextFiles {
		contextFiles := loadContextFiles()
		if contextFiles != "" {
			sb.WriteString("\n\n## Project Context\n")
			sb.WriteString(contextFiles)
		}
	}

	// Soul content (per-tenant, loaded from MinIO in SaaS mode)
	if a.soulContent != "" {
		sb.WriteString("\n\n## Persona\n")
		sb.WriteString(a.soulContent)
	}

	// Memory context (from PG memory provider via SystemPromptProvider)
	if a.memoryProvider != nil {
		if sp, ok := a.memoryProvider.(SystemPromptProvider); ok {
			if block := sp.SystemPromptBlock(); block != "" {
				sb.WriteString("\n\n")
				sb.WriteString(block)
			}
		}
	}

	// Skills guidance — use SkillLoader when available, fallback to local filesystem
	var skillsPrompt string
	if a.skillLoader != nil {
		loaded, err := a.skillLoader.LoadAll(context.Background())
		if err != nil {
			slog.Debug("Failed to load skills from SkillLoader", "error", err)
		} else {
			skillsPrompt = skills.BuildSkillsPrompt(loaded)
		}
	} else {
		skillsPrompt = loadSkillsPrompt()
	}
	if skillsPrompt != "" {
		sb.WriteString("\n\n## Available Skills\n")
		sb.WriteString(skillsPrompt)
	}

	return sb.String()
}

func loadContextFiles() string {
	var parts []string

	// Load SOUL.md
	soulPath := filepath.Join(config.HermesHome(), "SOUL.md")
	if data, err := os.ReadFile(soulPath); err == nil && len(data) > 0 {
		parts = append(parts, fmt.Sprintf("### Persona (SOUL.md)\n%s", string(data)))
	}

	// Load AGENTS.md from current directory
	if data, err := os.ReadFile("AGENTS.md"); err == nil && len(data) > 0 {
		parts = append(parts, fmt.Sprintf("### Project Instructions (AGENTS.md)\n%s", string(data)))
	}

	// Load .cursorrules from current directory
	if data, err := os.ReadFile(".cursorrules"); err == nil && len(data) > 0 {
		parts = append(parts, fmt.Sprintf("### Project Rules (.cursorrules)\n%s", string(data)))
	}

	return strings.Join(parts, "\n\n")
}

func loadSkillsPrompt() string {
	skillsDir := filepath.Join(config.HermesHome(), "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return ""
	}

	var skills []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillMD := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		if _, err := os.Stat(skillMD); err == nil {
			skills = append(skills, fmt.Sprintf("- /%s", entry.Name()))
		}
		// Check subdirectories
		subEntries, err := os.ReadDir(filepath.Join(skillsDir, entry.Name()))
		if err != nil {
			continue
		}
		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}
			subSkillMD := filepath.Join(skillsDir, entry.Name(), sub.Name(), "SKILL.md")
			if _, err := os.Stat(subSkillMD); err == nil {
				skills = append(skills, fmt.Sprintf("- /%s", sub.Name()))
			}
		}
	}

	if len(skills) == 0 {
		return ""
	}

	return fmt.Sprintf("The user can load skills with slash commands. Available skills:\n%s\n\nWhen the user types a skill command, load and follow its instructions.",
		strings.Join(skills, "\n"))
}
