package environments

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PersistentShellEnvironment maintains a long-running shell process
// that preserves state (environment variables, working directory, etc.)
// across multiple command executions.
type PersistentShellEnvironment struct {
	shellCmd *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	stderr   *bufio.Reader
	marker   string // unique marker to detect command completion

	mu     sync.Mutex
	closed bool
}

func init() {
	RegisterEnvironment("persistent_shell", func(params map[string]string) (Environment, error) {
		env, err := NewPersistentShell()
		if err != nil {
			return nil, err
		}
		return env, nil
	})
}

// NewPersistentShell creates and starts a new persistent shell session.
func NewPersistentShell() (*PersistentShellEnvironment, error) {
	// Use bash if available, fall back to sh.
	shellPath := "/bin/bash"
	if _, err := os.Stat(shellPath); err != nil {
		shellPath = "/bin/sh"
	}

	cmd := exec.Command(shellPath, "--norc", "--noprofile", "-i")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "PS1=")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	env := &PersistentShellEnvironment{
		shellCmd: cmd,
		stdin:    stdin,
		stdout:   bufio.NewReaderSize(stdoutPipe, 64*1024),
		stderr:   bufio.NewReaderSize(stderrPipe, 64*1024),
		marker:   fmt.Sprintf("__HERMES_DONE_%d__", time.Now().UnixNano()),
	}

	return env, nil
}

// Execute sends a command to the persistent shell and reads output until
// the completion marker appears. The exit code is captured via $?.
func (e *PersistentShellEnvironment) Execute(command string, timeout int) (stdout, stderr string, exitCode int, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return "", "", -1, fmt.Errorf("shell session is closed")
	}

	if timeout <= 0 {
		timeout = 120
	}
	if timeout > 600 {
		timeout = 600
	}

	// Build the command with exit code capture and marker.
	// We echo the marker with the exit code appended so we can parse it.
	wrappedCmd := fmt.Sprintf("%s\n__hermes_ec=$?\necho \"%s:$__hermes_ec\"\necho \"%s\" >&2\n",
		command, e.marker, e.marker)

	_, err = io.WriteString(e.stdin, wrappedCmd)
	if err != nil {
		return "", "", -1, fmt.Errorf("write command: %w", err)
	}

	// Read stdout until we see the marker.
	stdoutDone := make(chan string, 1)
	stderrDone := make(chan string, 1)
	errCh := make(chan error, 2)

	go func() {
		var sb strings.Builder
		for {
			line, readErr := e.stdout.ReadString('\n')
			if readErr != nil {
				errCh <- fmt.Errorf("read stdout: %w", readErr)
				return
			}
			trimmed := strings.TrimRight(line, "\n\r")
			if strings.HasPrefix(trimmed, e.marker) {
				// Parse exit code from marker line: __HERMES_DONE_xxx__:0
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					fmt.Sscanf(parts[1], "%d", &exitCode)
				}
				stdoutDone <- sb.String()
				return
			}
			sb.WriteString(line)
		}
	}()

	go func() {
		var sb strings.Builder
		for {
			line, readErr := e.stderr.ReadString('\n')
			if readErr != nil {
				errCh <- fmt.Errorf("read stderr: %w", readErr)
				return
			}
			trimmed := strings.TrimRight(line, "\n\r")
			if trimmed == e.marker {
				stderrDone <- sb.String()
				return
			}
			sb.WriteString(line)
		}
	}()

	timer := time.NewTimer(time.Duration(timeout) * time.Second)
	defer timer.Stop()

	var stdoutResult, stderrResult string
	doneCount := 0

	for doneCount < 2 {
		select {
		case s := <-stdoutDone:
			stdoutResult = s
			doneCount++
		case s := <-stderrDone:
			stderrResult = s
			doneCount++
		case readErr := <-errCh:
			return stdoutResult, stderrResult, -1, readErr
		case <-timer.C:
			return stdoutResult, stderrResult, -1, fmt.Errorf("command timed out after %d seconds", timeout)
		}
	}

	return stdoutResult, stderrResult, exitCode, nil
}

// Close terminates the persistent shell session.
func (e *PersistentShellEnvironment) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	// Send exit command.
	io.WriteString(e.stdin, "exit\n")
	e.stdin.Close()

	// Wait for the process to exit with a timeout.
	done := make(chan error, 1)
	go func() {
		done <- e.shellCmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		e.shellCmd.Process.Kill()
	}

	return nil
}

// IsAvailable returns true if the shell process is still running.
func (e *PersistentShellEnvironment) IsAvailable() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.closed
}

// Name returns "persistent_shell".
func (e *PersistentShellEnvironment) Name() string {
	return "persistent_shell"
}
