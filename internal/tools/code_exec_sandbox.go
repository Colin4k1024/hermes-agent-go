package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

// --- LimitedWriter ---

// LimitedWriter wraps a bytes.Buffer and enforces a maximum byte limit.
// When the limit is exceeded, further writes are silently dropped and a
// truncation notice is appended when the final output is read.
type LimitedWriter struct {
	buf      bytes.Buffer
	maxBytes int
	exceeded bool
	mu       sync.Mutex
}

// NewLimitedWriter creates a LimitedWriter with the given byte cap.
func NewLimitedWriter(maxBytes int) *LimitedWriter {
	return &LimitedWriter{maxBytes: maxBytes}
}

// Write implements io.Writer. Bytes beyond the limit are dropped.
func (lw *LimitedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.exceeded {
		return len(p), nil // accept but discard
	}

	remaining := lw.maxBytes - lw.buf.Len()
	if remaining <= 0 {
		lw.exceeded = true
		return len(p), nil
	}

	if len(p) > remaining {
		lw.buf.Write(p[:remaining])
		lw.exceeded = true
		return len(p), nil
	}

	lw.buf.Write(p)
	return len(p), nil
}

// String returns the captured output with a truncation notice if the limit
// was exceeded.
func (lw *LimitedWriter) String() string {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.exceeded {
		return lw.buf.String() + fmt.Sprintf("\n... [output truncated at %dKB]", lw.maxBytes/1024)
	}
	return lw.buf.String()
}

// Len returns the number of bytes currently stored.
func (lw *LimitedWriter) Len() int {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.buf.Len()
}

// Exceeded reports whether the limit was hit.
func (lw *LimitedWriter) Exceeded() bool {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.exceeded
}

// Ensure LimitedWriter implements io.Writer.
var _ io.Writer = (*LimitedWriter)(nil)

// --- SandboxConfig ---

const (
	DefaultMaxStdoutBytes = 50 * 1024 // 50 KB
	DefaultMaxStderrBytes = 10 * 1024 // 10 KB
	DefaultMaxToolCalls   = 50
	DefaultTimeout        = 30 * time.Second
)

// DefaultAllowedTools is the default set of tools that sandboxed code may invoke.
var DefaultAllowedTools = []string{
	"read_file",
	"write_file",
	"search_files",
	"patch",
	"terminal",
	"web_search",
	"web_extract",
}

// SandboxConfig holds resource limits and policy for code execution.
type SandboxConfig struct {
	MaxStdoutBytes  int           `json:"max_stdout_bytes"`
	MaxStderrBytes  int           `json:"max_stderr_bytes"`
	MaxToolCalls    int           `json:"max_tool_calls"`
	Timeout         time.Duration `json:"timeout"`
	AllowedTools    []string      `json:"allowed_tools"`
	RestrictNetwork bool          `json:"restrict_network"`
}

// DefaultSandboxConfig returns a SandboxConfig with safe defaults.
func DefaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		MaxStdoutBytes:  DefaultMaxStdoutBytes,
		MaxStderrBytes:  DefaultMaxStderrBytes,
		MaxToolCalls:    DefaultMaxToolCalls,
		Timeout:         DefaultTimeout,
		AllowedTools:    append([]string{}, DefaultAllowedTools...),
		RestrictNetwork: false,
	}
}

// IsToolAllowed checks whether a tool name is in the allowlist.
func (sc *SandboxConfig) IsToolAllowed(toolName string) bool {
	for _, t := range sc.AllowedTools {
		if t == toolName {
			return true
		}
	}
	return false
}

// --- Execution Metrics ---

// ExecMetrics captures resource-usage statistics from a single code execution.
type ExecMetrics struct {
	WallTimeMs    int64 `json:"wall_time_ms"`
	ExitCode      int   `json:"exit_code"`
	StdoutBytes   int   `json:"stdout_bytes"`
	StderrBytes   int   `json:"stderr_bytes"`
	ToolCallCount int   `json:"tool_call_count"`
}

// --- File-based RPC for tool call forwarding ---

// ToolCallRequest is serialised to request.json inside the RPC directory.
type ToolCallRequest struct {
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
}

// ToolCallResponse is serialised to response.json inside the RPC directory.
type ToolCallResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// rpcBaseDir returns the root directory for sandbox RPC exchanges.
func rpcBaseDir() string {
	return filepath.Join(config.HermesHome(), "cache", "sandbox_rpc")
}

// ProcessToolCallRequest reads a request.json, validates against the allowlist,
// dispatches through the registry, and writes response.json.
// It returns true if a request was processed, false otherwise.
func ProcessToolCallRequest(rpcDir string, cfg *SandboxConfig, metrics *ExecMetrics) bool {
	reqPath := filepath.Join(rpcDir, "request.json")
	respPath := filepath.Join(rpcDir, "response.json")

	data, err := os.ReadFile(reqPath)
	if err != nil {
		return false
	}

	// Remove request file to signal we've consumed it
	os.Remove(reqPath)

	var req ToolCallRequest
	if err := json.Unmarshal(data, &req); err != nil {
		slog.Warn("sandbox rpc: malformed request", "error", err, "dir", rpcDir)
		writeRPCResponse(respPath, ToolCallResponse{
			Error: fmt.Sprintf("malformed request: %v", err),
		})
		return true
	}

	// Check allowlist
	if cfg != nil && !cfg.IsToolAllowed(req.ToolName) {
		slog.Info("sandbox rpc: tool not allowed", "tool", req.ToolName)
		writeRPCResponse(respPath, ToolCallResponse{
			Error: fmt.Sprintf("tool %q not in sandbox allowlist", req.ToolName),
		})
		if metrics != nil {
			metrics.ToolCallCount++
		}
		return true
	}

	// Check max tool calls
	if metrics != nil && cfg != nil && metrics.ToolCallCount >= cfg.MaxToolCalls {
		slog.Info("sandbox rpc: max tool calls reached", "limit", cfg.MaxToolCalls)
		writeRPCResponse(respPath, ToolCallResponse{
			Error: fmt.Sprintf("max tool calls (%d) exceeded", cfg.MaxToolCalls),
		})
		return true
	}

	// Dispatch through global registry
	result := Registry().Dispatch(req.ToolName, req.Args, nil)

	if metrics != nil {
		metrics.ToolCallCount++
	}

	writeRPCResponse(respPath, ToolCallResponse{Result: result})
	return true
}

// writeRPCResponse writes a ToolCallResponse to the given path.
func writeRPCResponse(path string, resp ToolCallResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("sandbox rpc: marshal response", "error", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Error("sandbox rpc: write response", "error", err, "path", path)
	}
}

// SetupRPCDir creates a unique RPC directory for a sandbox session and returns its path.
func SetupRPCDir(sessionID string) (string, error) {
	dir := filepath.Join(rpcBaseDir(), sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("setup rpc dir: %w", err)
	}
	return dir, nil
}

// CleanupRPCDir removes the RPC directory after execution completes.
func CleanupRPCDir(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("sandbox rpc: cleanup failed", "dir", dir, "error", err)
	}
}
