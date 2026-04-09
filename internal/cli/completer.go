package cli

import (
	"sort"
	"strings"
)

// Completer provides tab-completion for the CLI.
type Completer struct {
	commands []CommandDef
	skills   []string
}

// NewCompleter creates a new Completer populated from the command registry.
func NewCompleter() *Completer {
	return &Completer{
		commands: CommandRegistry,
		skills:   []string{},
	}
}

// SetSkills updates the known skill names for completion.
func (c *Completer) SetSkills(skills []string) {
	c.skills = skills
}

// Complete returns suggestions for the current input at the given cursor position.
//   - If line starts with "/", suggest matching commands.
//   - If after "/model ", suggest known models.
//   - If after "/skin ", suggest available skins.
//   - If after "/personality ", suggest personalities.
//   - If after "/reasoning ", suggest reasoning levels.
func (c *Completer) Complete(line string, pos int) []string {
	// Only consider text up to the cursor.
	if pos > len(line) {
		pos = len(line)
	}
	prefix := line[:pos]

	// Must start with "/" for command completion.
	if !strings.HasPrefix(prefix, "/") {
		return nil
	}

	// Split into command and arguments.
	parts := strings.SplitN(prefix, " ", 2)
	cmdPart := strings.TrimPrefix(parts[0], "/")

	// If no space yet, complete the command name itself.
	if len(parts) == 1 {
		return c.completeCommandName(cmdPart)
	}

	// We have a command and an argument portion.
	argPrefix := parts[1]

	// Resolve the command to its canonical name.
	canonical, found := ResolveCommand(cmdPart)
	if !found {
		return nil
	}

	return c.completeCommandArgs(canonical, argPrefix)
}

// completeCommandName returns command names matching the given prefix.
func (c *Completer) completeCommandName(prefix string) []string {
	prefix = strings.ToLower(prefix)
	seen := make(map[string]bool)
	var matches []string

	for _, cmd := range c.commands {
		for _, name := range cmd.AllNames() {
			if strings.HasPrefix(strings.ToLower(name), prefix) && !seen[name] {
				seen[name] = true
				matches = append(matches, "/"+name)
			}
		}
	}

	sort.Strings(matches)
	return matches
}

// completeCommandArgs returns argument completions for a specific command.
func (c *Completer) completeCommandArgs(command, argPrefix string) []string {
	argPrefix = strings.ToLower(argPrefix)

	switch command {
	case "model":
		return c.filterPrefix(knownModelNames(), argPrefix)

	case "skin":
		skins := ListSkins()
		var names []string
		for _, s := range skins {
			if name, ok := s["name"]; ok {
				names = append(names, name)
			}
		}
		return c.filterPrefix(names, argPrefix)

	case "personality":
		personalities := []string{
			"default", "concise", "creative", "technical",
			"friendly", "formal", "pirate", "shakespeare",
		}
		return c.filterPrefix(personalities, argPrefix)

	case "reasoning":
		levels := []string{"none", "low", "minimal", "medium", "high", "xhigh", "show", "hide", "on", "off"}
		return c.filterPrefix(levels, argPrefix)

	case "verbose":
		modes := []string{"off", "new", "all", "verbose"}
		return c.filterPrefix(modes, argPrefix)

	default:
		// Try subcommands from the CommandDef.
		def := GetCommandDef(command)
		if def != nil && len(def.Subcommands) > 0 {
			return c.filterPrefix(def.Subcommands, argPrefix)
		}
	}

	return nil
}

// filterPrefix returns items from candidates that start with prefix.
func (c *Completer) filterPrefix(candidates []string, prefix string) []string {
	var matches []string
	for _, item := range candidates {
		if strings.HasPrefix(strings.ToLower(item), prefix) {
			matches = append(matches, item)
		}
	}
	sort.Strings(matches)
	return matches
}

// knownModelNames returns a list of well-known model short names and full names.
func knownModelNames() []string {
	return []string{
		// Short aliases
		"claude-opus", "claude-sonnet", "claude-haiku",
		"gpt-4o", "gpt-4o-mini",
		"o1", "o3",
		"gemini-2.5-pro", "gemini-2.5-flash",
		"deepseek-chat", "deepseek-r1",
		"llama-4-maverick",
		// Full names
		"anthropic/claude-opus-4-20250514",
		"anthropic/claude-sonnet-4-20250514",
		"anthropic/claude-haiku-4-20250414",
		"openai/gpt-4o",
		"openai/gpt-4o-mini",
		"openai/o1",
		"openai/o3",
		"google/gemini-2.5-pro",
		"google/gemini-2.5-flash",
		"deepseek/deepseek-chat",
		"deepseek/deepseek-r1",
	}
}
