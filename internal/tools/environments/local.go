package environments

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// LocalEnvironment executes commands on the local machine.
type LocalEnvironment struct {
	workDir string
}

func init() {
	RegisterEnvironment("local", func(params map[string]string) (Environment, error) {
		env := NewLocalEnvironment()
		if dir, ok := params["working_directory"]; ok && dir != "" {
			env.workDir = dir
		}
		return env, nil
	})
}

// NewLocalEnvironment creates a new local execution environment.
func NewLocalEnvironment() *LocalEnvironment {
	cwd, _ := os.Getwd()
	return &LocalEnvironment{
		workDir: cwd,
	}
}

// Execute runs a command locally via /bin/sh.
func (e *LocalEnvironment) Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error) {
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = e.workDir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

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
			return stdout, stderr, -1, fmt.Errorf("command execution failed: %w", runErr)
		}
	}

	return stdout, stderr, exitCode, nil
}

// IsAvailable always returns true for local environment.
func (e *LocalEnvironment) IsAvailable() bool {
	return true
}

// Name returns "local".
func (e *LocalEnvironment) Name() string {
	return "local"
}

// SetWorkDir sets the working directory for command execution.
func (e *LocalEnvironment) SetWorkDir(dir string) {
	e.workDir = dir
}

// WorkDir returns the current working directory.
func (e *LocalEnvironment) WorkDir() string {
	return e.workDir
}
