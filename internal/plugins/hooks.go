package plugins

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// PluginHook represents a lifecycle hook callback.
type PluginHook func(event *HookEvent) error

// HookEvent carries data through the plugin hook pipeline.
type HookEvent struct {
	Type       string
	ToolName   string
	ToolArgs   map[string]any
	ToolResult string
	SessionID  string
	Messages   any
	Response   any
}

// Hook types
const (
	HookPreToolCall  = "pre_tool_call"
	HookPostToolCall = "post_tool_call"
	HookPreLLMCall   = "pre_llm_call"
	HookPostLLMCall  = "post_llm_call"
	HookSessionStart = "on_session_start"
	HookSessionEnd   = "on_session_end"
)

// PluginHookRegistry manages plugin lifecycle hooks.
type PluginHookRegistry struct {
	mu    sync.RWMutex
	hooks map[string][]registeredHook
}

type registeredHook struct {
	name     string
	priority int
	fn       PluginHook
}

var globalPluginHooks = &PluginHookRegistry{
	hooks: make(map[string][]registeredHook),
}

// GlobalPluginHooks returns the global plugin hook registry.
func GlobalPluginHooks() *PluginHookRegistry { return globalPluginHooks }

// Register adds a hook callback.
func (r *PluginHookRegistry) Register(hookType, name string, priority int, fn PluginHook) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hooks[hookType] = append(r.hooks[hookType], registeredHook{
		name: name, priority: priority, fn: fn,
	})
}

// Fire executes all hooks of the given type in priority order. Stops on first error.
func (r *PluginHookRegistry) Fire(hookType string, event *HookEvent) error {
	r.mu.RLock()
	hooks := make([]registeredHook, len(r.hooks[hookType]))
	copy(hooks, r.hooks[hookType])
	r.mu.RUnlock()

	// Sort by priority (stable, lower = earlier)
	for i := 0; i < len(hooks); i++ {
		for j := i + 1; j < len(hooks); j++ {
			if hooks[j].priority < hooks[i].priority {
				hooks[i], hooks[j] = hooks[j], hooks[i]
			}
		}
	}

	event.Type = hookType
	for _, h := range hooks {
		if err := h.fn(event); err != nil {
			slog.Warn("Plugin hook error", "type", hookType, "hook", h.name, "error", err)
			return err
		}
	}
	return nil
}

// HasHooks returns true if any hooks are registered for the type.
func (r *PluginHookRegistry) HasHooks(hookType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.hooks[hookType]) > 0
}

// Clear removes all hooks (useful for testing).
func (r *PluginHookRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = make(map[string][]registeredHook)
}

// LoadShellHooks discovers and registers shell hooks from ~/.hermes/hooks/.
func LoadShellHooks() {
	hooksDir := filepath.Join(config.HermesHome(), "hooks")
	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		hookType := ""
		switch {
		case name == "pre_tool_call" || name == "pre_tool_call.sh":
			hookType = HookPreToolCall
		case name == "post_tool_call" || name == "post_tool_call.sh":
			hookType = HookPostToolCall
		case name == "pre_llm_call" || name == "pre_llm_call.sh":
			hookType = HookPreLLMCall
		case name == "post_llm_call" || name == "post_llm_call.sh":
			hookType = HookPostLLMCall
		case name == "on_session_start" || name == "on_session_start.sh":
			hookType = HookSessionStart
		case name == "on_session_end" || name == "on_session_end.sh":
			hookType = HookSessionEnd
		default:
			continue
		}

		scriptPath := filepath.Join(hooksDir, name)
		globalPluginHooks.Register(hookType, "shell:"+name, 50, func(event *HookEvent) error {
			cmd := exec.Command("sh", "-c", scriptPath)
			cmd.Env = append(os.Environ(),
				"HERMES_HOOK_TYPE="+event.Type,
				"HERMES_TOOL_NAME="+event.ToolName,
				"HERMES_SESSION_ID="+event.SessionID,
			)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			done := make(chan error, 1)
			go func() { done <- cmd.Run() }()

			select {
			case err := <-done:
				return err
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				return nil
			}
		})

		slog.Debug("Shell hook loaded", "type", hookType, "script", scriptPath)
	}
}
