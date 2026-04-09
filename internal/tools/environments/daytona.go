package environments

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DaytonaEnvironment executes commands inside a Daytona workspace.
type DaytonaEnvironment struct {
	workspaceID string
	project     string
}

func init() {
	RegisterEnvironment("daytona", func(params map[string]string) (Environment, error) {
		workspaceID := params["workspace_id"]
		project := params["project"]
		return &DaytonaEnvironment{
			workspaceID: workspaceID,
			project:     project,
		}, nil
	})
}

// ensureWorkspace creates a Daytona workspace if one is not already set.
func (e *DaytonaEnvironment) ensureWorkspace() error {
	if e.workspaceID != "" {
		return nil
	}

	// Create a new workspace via Daytona CLI.
	args := []string{"workspace", "create"}
	if e.project != "" {
		args = append(args, "--project", e.project)
	}
	args = append(args, "--no-prompt")

	cmd := exec.Command("daytona", args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create daytona workspace: %w: %s", err, stderrBuf.String())
	}

	// Parse workspace ID from output (first non-empty line).
	output := strings.TrimSpace(stdoutBuf.String())
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			e.workspaceID = line
			break
		}
	}

	if e.workspaceID == "" {
		return fmt.Errorf("daytona workspace create returned empty workspace ID")
	}
	return nil
}

// Execute runs a command inside the Daytona workspace via `daytona exec`.
func (e *DaytonaEnvironment) Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error) {
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}

	if err := e.ensureWorkspace(); err != nil {
		return "", "", -1, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	args := []string{"exec", e.workspaceID}
	if e.project != "" {
		args = append(args, "--project", e.project)
	}
	args = append(args, "--", "sh", "-c", command)

	cmd := exec.CommandContext(ctx, "daytona", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	exitCode = 0

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return stdout, stderr, -1, fmt.Errorf("command timed out after %d seconds", timeout)
		} else {
			return stdout, stderr, -1, fmt.Errorf("daytona exec failed: %w", runErr)
		}
	}

	return stdout, stderr, exitCode, nil
}

// IsAvailable checks if the `daytona` CLI is installed.
func (e *DaytonaEnvironment) IsAvailable() bool {
	_, err := exec.LookPath("daytona")
	return err == nil
}

// Name returns "daytona".
func (e *DaytonaEnvironment) Name() string {
	return "daytona"
}
