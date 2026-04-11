package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hermes-agent/hermes-agent-go/internal/config"
)

func init() {
	Register(&ToolEntry{
		Name:    "execute_code",
		Toolset: "code_execution",
		Schema: map[string]any{
			"name":        "execute_code",
			"description": "Execute code in a sandboxed subprocess. Supports Python and Bash. Environment variables are stripped for security.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]any{
						"type":        "string",
						"description": "Programming language to execute",
						"enum":        []string{"python", "bash"},
					},
					"code": map[string]any{
						"type":        "string",
						"description": "Code to execute",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Execution timeout in seconds (default: 30, max: 120)",
						"default":     30,
					},
				},
				"required": []string{"language", "code"},
			},
		},
		Handler: handleExecuteCode,
		Emoji:   "\u25b6\ufe0f",
	})
}

// safeEnv returns a minimal set of environment variables for sandboxed execution.
func safeEnv() []string {
	safe := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"LANG=en_US.UTF-8",
		"TERM=xterm-256color",
	}
	// Include TMPDIR if set
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		safe = append(safe, "TMPDIR="+tmp)
	}
	return safe
}

func handleExecuteCode(args map[string]any, ctx *ToolContext) string {
	language, _ := args["language"].(string)
	code, _ := args["code"].(string)

	if language == "" {
		return `{"error":"language is required"}`
	}
	if code == "" {
		return `{"error":"code is required"}`
	}

	cfg := DefaultSandboxConfig()

	timeout := 30
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}
	if timeout > 120 {
		timeout = 120
	}
	cfg.Timeout = time.Duration(timeout) * time.Second

	switch language {
	case "python":
		return executePython(code, &cfg)
	case "bash":
		return executeBash(code, &cfg)
	default:
		return toJSON(map[string]any{"error": fmt.Sprintf("Unsupported language: %s", language)})
	}
}

func executePython(code string, cfg *SandboxConfig) string {
	// Write code to a temporary file
	tmpDir := filepath.Join(config.HermesHome(), "cache")
	os.MkdirAll(tmpDir, 0755)

	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("exec_%d.py", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte(code), 0644); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to write temp file: %v", err)})
	}
	defer os.Remove(tmpFile)

	return runSandboxed("python3", []string{tmpFile}, tmpDir, "python", cfg)
}

func executeBash(code string, cfg *SandboxConfig) string {
	cwd, _ := os.Getwd()
	return runSandboxed("bash", []string{"-c", code}, cwd, "bash", cfg)
}

// runSandboxed executes a command inside the sandbox constraints defined by cfg.
func runSandboxed(bin string, cmdArgs []string, workDir, language string, cfg *SandboxConfig) string {
	execCtx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, bin, cmdArgs...)
	cmd.Env = safeEnv()
	cmd.Dir = workDir

	stdoutW := NewLimitedWriter(cfg.MaxStdoutBytes)
	stderrW := NewLimitedWriter(cfg.MaxStderrBytes)
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	metrics := ExecMetrics{}
	startTime := time.Now()
	err := cmd.Run()
	metrics.WallTimeMs = time.Since(startTime).Milliseconds()
	metrics.StdoutBytes = stdoutW.Len()
	metrics.StderrBytes = stderrW.Len()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			metrics.ExitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			metrics.ExitCode = -1
			slog.Info("sandbox execution timed out", "language", language, "timeout", cfg.Timeout)
			return toJSON(map[string]any{
				"error":     "Execution timed out",
				"timeout":   int(cfg.Timeout.Seconds()),
				"stdout":    stdoutW.String(),
				"stderr":    stderrW.String(),
				"exit_code": metrics.ExitCode,
				"metrics":   metrics,
			})
		}
	}

	return toJSON(map[string]any{
		"stdout":      stdoutW.String(),
		"stderr":      stderrW.String(),
		"exit_code":   metrics.ExitCode,
		"language":    language,
		"duration_ms": metrics.WallTimeMs,
		"metrics":     metrics,
	})
}
