package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Version constants (set at build time via ldflags).
var (
	Version     = "dev"
	ReleaseDate = "unknown"
)

// Banner text art for the Hermes Agent logo (simplified for terminal).
const hermesBannerText = `
 _   _ _____ ____  __  __ _____ ____       _    ____ _____ _   _ _____
| | | | ____|  _ \|  \/  | ____/ ___|     / \  / ___| ____| \ | |_   _|
| |_| |  _| | |_) | |\/| |  _| \___ \    / _ \| |  _|  _| |  \| | | |
|  _  | |___|  _ <| |  | | |___ ___) |  / ___ \ |_| | |___| |\  | | |
|_| |_|_____|_| \_\_|  |_|_____|____/  /_/   \_\____|_____|_| \_| |_|
`

// PrintWelcomeBanner prints the welcome banner to stdout using Lip Gloss styles.
func PrintWelcomeBanner(model, sessionID string) {
	skin := GetActiveSkin()

	titleColor := skin.GetColor("banner_title", "#FFD700")
	borderColor := skin.GetColor("banner_border", "#CD7F32")
	accentColor := skin.GetColor("banner_accent", "#FFBF00")
	dimColor := skin.GetColor("banner_dim", "#B8860B")
	textColor := skin.GetColor("banner_text", "#FFF8DC")

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleColor)).
		Bold(true)

	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(borderColor))

	accentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(dimColor))

	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(textColor))

	agentName := skin.GetBranding("agent_name", "Hermes Agent")

	// Print logo.
	fmt.Println(titleStyle.Render(hermesBannerText))

	// Build info section.
	var info strings.Builder

	// Version line.
	versionLine := fmt.Sprintf("%s v%s (%s)", agentName, Version, ReleaseDate)
	info.WriteString(titleStyle.Render(versionLine))
	info.WriteString("\n")

	// Model line.
	modelShort := model
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		modelShort = model[idx+1:]
	}
	if len(modelShort) > 28 {
		modelShort = modelShort[:25] + "..."
	}
	info.WriteString(accentStyle.Render("Model: "))
	info.WriteString(textStyle.Render(modelShort))
	info.WriteString("\n")

	// Working directory.
	cwd, _ := os.Getwd()
	info.WriteString(dimStyle.Render("Dir:   "))
	info.WriteString(dimStyle.Render(cwd))
	info.WriteString("\n")

	// Session.
	if sessionID != "" {
		info.WriteString(dimStyle.Render("Session: "))
		info.WriteString(dimStyle.Render(sessionID))
		info.WriteString("\n")
	}

	// Welcome message.
	welcome := skin.GetBranding("welcome", "Welcome to Hermes Agent! Type your message or /help for commands.")
	info.WriteString("\n")
	info.WriteString(textStyle.Render(welcome))

	// Border box.
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(0, 2).
		Width(80)

	fmt.Println(boxStyle.Render(info.String()))

	_ = borderStyle // used for consistency
}

// PrintCompactBanner prints a minimal one-line banner.
func PrintCompactBanner(model string) {
	skin := GetActiveSkin()
	agentName := skin.GetBranding("agent_name", "Hermes Agent")

	modelShort := model
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		modelShort = model[idx+1:]
	}

	titleColor := skin.GetColor("banner_title", "#FFD700")
	dimColor := skin.GetColor("banner_dim", "#B8860B")

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(titleColor)).
		Bold(true)
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(dimColor))

	fmt.Printf("%s %s\n",
		titleStyle.Render(agentName),
		dimStyle.Render("("+modelShort+")"),
	)
}

// PrintGoodbye prints the goodbye message.
func PrintGoodbye() {
	skin := GetActiveSkin()
	goodbye := skin.GetBranding("goodbye", "Goodbye!")
	dimColor := skin.GetColor("banner_dim", "#B8860B")
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(dimColor))
	fmt.Println(dimStyle.Render(goodbye))
}

// PrintHelp prints the help text for all available commands.
func PrintHelp() {
	skin := GetActiveSkin()
	header := skin.GetBranding("help_header", "(^_^)? Available Commands")
	accentColor := skin.GetColor("banner_accent", "#FFBF00")
	dimColor := skin.GetColor("banner_dim", "#B8860B")
	textColor := skin.GetColor("banner_text", "#FFF8DC")

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accentColor)).
		Bold(true)
	cmdStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(textColor))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(dimColor))

	fmt.Println()
	fmt.Println(headerStyle.Render(header))
	fmt.Println()

	byCategory := GetCommandsByCategory()
	for _, cat := range CommandCategories() {
		cmds := byCategory[cat]
		fmt.Println(headerStyle.Render("  " + cat))
		for _, cmd := range cmds {
			if cmd.GatewayOnly {
				continue
			}
			name := "/" + cmd.Name
			if cmd.ArgsHint != "" {
				name += " " + cmd.ArgsHint
			}
			aliasStr := ""
			if len(cmd.Aliases) > 0 {
				aliasStr = " (alias: /" + strings.Join(cmd.Aliases, ", /") + ")"
			}
			fmt.Printf("    %s  %s%s\n",
				cmdStyle.Render(fmt.Sprintf("%-30s", name)),
				descStyle.Render(cmd.Description),
				descStyle.Render(aliasStr),
			)
		}
		fmt.Println()
	}
}
