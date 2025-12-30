package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/cache"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/runner"
	"github.com/detent/cli/internal/sentry"
	"github.com/detent/cli/internal/signal"
	"github.com/detent/cli/internal/tui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

const (
	// Channel buffer sizes
	logChannelBufferSize = 100 // Buffer size for streaming act logs to TUI
)

var (
	// ErrFoundErrors is returned when the check command finds errors in workflow execution.
	// This allows heal to distinguish between "errors found" (expected) and other failures.
	ErrFoundErrors = errors.New("found errors in workflow execution")

	// Command-specific flags
	outputFormat string
	event        string
	forceRun     bool
	dryRun       bool
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run workflows locally and extract errors",
	Long: `Run GitHub Actions workflows locally using act (nektos/act) with enhanced
error reporting. Automatically injects continue-on-error to ensure all steps
execute, then extracts and groups errors by file for efficient debugging.

The command performs these steps:
  1. Checks act installation and Docker availability
  2. Prepares workflows (injects continue-on-error and timeouts)
  3. Runs workflows using act
  4. Extracts and groups errors from output
  5. Saves results to .detent/ directory

Requirements:
  - Docker must be running
  - act (nektos/act) must be installed
  - Workflows in .github/workflows/ (or custom path via --workflows)

Results are persisted to .detent/ for future analysis and comparison.`,
	Example: `  # Run all workflows in current directory
  detent check

  # Run specific workflow
  detent check --workflow ci.yml

  # Trigger with pull_request event
  detent check --event pull_request

  # Use JSON output for CI integration
  detent check --output json

  # Preview UI without running workflows
  detent check --dry-run`,
	Args:          cobra.NoArgs,
	RunE:          runCheck,
	SilenceUsage:  true, // Don't show usage on runtime errors
	SilenceErrors: true, // We handle errors ourselves
}

func init() {
	checkCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "output format (text, json, json-detailed)")
	checkCmd.Flags().StringVarP(&event, "event", "e", "push", "GitHub event type (push, pull_request, etc.)")
	checkCmd.Flags().BoolVarP(&forceRun, "force", "f", false, "force fresh run, ignoring cached results")
	checkCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview UI without running workflows")
}

// buildRunConfig validates flags, resolves paths, generates UUID, and builds a RunConfig.
// Returns the runner configuration or an error.
func buildRunConfig() (*runner.RunConfig, error) {
	// Validate output format
	if outputFormat != "text" && outputFormat != "json" && outputFormat != "json-detailed" {
		return nil, fmt.Errorf("invalid output format %q: must be 'text', 'json', or 'json-detailed'", outputFormat)
	}

	// Resolve directory paths
	absRepoPath, err := filepath.Abs(".")
	if err != nil {
		return nil, fmt.Errorf("resolving current directory: %w", err)
	}

	workflowPath := filepath.Join(absRepoPath, workflowsDir)

	// Compute deterministic run ID from tree+commit hash
	runID, _, _, err := git.ComputeCurrentRunID(absRepoPath)
	if err != nil {
		return nil, err
	}

	// Determine if TUI should be used
	// Disable TUI when running in AI agent environments for better machine-readable output
	// agentInfo is set in PersistentPreRunE (root.go)
	useTUI := isatty.IsTerminal(os.Stderr.Fd()) && !agentInfo.IsAgent

	cfg := &runner.RunConfig{
		RepoRoot:     absRepoPath,
		WorkflowPath: workflowPath,
		WorkflowFile: workflowFile,
		Event:        event,
		UseTUI:       useTUI,
		StreamOutput: false,
		RunID:        runID,
		DryRun:       dryRun,
		IsAgentMode:  agentInfo.IsAgent,
		AgentName:    agentInfo.Name,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// displayOutput formats and prints the workflow results based on the output format flag.
// Only displays output in non-TUI mode (TUI shows results in completion view).
func displayOutput(cfg *runner.RunConfig, result *runner.RunResult) error {
	// Only print error report in non-TUI mode (TUI shows it in completion view)
	if cfg.UseTUI {
		return nil
	}

	switch outputFormat {
	case "json":
		if err := output.FormatJSON(os.Stdout, result.Grouped); err != nil {
			return fmt.Errorf("formatting JSON output: %w", err)
		}
	case "json-detailed":
		// Use already-extracted errors to create comprehensive grouping
		groupedDetailed := internalerrors.GroupComprehensive(result.Extracted, cfg.RepoRoot)
		if err := output.FormatJSONDetailed(os.Stdout, groupedDetailed); err != nil {
			return fmt.Errorf("formatting JSON detailed output: %w", err)
		}
	default:
		output.FormatText(os.Stdout, result.Grouped)
	}

	return nil
}

// checkWorkflowStatus examines the workflow execution result and returns an error if the workflow failed
// or if errors (not just warnings) were found.
func checkWorkflowStatus(result *runner.RunResult) error {
	// Return error if act failed
	if result.ExitCode != 0 {
		return fmt.Errorf("workflow execution failed with exit code %d", result.ExitCode)
	}

	// Check if there are any actual errors (not warnings) using O(1) method
	if result.HasErrors() {
		return ErrFoundErrors
	}

	return nil
}

// runCheck orchestrates the workflow execution and error reporting.
// It performs these steps in sequence:
// 1. Setup and validate environment configuration
// 2. Check cache for prior run (skip if --force)
// 3. Prepare workflow files and worktree
// 4. Execute workflow with appropriate UI mode
// 5. Process and persist results
// 6. Display output and return status
func runCheck(cmd *cobra.Command, args []string) error {
	sentry.AddBreadcrumb("check", "starting check command")

	// Setup: validate flags and build config
	cfg, err := buildRunConfig()
	if err != nil {
		sentry.CaptureError(err)
		return err
	}

	// Dry-run mode: show simulated TUI without actual execution
	if cfg.DryRun {
		return runCheckDryRun(cmd.Context(), cfg)
	}

	// Check cache for prior run (skip if --force)
	if !forceRun {
		result, cacheErr := cache.Check(cfg.RepoRoot, outputFormat)
		if result.Hit {
			return cacheErr // Return error if cached run had errors, nil otherwise
		}
		// If cache check fails or no hit, continue with fresh run
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(cmd.Context(), runner.ActTimeout)
	defer cancel()

	// Create runner
	r := runner.New(cfg)
	defer r.Cleanup()

	// Prepare: run preflight or standard preparation
	sentry.AddBreadcrumb("check", "running preflight checks")
	if cfg.UseTUI {
		err = r.PrepareWithTUI(ctx)
	} else {
		err = r.Prepare(ctx)
	}
	if err != nil {
		sentry.CaptureError(err)
		// Enhance dirty worktree error with instructions for AI agents
		if cfg.IsAgentMode && errors.Is(err, git.ErrWorktreeDirty) {
			return fmt.Errorf("%w\n\nTell the user to commit or stash their changes before running detent:\n"+
				"  git add . && git commit -m \"WIP\"\n"+
				"  # or\n"+
				"  git stash", err)
		}
		return err
	}

	// Execute: run workflow
	sentry.AddBreadcrumb("check", "executing workflows")
	var cancelled bool
	if cfg.UseTUI {
		cancelled, err = runCheckWithTUI(ctx, r)
		if err != nil {
			sentry.CaptureError(err)
			return fmt.Errorf("running act: %w", err)
		}
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "  Running...")
		err = r.Run(ctx)
		if err != nil {
			sentry.CaptureError(err)
			return fmt.Errorf("running act: %w", err)
		}
	}

	// Handle cancellation
	if cancelled {
		signal.PrintCancellationMessage("check")
		return nil
	}

	// Persist results
	sentry.AddBreadcrumb("check", "persisting results")
	err = r.Persist()
	if err != nil {
		sentry.CaptureError(err)
		return err
	}

	// Get result for display and status check
	result := r.GetResult()

	// Display output (command concern)
	err = displayOutput(cfg, result)
	if err != nil {
		return err
	}

	// Check status and return
	return checkWorkflowStatus(result)
}

// Timing constants for dry-run simulation
const (
	dryRunPreflightVisualDelay     = 200 * time.Millisecond
	dryRunPreflightCompletionPause = 300 * time.Millisecond
)

// runCheckDryRun shows simulated TUI without actual workflow execution.
func runCheckDryRun(ctx context.Context, cfg *runner.RunConfig) error {
	if !cfg.UseTUI {
		_, _ = fmt.Fprintln(os.Stderr, "  Preparing...")
		_, _ = fmt.Fprintln(os.Stderr, "  Running... (dry-run)")
		return nil
	}

	// Phase 1: Simulate preflight checks (1:1 faithful to real flow)
	simulateDryRunPreflight()

	// Phase 2: Main TUI with simulated workflow execution
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	model := tui.NewCheckModel(cancel)
	program := tea.NewProgram(&model, tea.WithContext(ctx))
	logChan := make(chan string, logChannelBufferSize)

	// Start goroutine to send simulated progress
	go simulateDryRunProgress(ctx, logChan, program)

	// Start log processor
	go func() {
		for {
			select {
			case line, ok := <-logChan:
				if !ok {
					return
				}
				program.Send(tui.LogMsg(line))
			case <-ctx.Done():
				return
			}
		}
	}()

	_, err := program.Run()
	return err
}

// simulateDryRunPreflight displays minimal preflight output matching the real flow.
func simulateDryRunPreflight() {
	_, _ = fmt.Fprintln(os.Stderr, "  Preparing...")
	time.Sleep(dryRunPreflightVisualDelay + dryRunPreflightCompletionPause)
}

// simulateDryRunProgress sends simulated workflow progress to the TUI.
// Matches the real act output format exactly for 1:1 faithful preview.
func simulateDryRunProgress(ctx context.Context, logChan chan string, program *tea.Program) {
	defer close(logChan)

	// Simulated workflow steps - uses [dry-run] prefix for logs to indicate mock data.
	// Status messages match the real TUI parser format for authentic progress display.
	steps := []struct {
		status string
		logs   []string
		delay  time.Duration
	}{
		{
			status: "build: Set up job",
			logs: []string{
				"[dry-run] Starting container...",
				"[dry-run] Pulling image: node:18-alpine",
			},
			delay: 500 * time.Millisecond,
		},
		{
			status: "build: Checkout",
			logs: []string{
				"[dry-run] Checking out repository",
			},
			delay: 300 * time.Millisecond,
		},
		{
			status: "build: Install dependencies",
			logs: []string{
				"[dry-run] npm ci",
				"[dry-run] Installing packages...",
			},
			delay: 600 * time.Millisecond,
		},
		{
			status: "build: Type check",
			logs: []string{
				"[dry-run] tsc --noEmit",
				"[dry-run] Simulated type error in src/app.ts:42",
			},
			delay: 400 * time.Millisecond,
		},
		{
			status: "build: Lint",
			logs: []string{
				"[dry-run] eslint .",
				"[dry-run] Simulated lint errors found",
			},
			delay: 400 * time.Millisecond,
		},
		{
			status: "build: Build",
			logs: []string{
				"[dry-run] npm run build",
				"[dry-run] Build complete",
			},
			delay: 500 * time.Millisecond,
		},
	}

	for i, step := range steps {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Send progress update (matches TUI parser expectations)
		program.Send(tui.ProgressMsg{
			Status:      step.status,
			CurrentStep: i + 1,
			TotalSteps:  len(steps),
		})

		// Send logs with realistic timing
		for _, log := range step.logs {
			select {
			case logChan <- log:
			case <-ctx.Done():
				return
			}
			time.Sleep(150 * time.Millisecond)
		}

		time.Sleep(step.delay)
	}

	// Send completion with mock errors (demonstrates error report display)
	mockExtracted := []*internalerrors.ExtractedError{
		{
			File:     "src/app.ts",
			Line:     42,
			Column:   5,
			Message:  "Type 'string' is not assignable to type 'number'",
			Severity: "error",
			Category: internalerrors.CategoryTypeCheck,
			Source:   "typescript",
		},
		{
			File:     "src/utils.ts",
			Line:     18,
			Column:   10,
			Message:  "'temp' is defined but never used",
			Severity: "warning",
			Category: internalerrors.CategoryLint,
			Source:   "eslint",
		},
		{
			File:     "src/index.ts",
			Line:     7,
			Column:   1,
			Message:  "Missing semicolon",
			Severity: "error",
			Category: internalerrors.CategoryLint,
			Source:   "eslint",
		},
	}
	mockErrors := internalerrors.GroupByFile(mockExtracted)

	program.Send(tui.DoneMsg{
		Duration: 2700 * time.Millisecond,
		ExitCode: 1,
		Errors:   mockErrors,
	})
}

// runCheckWithTUI executes a check run using the TUI interface.
// It creates the TUI program and delegates execution to the CheckRunner's RunWithTUI method.
func runCheckWithTUI(ctx context.Context, r *runner.CheckRunner) (bool, error) {
	// Create cancellable context for TUI control
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup TUI program
	model := tui.NewCheckModel(cancel)
	program := tea.NewProgram(
		&model,
		tea.WithContext(ctx),
	)
	logChan := make(chan string, logChannelBufferSize)

	// Run with TUI
	return r.RunWithTUI(ctx, logChan, program)
}
