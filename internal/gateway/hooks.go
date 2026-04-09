package gateway

import (
	"fmt"
	"log/slog"
	"sync"
)

// Hook type constants for the hook system.
const (
	HookBeforeMessage  = "before_message"
	HookAfterMessage   = "after_message"
	HookBeforeToolCall = "before_tool_call"
	HookAfterToolCall  = "after_tool_call"
	HookOnError        = "on_error"
)

// HookEvent carries context for a hook invocation.
type HookEvent struct {
	// Type is the hook type that triggered this event.
	Type string

	// SessionKey identifies the session.
	SessionKey string

	// Source describes the message origin.
	Source *SessionSource

	// Message is the incoming or outgoing text.
	Message string

	// Response is the agent response (only for after_message hooks).
	Response string

	// ToolName is the tool being called (only for tool call hooks).
	ToolName string

	// ToolArgs holds tool arguments (only for tool call hooks).
	ToolArgs map[string]any

	// ToolResult holds the tool output (only for after_tool_call hooks).
	ToolResult string

	// Error holds the error (only for on_error hooks).
	Error error

	// Metadata for arbitrary key-value data.
	Metadata map[string]string
}

// HookFunc is the function signature for hook callbacks.
type HookFunc func(event *HookEvent) error

// HookRegistration represents a registered hook with metadata.
type HookRegistration struct {
	Name     string
	Type     string
	Fn       HookFunc
	Priority int // lower = earlier execution
}

// HookRegistry manages before/after hooks for message processing.
type HookRegistry struct {
	mu    sync.RWMutex
	hooks map[string][]HookRegistration
}

// NewHookRegistry creates a new hook registry.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks: make(map[string][]HookRegistration),
	}
}

// RegisterHook registers a hook function for a specific hook type.
func (h *HookRegistry) RegisterHook(hookType string, fn HookFunc) {
	h.RegisterNamedHook(hookType, "", fn, 0)
}

// RegisterNamedHook registers a named hook with priority.
func (h *HookRegistry) RegisterNamedHook(hookType, name string, fn HookFunc, priority int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if name == "" {
		name = fmt.Sprintf("%s_hook_%d", hookType, len(h.hooks[hookType]))
	}

	reg := HookRegistration{
		Name:     name,
		Type:     hookType,
		Fn:       fn,
		Priority: priority,
	}

	hooks := h.hooks[hookType]
	// Insert sorted by priority.
	inserted := false
	for i, existing := range hooks {
		if priority < existing.Priority {
			hooks = append(hooks[:i+1], hooks[i:]...)
			hooks[i] = reg
			inserted = true
			break
		}
	}
	if !inserted {
		hooks = append(hooks, reg)
	}
	h.hooks[hookType] = hooks
}

// FireHook executes all registered hooks for a type in priority order.
// Returns the first error encountered; remaining hooks are still executed.
func (h *HookRegistry) FireHook(hookType string, event *HookEvent) error {
	h.mu.RLock()
	hooks := make([]HookRegistration, len(h.hooks[hookType]))
	copy(hooks, h.hooks[hookType])
	h.mu.RUnlock()

	if len(hooks) == 0 {
		return nil
	}

	event.Type = hookType

	var firstErr error
	for _, hook := range hooks {
		if err := hook.Fn(event); err != nil {
			slog.Warn("Hook error",
				"hook_type", hookType,
				"hook_name", hook.Name,
				"error", err,
			)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// HasHooks returns true if any hooks are registered for the given type.
func (h *HookRegistry) HasHooks(hookType string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.hooks[hookType]) > 0
}

// HookCount returns the number of hooks registered for a type.
func (h *HookRegistry) HookCount(hookType string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.hooks[hookType])
}

// AllHookTypes returns the list of hook types that have registered hooks.
func (h *HookRegistry) AllHookTypes() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var types []string
	for t, hooks := range h.hooks {
		if len(hooks) > 0 {
			types = append(types, t)
		}
	}
	return types
}

// LoadHooksFromConfig loads hooks from a config map.
// Expected format from config.yaml:
//
//	hooks:
//	  before_message:
//	    - name: "log_incoming"
//	  after_message:
//	    - name: "log_outgoing"
//
// Note: config-defined hooks are metadata-only; actual hook functions must be
// registered programmatically. This method is provided for future extensibility
// when hook plugins are supported.
func (h *HookRegistry) LoadHooksFromConfig(hooksCfg map[string]any) {
	if hooksCfg == nil {
		return
	}

	for hookType, entries := range hooksCfg {
		entryList, ok := entries.([]any)
		if !ok {
			continue
		}
		for _, entry := range entryList {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			name, _ := entryMap["name"].(string)
			if name != "" {
				slog.Debug("Config hook registered (placeholder)", "type", hookType, "name", name)
			}
		}
	}
}
