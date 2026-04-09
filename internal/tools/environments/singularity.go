package environments

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SingularityEnvironment executes commands via Singularity (or Apptainer) containers.
type SingularityEnvironment struct {
	image      string
	bindPaths  []string
	scratchDir string
	executable string // "singularity" or "apptainer"
}

func init() {
	RegisterEnvironment("singularity", func(params map[string]string) (Environment, error) {
		image := params["image"]
		if image == "" {
			return nil, fmt.Errorf("singularity environment requires 'image' parameter")
		}

		var bindPaths []string
		if bp, ok := params["bind_paths"]; ok && bp != "" {
			bindPaths = strings.Split(bp, ",")
		}

		// Determine which executable is available: apptainer or singularity.
		executable := "singularity"
		if _, err := exec.LookPath("apptainer"); err == nil {
			executable = "apptainer"
		}

		// Create a scratch directory for temporary files.
		scratchDir, err := os.MkdirTemp("", "hermes-singularity-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create scratch directory: %w", err)
		}

		return &SingularityEnvironment{
			image:      image,
			bindPaths:  bindPaths,
			scratchDir: scratchDir,
			executable: executable,
		}, nil
	})
}

// Execute runs a command inside a Singularity/Apptainer container.
func (e *SingularityEnvironment) Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error) {
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	args := []string{"exec"}

	// Bind the scratch directory.
	scratchBind := e.scratchDir + ":/scratch"
	args = append(args, "--bind", scratchBind)

	// Bind CWD as /workspace.
	cwd, _ := os.Getwd()
	args = append(args, "--bind", cwd+":/workspace")
	args = append(args, "--pwd", "/workspace")

	// Add user-specified bind paths.
	for _, bp := range e.bindPaths {
		bp = strings.TrimSpace(bp)
		if bp != "" {
			args = append(args, "--bind", bp)
		}
	}

	args = append(args, e.image, "sh", "-c", command)

	cmd := exec.CommandContext(ctx, e.executable, args...)

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
			return stdout, stderr, -1, fmt.Errorf("%s exec failed: %w", e.executable, runErr)
		}
	}

	return stdout, stderr, exitCode, nil
}

// IsAvailable checks if singularity or apptainer is installed.
func (e *SingularityEnvironment) IsAvailable() bool {
	_, err := exec.LookPath(e.executable)
	return err == nil
}

// Name returns "singularity".
func (e *SingularityEnvironment) Name() string {
	return "singularity"
}

// ScratchDir returns the path to the scratch directory.
func (e *SingularityEnvironment) ScratchDir() string {
	return e.scratchDir
}

// Cleanup removes the scratch directory. Should be called when the environment
// is no longer needed.
func (e *SingularityEnvironment) Cleanup() error {
	if e.scratchDir == "" {
		return nil
	}
	// Safety check: only remove paths under the system temp dir.
	tempDir := os.TempDir()
	absPath, err := filepath.Abs(e.scratchDir)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absPath, tempDir) {
		return fmt.Errorf("refusing to remove scratch dir outside temp: %s", absPath)
	}
	return os.RemoveAll(absPath)
}
