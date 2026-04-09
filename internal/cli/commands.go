// Package cli implements the interactive CLI application for Hermes Agent.
package cli

import (
	"sort"
	"strings"
)

// CommandDef defines a single slash command.
type CommandDef struct {
	Name            string   // Canonical name without slash: "background"
	Description     string   // Human-readable description
	Category        string   // "Session", "Configuration", etc.
	Aliases         []string // Alternative names: ["bg"]
	ArgsHint        string   // Argument placeholder: "<prompt>", "[name]"
	Subcommands     []string // Tab-completable subcommands
	CLIOnly         bool     // Only available in CLI
	GatewayOnly     bool     // Only available in gateway/messaging
	GatewayConfGate string   // Config dotpath; when truthy, overrides CLIOnly for gateway
}

// AllNames returns the canonical name plus all aliases.
func (c *CommandDef) AllNames() []string {
	names := make([]string, 0, 1+len(c.Aliases))
	names = append(names, c.Name)
	names = append(names, c.Aliases...)
	return names
}

// CommandRegistry is the central registry of all slash commands.
var CommandRegistry = []CommandDef{
	// Session
	{Name: "new", Description: "Start a new session (fresh session ID + history)", Category: "Session", Aliases: []string{"reset"}},
	{Name: "clear", Description: "Clear screen and start a new session", Category: "Session", CLIOnly: true},
	{Name: "history", Description: "Show conversation history", Category: "Session", CLIOnly: true},
	{Name: "save", Description: "Save the current conversation", Category: "Session", CLIOnly: true},
	{Name: "retry", Description: "Retry the last message (resend to agent)", Category: "Session"},
	{Name: "undo", Description: "Remove the last user/assistant exchange", Category: "Session"},
	{Name: "title", Description: "Set a title for the current session", Category: "Session", ArgsHint: "[name]"},
	{Name: "branch", Description: "Branch the current session (explore a different path)", Category: "Session", Aliases: []string{"fork"}, ArgsHint: "[name]"},
	{Name: "compress", Description: "Manually compress conversation context", Category: "Session"},
	{Name: "rollback", Description: "List or restore filesystem checkpoints", Category: "Session", ArgsHint: "[number]"},
	{Name: "stop", Description: "Kill all running background processes", Category: "Session"},
	{Name: "approve", Description: "Approve a pending dangerous command", Category: "Session", GatewayOnly: true, ArgsHint: "[session|always]"},
	{Name: "deny", Description: "Deny a pending dangerous command", Category: "Session", GatewayOnly: true},
	{Name: "background", Description: "Run a prompt in the background", Category: "Session", Aliases: []string{"bg"}, ArgsHint: "<prompt>"},
	{Name: "btw", Description: "Ephemeral side question using session context (no tools, not persisted)", Category: "Session", ArgsHint: "<question>"},
	{Name: "queue", Description: "Queue a prompt for the next turn (doesn't interrupt)", Category: "Session", Aliases: []string{"q"}, ArgsHint: "<prompt>"},
	{Name: "status", Description: "Show session info", Category: "Session", GatewayOnly: true},
	{Name: "profile", Description: "Show active profile name and home directory", Category: "Info"},
	{Name: "sethome", Description: "Set this chat as the home channel", Category: "Session", GatewayOnly: true, Aliases: []string{"set-home"}},
	{Name: "resume", Description: "Resume a previously-named session", Category: "Session", ArgsHint: "[name]"},

	// Configuration
	{Name: "config", Description: "Show current configuration", Category: "Configuration", CLIOnly: true},
	{Name: "model", Description: "Switch model for this session", Category: "Configuration", ArgsHint: "[model] [--global]"},
	{Name: "provider", Description: "Show available providers and current provider", Category: "Configuration"},
	{Name: "prompt", Description: "View/set custom system prompt", Category: "Configuration", CLIOnly: true, ArgsHint: "[text]", Subcommands: []string{"clear"}},
	{Name: "personality", Description: "Set a predefined personality", Category: "Configuration", ArgsHint: "[name]"},
	{Name: "statusbar", Description: "Toggle the context/model status bar", Category: "Configuration", CLIOnly: true, Aliases: []string{"sb"}},
	{
		Name: "verbose", Description: "Cycle tool progress display: off -> new -> all -> verbose",
		Category: "Configuration", CLIOnly: true,
		GatewayConfGate: "display.tool_progress_command",
	},
	{Name: "yolo", Description: "Toggle YOLO mode (skip all dangerous command approvals)", Category: "Configuration"},
	{
		Name: "reasoning", Description: "Manage reasoning effort and display",
		Category: "Configuration", ArgsHint: "[level|show|hide]",
		Subcommands: []string{"none", "low", "minimal", "medium", "high", "xhigh", "show", "hide", "on", "off"},
	},
	{Name: "skin", Description: "Show or change the display skin/theme", Category: "Configuration", CLIOnly: true, ArgsHint: "[name]"},
	{
		Name: "voice", Description: "Toggle voice mode",
		Category: "Configuration", ArgsHint: "[on|off|tts|status]",
		Subcommands: []string{"on", "off", "tts", "status"},
	},

	// Tools & Skills
	{Name: "tools", Description: "Manage tools: /tools [list|disable|enable] [name...]", Category: "Tools & Skills", ArgsHint: "[list|disable|enable] [name...]", CLIOnly: true},
	{Name: "toolsets", Description: "List available toolsets", Category: "Tools & Skills", CLIOnly: true},
	{
		Name: "skills", Description: "Search, install, inspect, or manage skills",
		Category: "Tools & Skills", CLIOnly: true,
		Subcommands: []string{"search", "browse", "inspect", "install"},
	},
	{
		Name: "cron", Description: "Manage scheduled tasks",
		Category: "Tools & Skills", CLIOnly: true, ArgsHint: "[subcommand]",
		Subcommands: []string{"list", "add", "create", "edit", "pause", "resume", "run", "remove"},
	},
	{Name: "reload-mcp", Description: "Reload MCP servers from config", Category: "Tools & Skills", Aliases: []string{"reload_mcp"}},
	{
		Name: "browser", Description: "Connect browser tools to your live Chrome via CDP",
		Category: "Tools & Skills", CLIOnly: true, ArgsHint: "[connect|disconnect|status]",
		Subcommands: []string{"connect", "disconnect", "status"},
	},
	{Name: "plugins", Description: "List installed plugins and their status", Category: "Tools & Skills", CLIOnly: true},

	// Info
	{Name: "commands", Description: "Browse all commands and skills (paginated)", Category: "Info", GatewayOnly: true, ArgsHint: "[page]"},
	{Name: "help", Description: "Show available commands", Category: "Info"},
	{Name: "usage", Description: "Show token usage for the current session", Category: "Info"},
	{Name: "insights", Description: "Show usage insights and analytics", Category: "Info", ArgsHint: "[days]"},
	{Name: "platforms", Description: "Show gateway/messaging platform status", Category: "Info", CLIOnly: true, Aliases: []string{"gateway"}},
	{Name: "paste", Description: "Check clipboard for an image and attach it", Category: "Info", CLIOnly: true},
	{Name: "update", Description: "Update Hermes Agent to the latest version", Category: "Info", GatewayOnly: true},

	// Exit
	{Name: "quit", Description: "Exit the CLI", Category: "Exit", CLIOnly: true, Aliases: []string{"exit"}},
}

// commandLookup maps every name and alias to its CommandDef index.
var commandLookup map[string]int

func init() {
	rebuildLookups()
}

func rebuildLookups() {
	commandLookup = make(map[string]int, len(CommandRegistry)*2)
	for i, cmd := range CommandRegistry {
		commandLookup[cmd.Name] = i
		for _, alias := range cmd.Aliases {
			commandLookup[alias] = i
		}
	}
}

// ResolveCommand resolves a command name or alias to its canonical name.
// Accepts names with or without the leading slash.
// Returns (canonical_name, found).
func ResolveCommand(input string) (string, bool) {
	name := strings.ToLower(strings.TrimLeft(input, "/"))
	idx, ok := commandLookup[name]
	if !ok {
		return "", false
	}
	return CommandRegistry[idx].Name, true
}

// GetCommandDef returns the CommandDef for a name or alias.
func GetCommandDef(input string) *CommandDef {
	name := strings.ToLower(strings.TrimLeft(input, "/"))
	idx, ok := commandLookup[name]
	if !ok {
		return nil
	}
	cmd := CommandRegistry[idx]
	return &cmd
}

// GetCommandsByCategory returns commands grouped by category.
func GetCommandsByCategory() map[string][]CommandDef {
	result := make(map[string][]CommandDef)
	for _, cmd := range CommandRegistry {
		result[cmd.Category] = append(result[cmd.Category], cmd)
	}
	return result
}

// GetCLICommands returns only commands available in the CLI.
func GetCLICommands() []CommandDef {
	var result []CommandDef
	for _, cmd := range CommandRegistry {
		if !cmd.GatewayOnly {
			result = append(result, cmd)
		}
	}
	return result
}

// GetGatewayKnownCommands returns the set of all command names + aliases
// recognized by the gateway.
func GetGatewayKnownCommands() map[string]bool {
	result := make(map[string]bool)
	for _, cmd := range CommandRegistry {
		if cmd.CLIOnly && cmd.GatewayConfGate == "" {
			continue
		}
		result[cmd.Name] = true
		for _, alias := range cmd.Aliases {
			result[alias] = true
		}
	}
	return result
}

// GatewayHelpLines generates gateway help text lines from the registry.
func GatewayHelpLines() []string {
	var lines []string
	for _, cmd := range CommandRegistry {
		if cmd.CLIOnly && cmd.GatewayConfGate == "" {
			continue
		}
		args := ""
		if cmd.ArgsHint != "" {
			args = " " + cmd.ArgsHint
		}
		aliasNote := ""
		if len(cmd.Aliases) > 0 {
			var parts []string
			for _, a := range cmd.Aliases {
				parts = append(parts, "`/"+a+"`")
			}
			aliasNote = " (alias: " + strings.Join(parts, ", ") + ")"
		}
		lines = append(lines, "`/"+cmd.Name+args+"` -- "+cmd.Description+aliasNote)
	}
	return lines
}

// CommandCategories returns sorted list of unique category names.
func CommandCategories() []string {
	catSet := make(map[string]bool)
	for _, cmd := range CommandRegistry {
		catSet[cmd.Category] = true
	}
	cats := make([]string, 0, len(catSet))
	for c := range catSet {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	return cats
}
