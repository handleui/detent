package act

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	return args
}
