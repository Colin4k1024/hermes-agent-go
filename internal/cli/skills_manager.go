package cli

import (
	"fmt"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
	"github.com/hermes-agent/hermes-agent-go/internal/skills"
)

// RunSkillsSearch searches the hub and local skills for a query string.
func RunSkillsSearch(query string) {
	if query == "" {
		fmt.Println("Usage: skills search <query>")
		return
	}

	fmt.Printf("Searching for '%s'...\n\n", query)

	// Search local installed skills first.
	allSkills, err := skills.LoadAllSkills()
	if err != nil {
		fmt.Printf("Warning: could not load local skills: %v\n", err)
	}

	localMatches := searchLocalSkills(allSkills, query)
	if len(localMatches) > 0 {
		fmt.Printf("  Local matches (%d):\n", len(localMatches))
		for _, s := range localMatches {
			desc := s.Meta.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("    /%s  -  %s\n", s.Meta.Name, desc)
		}
		fmt.Println()
	}

	// Search hub sources.
	hubResults, err := skills.SearchHub(query)
	if err != nil {
		fmt.Printf("Hub search failed: %v\n", err)
		return
	}

	if len(hubResults) > 0 {
		fmt.Printf("  Hub results (%d):\n", len(hubResults))
		for _, h := range hubResults {
			desc := h.Description
			if desc == "" {
				desc = "(no description)"
			}
			trust := ""
			if h.Trust != "" {
				trust = fmt.Sprintf(" [%s]", h.Trust)
			}
			fmt.Printf("    %-25s  %s%s\n", h.Name, desc, trust)
		}
		fmt.Println()
		fmt.Println("Install with: hermes skills install <name>")
	}

	if len(localMatches) == 0 && len(hubResults) == 0 {
		fmt.Println("  No skills found matching your query.")
	}
}

// RunSkillsInstall installs a skill from the hub by name.
func RunSkillsInstall(name string) {
	if name == "" {
		fmt.Println("Usage: skills install <name>")
		return
	}

	fmt.Printf("Searching hub for '%s'...\n", name)

	// Search hub for the exact skill.
	results, err := skills.SearchHub(name)
	if err != nil {
		fmt.Printf("Hub search failed: %v\n", err)
		return
	}

	// Find exact match or first result.
	var target *skills.HubSkill
	for i, r := range results {
		if strings.EqualFold(r.Name, name) {
			target = &results[i]
			break
		}
	}
	if target == nil && len(results) > 0 {
		target = &results[0]
	}

	if target == nil {
		fmt.Printf("Skill '%s' not found in hub.\n", name)
		return
	}

	// Determine the download URL.
	sourceURL := target.URL
	if sourceURL == "" {
		fmt.Printf("No download URL available for '%s'.\n", name)
		return
	}

	// Append SKILL.md if it looks like a directory URL.
	if !strings.HasSuffix(sourceURL, ".md") {
		sourceURL += "/SKILL.md"
	}

	fmt.Printf("Installing '%s' from %s...\n", target.Name, target.Source)

	if err := skills.InstallFromHub(target.Name, sourceURL); err != nil {
		fmt.Printf("Installation failed: %v\n", err)
		return
	}

	fmt.Printf("Skill '%s' installed successfully.\n", target.Name)
	fmt.Printf("Path: %s/skills/%s/\n", config.HermesHome(), target.Name)
	fmt.Printf("Use it with: /%s\n", target.Name)
}

// RunSkillsInspect shows full details about an installed skill.
func RunSkillsInspect(name string) {
	if name == "" {
		fmt.Println("Usage: skills inspect <name>")
		return
	}

	skill, err := skills.FindSkill(name)
	if err != nil {
		fmt.Printf("Skill not found: %v\n", err)
		return
	}

	fmt.Println("Skill Details")
	fmt.Println("=============")
	fmt.Println()
	fmt.Printf("  Name:          %s\n", skill.Meta.Name)
	fmt.Printf("  Description:   %s\n", nonEmpty(skill.Meta.Description, "(none)"))
	fmt.Printf("  Version:       %s\n", nonEmpty(skill.Meta.Version, "(unversioned)"))
	fmt.Printf("  Author:        %s\n", nonEmpty(skill.Meta.Author, "(unknown)"))
	fmt.Printf("  Category:      %s\n", nonEmpty(skill.Meta.Category, "general"))
	fmt.Printf("  Tags:          %s\n", formatList(skill.Meta.Tags))
	fmt.Printf("  Platforms:     %s\n", formatList(skill.Meta.Platforms))
	fmt.Printf("  Prerequisites: %s\n", formatList(skill.Meta.Prerequisites))
	fmt.Printf("  Path:          %s\n", skill.Meta.Path)
	fmt.Printf("  Directory:     %s\n", skill.DirName)

	// Show a preview of the skill body.
	if skill.Body != "" {
		preview := skill.Body
		if len(preview) > 500 {
			preview = preview[:500] + "\n... (truncated)"
		}
		fmt.Println()
		fmt.Println("  Content preview:")
		for _, line := range strings.Split(preview, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
}

// RunSkillsList lists all installed skills with metadata, grouped by category.
func RunSkillsList() {
	allSkills, err := skills.LoadAllSkills()
	if err != nil {
		fmt.Printf("Error loading skills: %v\n", err)
		return
	}

	if len(allSkills) == 0 {
		fmt.Println("No skills installed.")
		fmt.Printf("Skills directory: %s/skills/\n", config.HermesHome())
		fmt.Println()
		fmt.Println("Search for skills: hermes skills search <query>")
		fmt.Println("Install a skill:   hermes skills install <name>")
		return
	}

	byCategory := skills.GetSkillsByCategory(allSkills)

	fmt.Printf("Installed Skills (%d total)\n", len(allSkills))
	fmt.Println("==========================")
	fmt.Println()

	for category, catSkills := range byCategory {
		fmt.Printf("  %s (%d):\n", category, len(catSkills))
		for _, s := range catSkills {
			desc := s.Meta.Description
			if desc == "" {
				desc = "(no description)"
			}
			version := ""
			if s.Meta.Version != "" {
				version = fmt.Sprintf(" v%s", s.Meta.Version)
			}
			fmt.Printf("    /%s%s  -  %s\n", s.Meta.Name, version, desc)
		}
		fmt.Println()
	}
}

// --- helpers ---

func searchLocalSkills(allSkills []*skills.SkillEntry, query string) []*skills.SkillEntry {
	q := strings.ToLower(query)
	var matches []*skills.SkillEntry
	for _, s := range allSkills {
		if strings.Contains(strings.ToLower(s.Meta.Name), q) ||
			strings.Contains(strings.ToLower(s.Meta.Description), q) ||
			strings.Contains(strings.ToLower(s.Meta.Category), q) ||
			matchesTags(s.Meta.Tags, q) {
			matches = append(matches, s)
		}
	}
	return matches
}

func matchesTags(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}
