package environments

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DockerEnvironment executes commands inside a Docker container.
type DockerEnvironment struct {
	image         string
	containerName string
	volumes       []string
	forwardEnv    []string
}

func init() {
	RegisterEnvironment("docker", func(params map[string]string) (Environment, error) {
		image := params["image"]
		if image == "" {
			image = "ubuntu:latest"
		}
		containerName := params["container_name"]
		if containerName == "" {
			containerName = "hermes-docker-env"
		}

		var volumes []string
		if v, ok := params["volumes"]; ok && v != "" {
			volumes = strings.Split(v, ",")
		}
		// Mount CWD as /workspace by default.
		cwd, _ := os.Getwd()
		volumes = append(volumes, cwd+":/workspace")

		var forwardEnv []string
		if fe, ok := params["forward_env"]; ok && fe != "" {
			forwardEnv = strings.Split(fe, ",")
		}

		env := &DockerEnvironment{
			image:         image,
			containerName: containerName,
			volumes:       volumes,
			forwardEnv:    forwardEnv,
		}
		return env, nil
	})
}

// ensureContainer checks if the container is running; if not, creates and starts it.
func (e *DockerEnvironment) ensureContainer() error {
	// Check if container is already running.
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", e.containerName).Output()
	if err == nil && strings.TrimSpace(string(out)) == "true" {
		return nil
	}

	// Remove stale container if it exists but is not running.
	_ = exec.Command("docker", "rm", "-f", e.containerName).Run()

	// Build docker run arguments.
	args := []string{"run", "-d", "--name", e.containerName}
	for _, v := range e.volumes {
		args = append(args, "-v", v)
	}
	args = append(args, "-w", "/workspace")
	args = append(args, e.image, "sleep", "infinity")

	cmd := exec.Command("docker", args...)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container %s: %w: %s", e.containerName, err, stderrBuf.String())
	}
	return nil
}

// Execute runs a command inside the Docker container via `docker exec`.
func (e *DockerEnvironment) Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error) {
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}

	if err := e.ensureContainer(); err != nil {
		return "", "", -1, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	args := []string{"exec"}
	// Forward environment variables from the host.
	for _, envVar := range e.forwardEnv {
		val := os.Getenv(envVar)
		if val != "" {
			args = append(args, "-e", envVar+"="+val)
		}
	}
	args = append(args, e.containerName, "sh", "-c", command)

	cmd := exec.CommandContext(ctx, "docker", args...)

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
			return stdout, stderr, -1, fmt.Errorf("docker exec failed: %w", runErr)
		}
	}

	return stdout, stderr, exitCode, nil
}

// IsAvailable checks if Docker is installed and the daemon is reachable.
func (e *DockerEnvironment) IsAvailable() bool {
	err := exec.Command("docker", "info").Run()
	return err == nil
}

// Name returns "docker".
func (e *DockerEnvironment) Name() string {
	return "docker"
}
