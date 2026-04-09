package environments

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// ModalEnvironment executes commands via Modal's serverless Python SDK.
// Commands are run inside a Modal sandbox using `modal sandbox create` and
// `modal sandbox exec`.
type ModalEnvironment struct {
	appName string
	gpu     string
	timeout int
}

func init() {
	RegisterEnvironment("modal", func(params map[string]string) (Environment, error) {
		appName := params["app_name"]
		if appName == "" {
			appName = "hermes-sandbox"
		}
		gpu := params["gpu"]
		timeoutParam := 0
		if t, ok := params["timeout"]; ok && t != "" {
			// Parse timeout; ignored if invalid, will use default in Execute.
			fmt.Sscanf(t, "%d", &timeoutParam)
		}
		return &ModalEnvironment{
			appName: appName,
			gpu:     gpu,
			timeout: timeoutParam,
		}, nil
	})
}

// Execute runs a command inside a Modal sandbox.
// It creates a sandbox, executes the command, and collects output.
func (e *ModalEnvironment) Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error) {
	if timeout <= 0 {
		if e.timeout > 0 {
			timeout = e.timeout
		} else {
			timeout = 120
		}
	}
	if timeout > 600 {
		timeout = 600
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Step 1: Create a sandbox.
	createArgs := []string{"sandbox", "create", "--app", e.appName}
	if e.gpu != "" {
		createArgs = append(createArgs, "--gpu", e.gpu)
	}
	createArgs = append(createArgs, "--timeout", fmt.Sprintf("%d", timeout))

	createCmd := exec.CommandContext(ctx, "modal", createArgs...)
	var createOut, createErr bytes.Buffer
	createCmd.Stdout = &createOut
	createCmd.Stderr = &createErr

	if runErr := createCmd.Run(); runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", createErr.String(), -1, fmt.Errorf("modal sandbox creation timed out after %d seconds", timeout)
		}
		return "", createErr.String(), -1, fmt.Errorf("failed to create modal sandbox: %w: %s", runErr, createErr.String())
	}

	// The sandbox ID is printed to stdout by `modal sandbox create`.
	sandboxID := bytes.TrimSpace(createOut.Bytes())
	if len(sandboxID) == 0 {
		return "", createErr.String(), -1, fmt.Errorf("modal sandbox create returned empty sandbox ID")
	}

	// Step 2: Execute the command inside the sandbox.
	execArgs := []string{"sandbox", "exec", string(sandboxID), "sh", "-c", command}
	execCmd := exec.CommandContext(ctx, "modal", execArgs...)

	var stdoutBuf, stderrBuf bytes.Buffer
	execCmd.Stdout = &stdoutBuf
	execCmd.Stderr = &stderrBuf

	runErr := execCmd.Run()

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	exitCode = 0

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return stdout, stderr, -1, fmt.Errorf("command timed out after %d seconds", timeout)
		} else {
			return stdout, stderr, -1, fmt.Errorf("modal sandbox exec failed: %w", runErr)
		}
	}

	return stdout, stderr, exitCode, nil
}

// IsAvailable checks if the `modal` CLI is installed.
func (e *ModalEnvironment) IsAvailable() bool {
	_, err := exec.LookPath("modal")
	return err == nil
}

// Name returns "modal".
func (e *ModalEnvironment) Name() string {
	return "modal"
}
