package tools

import (
	"bytes"
	"context"
	"fmt"
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

	timeout := 30
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
	}
	if timeout > 120 {
		timeout = 120
	}

	switch language {
	case "python":
		return executePython(code, timeout)
	case "bash":
		return executeBash(code, timeout)
	default:
		return toJSON(map[string]any{"error": fmt.Sprintf("Unsupported language: %s", language)})
	}
}

func executePython(code string, timeout int) string {
	// Write code to a temporary file
	tmpDir := filepath.Join(config.HermesHome(), "cache")
	os.MkdirAll(tmpDir, 0755)

	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("exec_%d.py", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, []byte(code), 0644); err != nil {
		return toJSON(map[string]any{"error": fmt.Sprintf("Failed to write temp file: %v", err)})
	}
	defer os.Remove(tmpFile)

	execCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", tmpFile)
	cmd.Env = safeEnv()
	cmd.Dir = tmpDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			return toJSON(map[string]any{
				"error":     "Execution timed out",
				"timeout":   timeout,
				"stdout":    truncateOutput(stdout.String(), 50000),
				"stderr":    truncateOutput(stderr.String(), 10000),
				"exit_code": -1,
			})
		}
	}

	return toJSON(map[string]any{
		"stdout":      truncateOutput(stdout.String(), 50000),
		"stderr":      truncateOutput(stderr.String(), 10000),
		"exit_code":   exitCode,
		"language":    "python",
		"duration_ms": duration.Milliseconds(),
	})
}

func executeBash(code string, timeout int) string {
	execCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "bash", "-c", code)
	cmd.Env = safeEnv()

	cwd, _ := os.Getwd()
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if execCtx.Err() == context.DeadlineExceeded {
			return toJSON(map[string]any{
				"error":     "Execution timed out",
				"timeout":   timeout,
				"stdout":    truncateOutput(stdout.String(), 50000),
				"stderr":    truncateOutput(stderr.String(), 10000),
				"exit_code": -1,
			})
		}
	}

	return toJSON(map[string]any{
		"stdout":      truncateOutput(stdout.String(), 50000),
		"stderr":      truncateOutput(stderr.String(), 10000),
		"exit_code":   exitCode,
		"language":    "bash",
		"duration_ms": duration.Milliseconds(),
	})
}
