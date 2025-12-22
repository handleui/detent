package act

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	err := cmd.Run()
	duration := time.Since(start)

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

	if cfg.Verbose {
		args = append(args, "-v")
	}

	// Use medium-sized images to avoid interactive prompt on first run
	args = append(args,
		"-P", "ubuntu-latest=catthehacker/ubuntu:act-latest",
		"-P", "ubuntu-22.04=catthehacker/ubuntu:act-22.04",
		"-P", "ubuntu-20.04=catthehacker/ubuntu:act-20.04")

	return args
}
