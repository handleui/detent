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
		StreamOutput: !useTUI, // Non-TUI mode streams act logs for verbose output
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

// printCompletionSummary prints a completion summary line for non-TUI/verbose mode.
// Format: "  ✓ Check passed in 12s" or "  ✗ Check failed in 45s (2 errors, 1 warning)"
func printCompletionSummary(result *runner.RunResult) {
	duration := result.Duration.Round(time.Second)
	durationStr := duration.String()

	// Count errors and warnings
	var errorCount, warningCount int
	for _, err := range result.Extracted {
		switch err.Severity {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		}
	}

	success := result.Success()
	icon := tui.StatusIcon(success)

	var summary string
	if success {
		summary = fmt.Sprintf("  %s %s", icon, tui.SuccessStyle.Render(fmt.Sprintf("Check passed in %s", durationStr)))
	} else {
		// Build details string
		var details string
		if errorCount > 0 || warningCount > 0 {
			parts := []string{}
			if errorCount > 0 {
				parts = append(parts, fmt.Sprintf("%d error", errorCount))
				if errorCount > 1 {
					parts[len(parts)-1] += "s"
				}
			}
			if warningCount > 0 {
				parts = append(parts, fmt.Sprintf("%d warning", warningCount))
				if warningCount > 1 {
					parts[len(parts)-1] += "s"
				}
			}
			details = " (" + joinParts(parts) + ")"
		}
		summary = fmt.Sprintf("  %s %s", icon, tui.ErrorStyle.Render(fmt.Sprintf("Check failed in %s%s", durationStr, details)))
	}

	_, _ = fmt.Fprintln(os.Stderr, summary)
}

// joinParts joins string parts with ", "
func joinParts(parts []string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += ", "
		}
		result += part
	}
	return result
}

// printExitMessage prints the final exit message with timing.
// Format: "✓ No errors found in 2.3s" or "✗ Found 12 errors in 2.3s"
func printExitMessage(result *runner.RunResult) {
	duration := time.Since(StartTime).Round(100 * time.Millisecond)
	durationStr := formatDuration(duration)

	// Count errors only (not warnings)
	var errorCount int
	for _, err := range result.Extracted {
		if err.Severity == "error" {
			errorCount++
		}
	}

	var msg string
	if errorCount == 0 {
		msg = tui.ExitSuccess(fmt.Sprintf("No errors found in %s", durationStr))
	} else {
		errorWord := "error"
		if errorCount > 1 {
			errorWord = "errors"
		}
		msg = tui.ExitError(fmt.Sprintf("Found %d %s in %s", errorCount, errorWord, durationStr))
	}

	fmt.Fprintln(os.Stderr, msg)
}

// formatDuration formats a duration in a human-readable way (e.g., "2.3s", "1m 23s")
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
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
		_, _ = fmt.Fprintln(os.Stderr)
		_, _ = fmt.Fprintln(os.Stderr, tui.SecondaryStyle.Render("─── Workflow Output ───"))
		_, _ = fmt.Fprintln(os.Stderr)
		err = r.Run(ctx)
		if err != nil {
			sentry.CaptureError(err)
			return fmt.Errorf("running act: %w", err)
		}
		_, _ = fmt.Fprintln(os.Stderr)

		// Print completion summary
		printCompletionSummary(r.GetResult())
	}

	// Handle cancellation - let main.go print the error message
	if cancelled {
		return fmt.Errorf("cancelled")
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

	// Print exit message with timing (TUI mode - non-TUI already printed via printCompletionSummary)
	if cfg.UseTUI {
		printExitMessage(result)
	}

	// Check status and return
	return checkWorkflowStatus(result)
}


// runCheckDryRun shows simulated TUI without actual workflow execution.
func runCheckDryRun(ctx context.Context, cfg *runner.RunConfig) error {
	if !cfg.UseTUI {
		_, _ = fmt.Fprintln(os.Stderr, tui.SecondaryStyle.Render("  Preparing workspace..."))
		_, _ = fmt.Fprintln(os.Stderr, tui.SuccessStyle.Render("  ✓ Ready"))
		_, _ = fmt.Fprintln(os.Stderr)
		_, _ = fmt.Fprintln(os.Stderr, tui.SecondaryStyle.Render("─── Workflow Output (dry-run) ───"))
		_, _ = fmt.Fprintln(os.Stderr, tui.MutedStyle.Render("  [no output in dry-run mode]"))
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

// simulateDryRunPreflight displays animated preflight checks matching the real flow.
func simulateDryRunPreflight() {
	checks := []string{
		"Validating repository",
		"Checking prerequisites",
		"Preparing workflows",
		"Creating workspace",
	}

	// Single-line display that updates
	for i, check := range checks {
		if i > 0 {
			fmt.Fprint(os.Stderr, "\033[1A\033[J") // Clear previous line
		}
		fmt.Fprintln(os.Stderr, tui.PrimaryStyle.Render("· "+check))
		time.Sleep(150 * time.Millisecond)
	}

	// Clear the line on success
	fmt.Fprint(os.Stderr, "\033[1A\033[J")
}

// simulateDryRunProgress sends simulated workflow progress to the TUI.
// Matches the real act output format exactly for 1:1 faithful preview.
func simulateDryRunProgress(ctx context.Context, logChan chan string, program *tea.Program) {
	defer close(logChan)

	// Simulated workflow steps - uses [dry-run] prefix for logs to indicate mock data.
	// JobID matches what the TUI expects for step matching.
	steps := []struct {
		jobID string
		logs  []string
		delay time.Duration
	}{
		{
			jobID: "build",
			logs: []string{
				"[dry-run] Starting container...",
				"[dry-run] Pulling image: node:18-alpine",
			},
			delay: 500 * time.Millisecond,
		},
		{
			jobID: "lint",
			logs: []string{
				"[dry-run] eslint .",
				"[dry-run] Running linter...",
			},
			delay: 400 * time.Millisecond,
		},
		{
			jobID: "test",
			logs: []string{
				"[dry-run] npm test",
				"[dry-run] Running tests...",
			},
			delay: 600 * time.Millisecond,
		},
	}

	for _, step := range steps {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Send progress update with JobID for step matching
		program.Send(tui.ProgressMsg{
			Status: step.jobID,
			JobID:  step.jobID,
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

	// Setup TUI program with pre-populated jobs from workflow files
	model := tui.NewCheckModelWithJobs(cancel, r.GetJobs())
	program := tea.NewProgram(
		&model,
		tea.WithContext(ctx),
	)
	logChan := make(chan string, logChannelBufferSize)

	// Run with TUI
	return r.RunWithTUI(ctx, logChan, program)
}
