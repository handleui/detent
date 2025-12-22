package act

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// RunConfig configures the act execution.
// ActBinary should only be set by trusted code paths (defaults to "act").
type RunConfig struct {
	WorkflowPath string
	Event        string
	Verbose      bool
	WorkDir      string
	ActBinary    string
	StreamOutput bool // If true, stream act output to stderr in real-time
}

// RunResult contains the result of an act execution
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Run executes act with the given configuration.
// The act binary path is controlled internally and defaults to "act" from PATH.
func Run(ctx context.Context, cfg *RunConfig) (*RunResult, error) {
	args := buildArgs(cfg)

	actBinary := cfg.ActBinary
	if actBinary == "" {
		actBinary = "act"
	}

	cmd := exec.CommandContext(ctx, actBinary, args...) //nolint:gosec // ActBinary is trusted; defaults to "act"
	cmd.Dir = cfg.WorkDir

	// Set up process group to ensure graceful shutdown
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer

	if cfg.StreamOutput {
		// Stream output to stderr in real-time while capturing it
		cmd.Stdout = io.MultiWriter(&stdout, os.Stderr)
		cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	cmd.Env = os.Environ()

	start := time.Now()

	// Start the process instead of using Run() for better control
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting act: %w", err)
	}

	// Monitor context and handle graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
		// Process finished normally
	case <-ctx.Done():
		// Context cancelled - attempt graceful shutdown
		if cmd.Process != nil {
			// Try SIGTERM first for graceful shutdown
			_ = cmd.Process.Signal(syscall.SIGTERM)

			// Wait 5 seconds for graceful shutdown
			gracefulTimeout := time.After(5 * time.Second)
			select {
			case err = <-done:
				// Gracefully exited
			case <-gracefulTimeout:
				// Force kill if still running
				_ = cmd.Process.Kill()
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

func buildArgs(cfg *RunConfig) []string {
	var args []string

	if cfg.WorkflowPath != "" {
		args = append(args, "-W", cfg.WorkflowPath)
	}

	if cfg.Event != "" {
		args = append(args, cfg.Event)
	}

	// ALWAYS use verbose mode to capture step output for error extraction
	// Use medium-sized images to avoid interactive prompt on first run
	// Docker-resilient flags to prevent container buildup and failures
	args = append(args,
		"-v", // Always verbose (regardless of whether user wants to see it)
		"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
		"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
		"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04",
		"--rm",              // Remove containers after execution
		"--no-cache-server", // Disable cache server (can cause hangs/failures)
	)

	return args
}
