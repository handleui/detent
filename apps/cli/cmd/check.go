package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/runner"
	"github.com/detent/cli/internal/sentry"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/util"
	"github.com/detentsh/core/git"
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
  detent check --output json`,
	Args:          cobra.NoArgs,
	RunE:          runCheck,
	SilenceUsage:  true, // Don't show usage on runtime errors
	SilenceErrors: true, // We handle errors ourselves
}

func init() {
	checkCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "output format (text, json, json-detailed)")
	checkCmd.Flags().StringVarP(&event, "event", "e", "push", "GitHub event type (push, pull_request, etc.)")
}

// buildRunConfig validates flags, resolves paths, generates UUID, and builds a RunConfig.
// Returns the runner configuration or an error.
func buildRunConfig() (*runner.RunConfig, error) {
	if outputFormat != "text" && outputFormat != "json" && outputFormat != "json-detailed" {
		return nil, fmt.Errorf("invalid output format %q: must be 'text', 'json', or 'json-detailed'", outputFormat)
	}

	absRepoPath, err := filepath.Abs(".")
	if err != nil {
		return nil, fmt.Errorf("resolving current directory: %w", err)
	}

	workflowPath := filepath.Join(absRepoPath, workflowsDir)

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
	if cfg.UseTUI {
		return nil
	}

	switch outputFormat {
	case "json":
		if err := output.FormatJSON(os.Stdout, result.Grouped); err != nil {
			return fmt.Errorf("formatting JSON output: %w", err)
		}
	case "json-detailed":
		if err := output.FormatJSONDetailed(os.Stdout, result.GroupedComprehensive); err != nil {
			return fmt.Errorf("formatting JSON detailed output: %w", err)
		}
	default:
		output.FormatText(os.Stdout, result.GroupedComprehensive)
	}

	return nil
}

// checkWorkflowStatus examines the workflow execution result and returns an error if the workflow failed
// or if errors (not just warnings) were found.
func checkWorkflowStatus(result *runner.RunResult) error {
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
		var details string
		if errorCount > 0 || warningCount > 0 {
			var parts []string
			if errorCount > 0 {
				word := "error"
				if errorCount > 1 {
					word = "errors"
				}
				parts = append(parts, fmt.Sprintf("%d %s", errorCount, word))
			}
			if warningCount > 0 {
				word := "warning"
				if warningCount > 1 {
					word = "warnings"
				}
				parts = append(parts, fmt.Sprintf("%d %s", warningCount, word))
			}
			details = " (" + strings.Join(parts, ", ") + ")"
		}
		summary = fmt.Sprintf("  %s %s", icon, tui.ErrorStyle.Render(fmt.Sprintf("Check failed in %s%s", durationStr, details)))
	}

	_, _ = fmt.Fprintln(os.Stderr, summary)
}

// printExitMessage prints the final exit message with timing.
// Format: "✓ No errors found in 2.3s" or "✗ Found 12 errors in 2.3s"
func printExitMessage(result *runner.RunResult) {
	duration := time.Since(StartTime).Round(100 * time.Millisecond)
	durationStr := util.FormatDurationCompact(duration)

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

	cfg, err := buildRunConfig()
	if err != nil {
		sentry.CaptureError(err)
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), runner.ActTimeout)
	defer cancel()

	r := runner.New(cfg)
	defer r.Cleanup()

	sentry.AddBreadcrumb("check", "running preflight checks")
	if cfg.UseTUI {
		err = r.PrepareWithTUI(ctx)
	} else {
		err = r.Prepare(ctx)
	}
	if err != nil {
		sentry.CaptureError(err)
		return err
	}

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

		printCompletionSummary(r.GetResult())
	}

	if cancelled {
		return fmt.Errorf("cancelled")
	}

	sentry.AddBreadcrumb("check", "persisting results")
	err = r.Persist()
	if err != nil {
		sentry.CaptureError(err)
		return err
	}

	result := r.GetResult()

	err = displayOutput(cfg, result)
	if err != nil {
		return err
	}

	if cfg.UseTUI {
		printExitMessage(result)
	}

	return checkWorkflowStatus(result)
}

// runCheckWithTUI executes a check run using the TUI interface.
// It creates the TUI program and delegates execution to the CheckRunner's RunWithTUI method.
func runCheckWithTUI(ctx context.Context, r *runner.CheckRunner) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	model := tui.NewCheckModel(cancel)
	program := tea.NewProgram(
		&model,
		tea.WithContext(ctx),
	)
	logChan := make(chan string, logChannelBufferSize)

	return r.RunWithTUI(ctx, logChan, program)
}
