// Package toolsets provides the toolset definition and resolution system.
//
// Toolsets group tools together for specific scenarios and can be composed
// from individual tools or other toolsets. Supports recursive resolution
// with cycle detection.
package toolsets

import (
	"sort"
	"sync"

	"github.com/hermes-agent/hermes-agent-go/internal/tools"
)

// CoreTools is the shared tool list for CLI and all messaging platform toolsets.
// Edit this once to update all platforms simultaneously.
var CoreTools = []string{
	// Web
	"web_search", "web_extract",
	// Terminal + process management
	"terminal", "process",
	// File manipulation
	"read_file", "write_file", "patch", "search_files",
	// Vision + image generation
	"vision_analyze", "image_generate",
	// Skills
	"skills_list", "skill_view", "skill_manage",
	// Browser automation
	"browser_navigate", "browser_snapshot", "browser_click",
	"browser_type", "browser_scroll", "browser_back",
	"browser_press", "browser_get_images",
	"browser_vision", "browser_console",
	// Text-to-speech
	"text_to_speech",
	// Planning & memory
	"todo", "memory",
	// Session history search
	"session_search",
	// Clarifying questions
	"clarify",
	// Code execution + delegation
	"execute_code", "delegate_task",
	// Cronjob management
	"cronjob",
	// Cross-platform messaging (gated on gateway running via check_fn)
	"send_message",
	// Home Assistant smart home control (gated on HASS_TOKEN via check_fn)
	"ha_list_entities", "ha_get_state", "ha_list_services", "ha_call_service",
}

// ToolsetDef describes a toolset: direct tools plus included toolsets.
type ToolsetDef struct {
	Description string   `json:"description" yaml:"description"`
	Tools       []string `json:"tools" yaml:"tools"`
	Includes    []string `json:"includes" yaml:"includes"`
}

// Toolsets contains all static toolset definitions.
var Toolsets = map[string]*ToolsetDef{
	// Basic toolsets - individual tool categories
	"web": {
		Description: "Web research and content extraction tools",
		Tools:       []string{"web_search", "web_extract"},
	},
	"search": {
		Description: "Web search only (no content extraction/scraping)",
		Tools:       []string{"web_search"},
	},
	"vision": {
		Description: "Image analysis and vision tools",
		Tools:       []string{"vision_analyze"},
	},
	"image_gen": {
		Description: "Creative generation tools (images)",
		Tools:       []string{"image_generate"},
	},
	"terminal": {
		Description: "Terminal/command execution and process management tools",
		Tools:       []string{"terminal", "process"},
	},
	"moa": {
		Description: "Advanced reasoning and problem-solving tools",
		Tools:       []string{"mixture_of_agents"},
	},
	"skills": {
		Description: "Access, create, edit, and manage skill documents with specialized instructions and knowledge",
		Tools:       []string{"skills_list", "skill_view", "skill_manage"},
	},
	"browser": {
		Description: "Browser automation for web interaction (navigate, click, type, scroll, iframes, hold-click) with web search for finding URLs",
		Tools: []string{
			"browser_navigate", "browser_snapshot", "browser_click",
			"browser_type", "browser_scroll", "browser_back",
			"browser_press", "browser_get_images",
			"browser_vision", "browser_console", "web_search",
		},
	},
	"cronjob": {
		Description: "Cronjob management tool - create, list, update, pause, resume, remove, and trigger scheduled tasks",
		Tools:       []string{"cronjob"},
	},
	"messaging": {
		Description: "Cross-platform messaging: send messages to Telegram, Discord, Slack, SMS, etc.",
		Tools:       []string{"send_message"},
	},
	"rl": {
		Description: "RL training tools for running reinforcement learning on Tinker-Atropos",
		Tools: []string{
			"rl_list_environments", "rl_select_environment",
			"rl_get_current_config", "rl_edit_config",
			"rl_start_training", "rl_check_status",
			"rl_stop_training", "rl_get_results",
			"rl_list_runs", "rl_test_inference",
		},
	},
	"file": {
		Description: "File manipulation tools: read, write, patch (with fuzzy matching), and search (content + files)",
		Tools:       []string{"read_file", "write_file", "patch", "search_files"},
	},
	"tts": {
		Description: "Text-to-speech: convert text to audio with Edge TTS (free), ElevenLabs, or OpenAI",
		Tools:       []string{"text_to_speech"},
	},
	"todo": {
		Description: "Task planning and tracking for multi-step work",
		Tools:       []string{"todo"},
	},
	"memory": {
		Description: "Persistent memory across sessions (personal notes + user profile)",
		Tools:       []string{"memory"},
	},
	"session_search": {
		Description: "Search and recall past conversations with summarization",
		Tools:       []string{"session_search"},
	},
	"clarify": {
		Description: "Ask the user clarifying questions (multiple-choice or open-ended)",
		Tools:       []string{"clarify"},
	},
	"code_execution": {
		Description: "Run Python scripts that call tools programmatically (reduces LLM round trips)",
		Tools:       []string{"execute_code"},
	},
	"delegation": {
		Description: "Spawn subagents with isolated context for complex subtasks",
		Tools:       []string{"delegate_task"},
	},
	"homeassistant": {
		Description: "Home Assistant smart home control and monitoring",
		Tools:       []string{"ha_list_entities", "ha_get_state", "ha_list_services", "ha_call_service"},
	},

	// Scenario-specific toolsets
	"debugging": {
		Description: "Debugging and troubleshooting toolkit",
		Tools:       []string{"terminal", "process"},
		Includes:    []string{"web", "file"},
	},
	"safe": {
		Description: "Safe toolkit without terminal access",
		Includes:    []string{"web", "vision", "image_gen"},
	},

	// Full Hermes toolsets (CLI + messaging platforms)
	"hermes-acp": {
		Description: "Editor integration (VS Code, Zed, JetBrains) -- coding-focused tools without messaging, audio, or clarify UI",
		Tools: []string{
			"web_search", "web_extract",
			"terminal", "process",
			"read_file", "write_file", "patch", "search_files",
			"vision_analyze",
			"skills_list", "skill_view", "skill_manage",
			"browser_navigate", "browser_snapshot", "browser_click",
			"browser_type", "browser_scroll", "browser_back",
			"browser_press", "browser_get_images",
			"browser_vision", "browser_console",
			"todo", "memory",
			"session_search",
			"execute_code", "delegate_task",
		},
	},
	"hermes-api-server": {
		Description: "OpenAI-compatible API server -- full agent tools accessible via HTTP (no interactive UI tools like clarify or send_message)",
		Tools: []string{
			"web_search", "web_extract",
			"terminal", "process",
			"read_file", "write_file", "patch", "search_files",
			"vision_analyze", "image_generate",
			"skills_list", "skill_view", "skill_manage",
			"browser_navigate", "browser_snapshot", "browser_click",
			"browser_type", "browser_scroll", "browser_back",
			"browser_press", "browser_get_images",
			"browser_vision", "browser_console",
			"todo", "memory",
			"session_search",
			"execute_code", "delegate_task",
			"cronjob",
			"ha_list_entities", "ha_get_state", "ha_list_services", "ha_call_service",
		},
	},
	"hermes-cli": {
		Description: "Full interactive CLI toolset - all default tools plus cronjob management",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-telegram": {
		Description: "Telegram bot toolset - full access for personal use (terminal has safety checks)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-discord": {
		Description: "Discord bot toolset - full access (terminal has safety checks via dangerous command approval)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-whatsapp": {
		Description: "WhatsApp bot toolset - similar to Telegram (personal messaging, more trusted)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-slack": {
		Description: "Slack bot toolset - full access for workspace use (terminal has safety checks)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-signal": {
		Description: "Signal bot toolset - encrypted messaging platform (full access)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-homeassistant": {
		Description: "Home Assistant bot toolset - smart home event monitoring and control",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-email": {
		Description: "Email bot toolset - interact with Hermes via email (IMAP/SMTP)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-mattermost": {
		Description: "Mattermost bot toolset - self-hosted team messaging (full access)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-matrix": {
		Description: "Matrix bot toolset - decentralized encrypted messaging (full access)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-dingtalk": {
		Description: "DingTalk bot toolset - enterprise messaging platform (full access)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-feishu": {
		Description: "Feishu/Lark bot toolset - enterprise messaging via Feishu/Lark (full access)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-wecom": {
		Description: "WeCom bot toolset - enterprise WeChat messaging (full access)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-sms": {
		Description: "SMS bot toolset - interact with Hermes via SMS (Twilio)",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-webhook": {
		Description: "Webhook toolset - receive and process external webhook events",
		Tools:       copyStrings(CoreTools),
	},
	"hermes-gateway": {
		Description: "Gateway toolset - union of all messaging platform tools",
		Includes: []string{
			"hermes-telegram", "hermes-discord", "hermes-whatsapp",
			"hermes-slack", "hermes-signal", "hermes-homeassistant",
			"hermes-email", "hermes-sms", "hermes-mattermost",
			"hermes-matrix", "hermes-dingtalk", "hermes-feishu",
			"hermes-wecom", "hermes-webhook",
		},
	},
}

// mu protects runtime modifications to Toolsets.
var mu sync.RWMutex

// ResolveToolset recursively resolves a toolset to its constituent tool names.
// Handles cycle detection via the visited set.
func ResolveToolset(name string) []string {
	visited := make(map[string]bool)
	return resolveToolsetInner(name, visited)
}

func resolveToolsetInner(name string, visited map[string]bool) []string {
	// Special aliases for all tools.
	if name == "all" || name == "*" {
		allTools := make(map[string]bool)
		for tsName := range Toolsets {
			// Use a fresh visited set per branch to avoid cross-branch contamination.
			branchVisited := make(map[string]bool)
			for k, v := range visited {
				branchVisited[k] = v
			}
			resolved := resolveToolsetInner(tsName, branchVisited)
			for _, t := range resolved {
				allTools[t] = true
			}
		}
		return setToSortedSlice(allTools)
	}

	// Cycle / diamond detection.
	if visited[name] {
		return nil
	}
	visited[name] = true

	mu.RLock()
	ts, ok := Toolsets[name]
	mu.RUnlock()

	if !ok {
		// Fall back to tool registry for plugin-provided toolsets.
		return pluginToolsetTools(name)
	}

	toolSet := make(map[string]bool, len(ts.Tools))
	for _, t := range ts.Tools {
		toolSet[t] = true
	}

	for _, includedName := range ts.Includes {
		included := resolveToolsetInner(includedName, visited)
		for _, t := range included {
			toolSet[t] = true
		}
	}

	return setToSortedSlice(toolSet)
}

// pluginToolsetTools returns tools registered by plugins for a given toolset name.
func pluginToolsetTools(name string) []string {
	reg := tools.Registry()
	tsMap := reg.GetToolToToolsetMap()
	var result []string
	for toolName, tsName := range tsMap {
		if tsName == name {
			result = append(result, toolName)
		}
	}
	sort.Strings(result)
	return result
}

// ResolveMultipleToolsets resolves multiple toolsets and combines their tools (deduplicated).
func ResolveMultipleToolsets(names []string) []string {
	allTools := make(map[string]bool)
	for _, name := range names {
		resolved := ResolveToolset(name)
		for _, t := range resolved {
			allTools[t] = true
		}
	}
	return setToSortedSlice(allTools)
}

// GetAllToolsets returns all available toolsets including plugin-registered ones.
func GetAllToolsets() map[string]map[string]any {
	mu.RLock()
	defer mu.RUnlock()

	result := make(map[string]map[string]any, len(Toolsets))
	for name, ts := range Toolsets {
		result[name] = map[string]any{
			"description": ts.Description,
			"tools":       ts.Tools,
			"includes":    ts.Includes,
		}
	}

	// Add plugin-provided toolsets.
	reg := tools.Registry()
	tsMap := reg.GetToolToToolsetMap()
	pluginToolsets := make(map[string][]string)
	for toolName, tsName := range tsMap {
		if _, exists := result[tsName]; !exists {
			pluginToolsets[tsName] = append(pluginToolsets[tsName], toolName)
		}
	}
	for tsName, tsTools := range pluginToolsets {
		sort.Strings(tsTools)
		result[tsName] = map[string]any{
			"description": "Plugin toolset: " + tsName,
			"tools":       tsTools,
			"includes":    []string{},
		}
	}

	return result
}

// ValidateToolset checks if a toolset name is valid.
func ValidateToolset(name string) bool {
	if name == "all" || name == "*" {
		return true
	}

	mu.RLock()
	_, ok := Toolsets[name]
	mu.RUnlock()
	if ok {
		return true
	}

	// Check tool registry for plugin-provided toolsets.
	reg := tools.Registry()
	tsMap := reg.GetToolToToolsetMap()
	for _, tsName := range tsMap {
		if tsName == name {
			return true
		}
	}
	return false
}

// GetToolsetNames returns sorted names of all available toolsets.
func GetToolsetNames() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make(map[string]bool, len(Toolsets))
	for name := range Toolsets {
		names[name] = true
	}

	// Include plugin toolsets.
	reg := tools.Registry()
	tsMap := reg.GetToolToToolsetMap()
	for _, tsName := range tsMap {
		if _, ok := Toolsets[tsName]; !ok {
			names[tsName] = true
		}
	}

	return setToSortedSlice(names)
}

// CreateCustomToolset registers a custom toolset at runtime.
func CreateCustomToolset(name, description string, directTools, includes []string) {
	mu.Lock()
	defer mu.Unlock()

	Toolsets[name] = &ToolsetDef{
		Description: description,
		Tools:       directTools,
		Includes:    includes,
	}
}

// GetToolsetInfo returns detailed information about a toolset including resolved tools.
func GetToolsetInfo(name string) map[string]any {
	mu.RLock()
	ts, ok := Toolsets[name]
	mu.RUnlock()

	if !ok {
		return nil
	}

	resolved := ResolveToolset(name)
	return map[string]any{
		"name":           name,
		"description":    ts.Description,
		"direct_tools":   ts.Tools,
		"includes":       ts.Includes,
		"resolved_tools": resolved,
		"tool_count":     len(resolved),
		"is_composite":   len(ts.Includes) > 0,
	}
}

// --- Helpers ---

func copyStrings(s []string) []string {
	c := make([]string, len(s))
	copy(c, s)
	return c
}

func setToSortedSlice(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
