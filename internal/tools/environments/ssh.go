package environments

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// SSHEnvironment executes commands on a remote host via the ssh subprocess.
type SSHEnvironment struct {
	host    string
	user    string
	port    string
	keyFile string
}

func init() {
	RegisterEnvironment("ssh", func(params map[string]string) (Environment, error) {
		host := params["host"]
		if host == "" {
			return nil, fmt.Errorf("ssh environment requires 'host' parameter")
		}
		user := params["user"]
		if user == "" {
			user = "root"
		}
		port := params["port"]
		if port == "" {
			port = "22"
		}
		keyFile := params["key_file"]

		return &SSHEnvironment{
			host:    host,
			user:    user,
			port:    port,
			keyFile: keyFile,
		}, nil
	})
}

// sshArgs builds the base ssh argument list (options before the command).
func (e *SSHEnvironment) sshArgs() []string {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",
		"-p", e.port,
	}
	if e.keyFile != "" {
		args = append(args, "-i", e.keyFile)
	}

	// If SSH_PASSWORD is set, switch from BatchMode to sshpass-based auth.
	if password := os.Getenv("SSH_PASSWORD"); password != "" {
		// Remove BatchMode when using password auth.
		args = []string{
			"-o", "StrictHostKeyChecking=no",
			"-p", e.port,
		}
	}

	args = append(args, fmt.Sprintf("%s@%s", e.user, e.host))
	return args
}

// Execute runs a command on the remote host via ssh.
func (e *SSHEnvironment) Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error) {
	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	sshArgs := e.sshArgs()
	sshArgs = append(sshArgs, "--", command)

	var cmd *exec.Cmd

	// Use sshpass for password authentication when SSH_PASSWORD is set.
	if password := os.Getenv("SSH_PASSWORD"); password != "" {
		sshpassArgs := append([]string{"-e", "ssh"}, sshArgs...)
		cmd = exec.CommandContext(ctx, "sshpass", sshpassArgs...)
		cmd.Env = append(os.Environ(), "SSHPASS="+password)
	} else {
		cmd = exec.CommandContext(ctx, "ssh", sshArgs...)
	}

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
			return stdout, stderr, -1, fmt.Errorf("ssh execution failed: %w", runErr)
		}
	}

	return stdout, stderr, exitCode, nil
}

// IsAvailable checks if the ssh binary exists on the system.
func (e *SSHEnvironment) IsAvailable() bool {
	_, err := exec.LookPath("ssh")
	return err == nil
}

// Name returns "ssh".
func (e *SSHEnvironment) Name() string {
	return "ssh"
}
