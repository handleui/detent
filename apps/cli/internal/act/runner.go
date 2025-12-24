package act

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const gracefulShutdownTimeout = 5 * time.Second

// killProcessGroup sends a signal to an entire process group.
// Using negative PID sends the signal to all processes in the group.
// This ensures child processes (spawned by act/Docker) are also terminated.
func killProcessGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}

// RunConfig configures the act execution.
// ActBinary should only be set by trusted code paths (defaults to "act").
type RunConfig struct {
	WorkflowPath string
	Event        string
	Verbose      bool
	WorkDir      string
	ActBinary    string
	StreamOutput bool           // If true, stream act output to stderr in real-time
	LogChan      chan<- string  // Optional channel to send log lines to (for TUI)
}

// RunResult contains the result of an act execution
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

var validEventPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// filterEnvironment returns a filtered list of environment variables
// that only includes safe variables to prevent secret leakage to act containers
func filterEnvironment(env []string) []string {
	// Whitelist of safe environment variables
	safePrefixes := []string{
		"PATH=", "HOME=", "USER=", "SHELL=", "LANG=", "LC_",
		"TERM=", "TMPDIR=", "TZ=",
	}

	var filtered []string
	for _, e := range env {
		for _, prefix := range safePrefixes {
			if strings.HasPrefix(e, prefix) {
				filtered = append(filtered, e)
				break
			}
		}
	}
	return filtered
}

// Run executes the act tool with the given configuration.
// It handles context cancellation, graceful shutdown (SIGTERM then SIGKILL),
// and returns the captured output along with exit code and duration.
// The ActBinary path defaults to "act" from PATH if not specified.
func Run(ctx context.Context, cfg *RunConfig) (*RunResult, error) {
	args, argsErr := buildArgs(cfg)
	if argsErr != nil {
		return nil, argsErr
	}

	actBinary := cfg.ActBinary
	if actBinary == "" {
		actBinary = "act"
	}

	cmd := exec.CommandContext(ctx, actBinary, args...) //nolint:gosec // ActBinary is trusted; defaults to "act"
	cmd.Dir = cfg.WorkDir

	// Set up process group to ensure graceful shutdown
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer

	// Set up output writers based on configuration
	stdoutWriters := []io.Writer{&stdout}
	stderrWriters := []io.Writer{&stderr}

	if cfg.StreamOutput {
		// Stream output to stderr in real-time
		stdoutWriters = append(stdoutWriters, os.Stderr)
		stderrWriters = append(stderrWriters, os.Stderr)
	}

	if cfg.LogChan != nil {
		// Stream to TUI via channel
		stdoutChan := newChanWriter(cfg.LogChan)
		stderrChan := newChanWriter(cfg.LogChan)
		stdoutWriters = append(stdoutWriters, stdoutChan)
		stderrWriters = append(stderrWriters, stderrChan)
	}

	cmd.Stdout = io.MultiWriter(stdoutWriters...)
	cmd.Stderr = io.MultiWriter(stderrWriters...)

	cmd.Env = filterEnvironment(os.Environ())

	start := time.Now()

	// Start the process instead of using Run() for better control
	var err error
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting act: %w", err)
	}

	// Monitor context and handle graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err = <-done:
		// Process finished normally
	case <-ctx.Done():
		// Context cancelled - attempt graceful shutdown of entire process group
		if cmd.Process != nil {
			// Try SIGTERM to entire process group first for graceful shutdown
			// This ensures child processes (spawned by act/Docker) are also signaled
			if pgid, pgidErr := syscall.Getpgid(cmd.Process.Pid); pgidErr == nil {
				_ = killProcessGroup(pgid, syscall.SIGTERM)
			} else {
				// Fallback to single process if we can't get process group
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}

			// Wait for graceful shutdown
			gracefulTimeout := time.After(gracefulShutdownTimeout)
			select {
			case err = <-done:
				// Gracefully exited
			case <-gracefulTimeout:
				// Force kill entire process group if still running
				if pgid, pgidErr := syscall.Getpgid(cmd.Process.Pid); pgidErr == nil {
					_ = killProcessGroup(pgid, syscall.SIGKILL)
				}
				_ = cmd.Process.Kill() // Also kill main process directly as fallback
				err = <-done
			}
		} else {
			err = <-done
		}
	}

	duration := time.Since(start)

	// Check if context was cancelled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("running act: %w", err)
		}
	}

	return &RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

func buildArgs(cfg *RunConfig) ([]string, error) {
	var args []string

	if cfg.WorkflowPath != "" {
		args = append(args, "-W", cfg.WorkflowPath)
	}

	if cfg.Event != "" {
		if !validEventPattern.MatchString(cfg.Event) {
			return nil, fmt.Errorf("invalid event name %q: must contain only alphanumeric, underscore, or hyphen", cfg.Event)
		}
		args = append(args, cfg.Event)
	}

	// ALWAYS use verbose mode to capture step output for error extraction
	// Use medium-sized images to avoid interactive prompt on first run
	// Docker-resilient flags to prevent container buildup and failures
	// Container security hardening: drop dangerous capabilities
	args = append(args,
		"-v", // Always verbose (regardless of whether user wants to see it)
		"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
		"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
		"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
		"--rm",              // Remove containers after execution
		"--no-cache-server", // Disable cache server (can cause hangs/failures)
		// Security: drop dangerous capabilities rarely needed for CI workflows
		"--container-cap-drop", "SYS_ADMIN",  // Prevents container escapes via mount
		"--container-cap-drop", "NET_ADMIN",  // Prevents network manipulation
		"--container-cap-drop", "SYS_PTRACE", // Prevents process debugging/injection
		"--container-cap-drop", "MKNOD",      // Prevents device node creation
	)

	return args, nil
}

// chanWriter is an io.Writer that sends each line to a channel
type chanWriter struct {
	ch     chan<- string
	buffer bytes.Buffer
}

func newChanWriter(ch chan<- string) *chanWriter {
	return &chanWriter{ch: ch}
}

func (w *chanWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	w.buffer.Write(p)

	// Incremental line splitting - O(n) instead of O(n²)
	data := w.buffer.Bytes()
	for {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}

		line := string(bytes.TrimSpace(data[:idx]))
		data = data[idx+1:]

		select {
		case w.ch <- line:
		default:
			// Channel full or closed, skip
		}
	}

	// Keep remaining data in buffer
	w.buffer.Reset()
	w.buffer.Write(data)

	return n, nil
}
