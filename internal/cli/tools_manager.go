package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/toolsets"
)

// RunToolsManager handles the /tools subcommands: list, enable, disable, resolve.
// It modifies the config to persist toolset enable/disable choices.
func RunToolsManager(cfg *config.Config, args []string) {
	if len(args) == 0 {
		toolsManagerList(cfg)
		return
	}

	switch args[0] {
	case "list":
		toolsManagerList(cfg)

	case "enable":
		if len(args) < 2 {
			fmt.Println("Usage: tools enable <toolset>")
			return
		}
		toolsManagerEnable(cfg, args[1])

	case "disable":
		if len(args) < 2 {
			fmt.Println("Usage: tools disable <toolset>")
			return
		}
		toolsManagerDisable(cfg, args[1])

	case "resolve":
		if len(args) < 2 {
			fmt.Println("Usage: tools resolve <toolset>")
			return
		}
		toolsManagerResolve(args[1])

	default:
		fmt.Printf("Unknown tools subcommand: %s\n", args[0])
		fmt.Println("Available: list, enable, disable, resolve")
	}
}

// toolsManagerList shows all registered toolsets grouped by category with
// enable/disable status from the current config.
func toolsManagerList(cfg *config.Config) {
	allTS := toolsets.GetAllToolsets()

	enabledSet := make(map[string]bool)
	for _, ts := range cfg.Toolsets.Enabled {
		enabledSet[ts] = true
	}
	disabledSet := make(map[string]bool)
	for _, ts := range cfg.Toolsets.Disabled {
		disabledSet[ts] = true
	}

	// Sort toolset names for stable output.
	names := make([]string, 0, len(allTS))
	for name := range allTS {
		names = append(names, name)
	}
	sort.Strings(names)

	// Categorize: basic toolsets, scenario toolsets, platform toolsets.
	var basic, scenario, platform []string
	for _, name := range names {
		switch {
		case strings.HasPrefix(name, "hermes-"):
			platform = append(platform, name)
		case hasIncludes(allTS[name]):
			scenario = append(scenario, name)
		default:
			basic = append(basic, name)
		}
	}

	fmt.Println("Toolsets")
	fmt.Println("========")
	fmt.Println()

	printToolsetGroup("Basic Toolsets", basic, allTS, enabledSet, disabledSet)
	printToolsetGroup("Scenario Toolsets", scenario, allTS, enabledSet, disabledSet)
	printToolsetGroup("Platform Toolsets", platform, allTS, enabledSet, disabledSet)
}

func printToolsetGroup(header string, names []string, allTS map[string]map[string]any, enabled, disabled map[string]bool) {
	if len(names) == 0 {
		return
	}

	fmt.Printf("  %s:\n", header)
	for _, name := range names {
		info := allTS[name]
		desc, _ := info["description"].(string)
		tls, _ := info["tools"].([]string)
		toolCount := len(tls)

		status := "  "
		if enabled[name] {
			status = "+ "
		} else if disabled[name] {
			status = "- "
		}

		fmt.Printf("    %s%-22s  %s (%d tools)\n", status, name, truncateDesc(desc, 50), toolCount)
	}
	fmt.Println()
}

func hasIncludes(info map[string]any) bool {
	includes, ok := info["includes"].([]string)
	return ok && len(includes) > 0
}

func truncateDesc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// toolsManagerEnable adds a toolset to the enabled list and removes it from
// the disabled list if present. Persists to config.yaml.
func toolsManagerEnable(cfg *config.Config, name string) {
	if !toolsets.ValidateToolset(name) {
		fmt.Printf("Unknown toolset: %s\n", name)
		fmt.Println("Use 'tools list' to see available toolsets.")
		return
	}

	// Remove from disabled if present.
	cfg.Toolsets.Disabled = removeFromSlice(cfg.Toolsets.Disabled, name)

	// Add to enabled if not already there.
	if !containsStr(cfg.Toolsets.Enabled, name) {
		cfg.Toolsets.Enabled = append(cfg.Toolsets.Enabled, name)
	}

	if err := config.Save(cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		return
	}

	resolved := toolsets.ResolveToolset(name)
	fmt.Printf("Enabled toolset '%s' (%d tools)\n", name, len(resolved))
}

// toolsManagerDisable adds a toolset to the disabled list and removes it from
// the enabled list if present. Persists to config.yaml.
func toolsManagerDisable(cfg *config.Config, name string) {
	if !toolsets.ValidateToolset(name) {
		fmt.Printf("Unknown toolset: %s\n", name)
		fmt.Println("Use 'tools list' to see available toolsets.")
		return
	}

	// Remove from enabled if present.
	cfg.Toolsets.Enabled = removeFromSlice(cfg.Toolsets.Enabled, name)

	// Add to disabled if not already there.
	if !containsStr(cfg.Toolsets.Disabled, name) {
		cfg.Toolsets.Disabled = append(cfg.Toolsets.Disabled, name)
	}

	if err := config.Save(cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		return
	}

	fmt.Printf("Disabled toolset '%s'\n", name)
}

// toolsManagerResolve shows the fully resolved list of tools for a toolset.
func toolsManagerResolve(name string) {
	if !toolsets.ValidateToolset(name) {
		fmt.Printf("Unknown toolset: %s\n", name)
		return
	}

	resolved := toolsets.ResolveToolset(name)
	fmt.Printf("Toolset '%s' resolves to %d tools:\n", name, len(resolved))
	for _, t := range resolved {
		fmt.Printf("  - %s\n", t)
	}
}

// removeFromSlice returns a new slice with all occurrences of val removed.
func removeFromSlice(s []string, val string) []string {
	var result []string
	for _, item := range s {
		if item != val {
			result = append(result, item)
		}
	}
	return result
}

// containsStr checks if a slice contains a string.
func containsStr(s []string, val string) bool {
	for _, item := range s {
		if item == val {
			return true
		}
	}
	return false
}
