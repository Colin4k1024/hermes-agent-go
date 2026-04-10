package agent

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/hermes-agent/hermes-agent-go/internal/llm"
)

// MaxParallelWorkers is the maximum number of concurrent tool executions.
const MaxParallelWorkers = 8

// Tool categorization for parallel execution decisions.
var (
	// neverParallelToolsMap contains tools that must never run in parallel
	// (interactive or state-modifying tools where order matters).
	neverParallelToolsMap = map[string]bool{
		"clarify":        true,
		"delegate_task":  true,
		"memory":         true,
		"cronjob":        true,
		"send_message":   true,
	}

	// parallelSafeToolsMap contains read-only tools that are always safe to parallelize.
	parallelSafeToolsMap = map[string]bool{
		"read_file":       true,
		"search_files":    true,
		"web_search":      true,
		"web_extract":     true,
		"vision_analyze":  true,
		"skills_list":     true,
		"skill_view":      true,
		"session_search":  true,
		"todo":            true,
		"process":         true,
		"ha_get_state":    true,
		"ha_list_entities": true,
		"ha_list_services": true,
	}

	// pathScopedTools are tools that operate on specific file paths.
	// They can be parallelized if their paths don't overlap.
	pathScopedTools = map[string]bool{
		"create_file":  true,
		"write_file":   true,
		"edit_file":    true,
		"read_file":    true,
		"search_files": true,
		"terminal":     true,
	}
)

// ShouldParallelizeToolBatch returns true when a tool-call batch is safe
// to run concurrently, using path overlap detection for file-scoped tools.
func ShouldParallelizeToolBatch(toolCalls []llm.ToolCall) bool {
	if len(toolCalls) <= 1 {
		return false
	}

	var reservedPaths []string

	for _, tc := range toolCalls {
		toolName := tc.Function.Name

		// Never parallelize certain tools
		if neverParallelToolsMap[toolName] {
			return false
		}

		// Path-scoped tools: check for overlap
		if pathScopedTools[toolName] {
			scopePath := extractScopePath(toolName, tc.Function.Arguments)
			if scopePath == "" {
				// Can't determine path → serialize to be safe
				return false
			}
			for _, existing := range reservedPaths {
				if pathsOverlap(scopePath, existing) {
					return false
				}
			}
			reservedPaths = append(reservedPaths, scopePath)
			continue
		}

		// Must be in the safe list
		if !parallelSafeToolsMap[toolName] {
			return false
		}
	}

	return true
}

// extractScopePath returns the normalized absolute file path for a
// path-scoped tool call. Returns empty string if path can't be determined.
func extractScopePath(toolName, argsJSON string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}

	rawPath, ok := args["path"].(string)
	if !ok || strings.TrimSpace(rawPath) == "" {
		// terminal tool uses "working_directory" or "command"
		if toolName == "terminal" {
			if wd, ok := args["working_directory"].(string); ok && wd != "" {
				rawPath = wd
			} else {
				return "" // can't determine scope
			}
		} else {
			return ""
		}
	}

	// Expand and normalize
	if strings.HasPrefix(rawPath, "~") {
		home, _ := os.UserHomeDir()
		rawPath = filepath.Join(home, rawPath[1:])
	}

	if filepath.IsAbs(rawPath) {
		return filepath.Clean(rawPath)
	}

	cwd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(cwd, rawPath))
}

// pathsOverlap returns true when two paths may refer to the same file or subtree.
func pathsOverlap(left, right string) bool {
	if left == "" || right == "" {
		return false
	}

	leftParts := strings.Split(filepath.Clean(left), string(filepath.Separator))
	rightParts := strings.Split(filepath.Clean(right), string(filepath.Separator))

	commonLen := len(leftParts)
	if len(rightParts) < commonLen {
		commonLen = len(rightParts)
	}

	for i := 0; i < commonLen; i++ {
		if leftParts[i] != rightParts[i] {
			return false
		}
	}
	return true
}

// DeduplicateToolCalls removes duplicate (tool_name, arguments) pairs within
// a single turn. Only the first occurrence of each unique pair is kept.
func DeduplicateToolCalls(toolCalls []llm.ToolCall) []llm.ToolCall {
	if len(toolCalls) <= 1 {
		return toolCalls
	}

	type callKey struct {
		Name string
		Args string
	}

	seen := make(map[callKey]bool, len(toolCalls))
	unique := make([]llm.ToolCall, 0, len(toolCalls))
	removed := 0

	for _, tc := range toolCalls {
		key := callKey{Name: tc.Function.Name, Args: tc.Function.Arguments}
		if seen[key] {
			removed++
			slog.Warn("Removed duplicate tool call", "tool", tc.Function.Name)
			continue
		}
		seen[key] = true
		unique = append(unique, tc)
	}

	if removed == 0 {
		return toolCalls
	}
	return unique
}
