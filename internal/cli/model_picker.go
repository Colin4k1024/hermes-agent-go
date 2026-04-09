package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RunModelPicker shows an interactive model selection UI.
// Lists models from ModelCatalog with pricing info and capabilities.
// Returns the full model name the user selected.
func RunModelPicker(currentModel string) string {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Model Selection")
	fmt.Println("===============")
	fmt.Println()

	if currentModel != "" {
		fmt.Printf("Current model: %s\n\n", currentModel)
	}

	// Group models by provider for display.
	providerOrder := ListCatalogProviders()

	idx := 1
	indexToModel := make(map[int]ModelInfo)

	for _, provider := range providerOrder {
		models := ListModelsByProvider(provider)
		if len(models) == 0 {
			continue
		}

		fmt.Printf("  --- %s ---\n", strings.ToUpper(provider))
		for _, m := range models {
			// Build capability flags.
			caps := buildCapFlags(m)

			// Mark current model.
			marker := "  "
			if m.Name == currentModel {
				marker = "* "
			}

			fmt.Printf("  %s%2d. %-42s  ctx:%-7s  $%.2f/$%.2f  %s\n",
				marker, idx, m.Name,
				formatContext(m.ContextLength),
				m.InputPrice, m.OutputPrice,
				caps,
			)

			indexToModel[idx] = m
			idx++
		}
		fmt.Println()
	}

	fmt.Printf("Select model (1-%d) or name, or press Enter to keep current: ", idx-1)

	if !scanner.Scan() {
		return currentModel
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return currentModel
	}

	// Try numeric selection.
	if num, err := strconv.Atoi(input); err == nil {
		if m, ok := indexToModel[num]; ok {
			fmt.Printf("\nSelected: %s\n", m.Name)
			return m.Name
		}
		fmt.Println("Invalid selection number.")
		return currentModel
	}

	// Try name resolution.
	resolved := ResolveModelName(input)
	fmt.Printf("\nSelected: %s\n", resolved)
	return resolved
}

// buildCapFlags returns a compact string of capability flags for a model.
func buildCapFlags(m ModelInfo) string {
	var flags []string
	if m.SupportsTools {
		flags = append(flags, "tools")
	}
	if m.Vision {
		flags = append(flags, "vision")
	}
	if m.Reasoning {
		flags = append(flags, "reason")
	}
	if len(flags) == 0 {
		return ""
	}
	return "[" + strings.Join(flags, ",") + "]"
}

// formatContext formats a context length for compact display.
func formatContext(ctx int) string {
	if ctx >= 1000000 {
		return fmt.Sprintf("%.0fM", float64(ctx)/1000000)
	}
	if ctx >= 1000 {
		return fmt.Sprintf("%dk", ctx/1000)
	}
	return strconv.Itoa(ctx)
}
