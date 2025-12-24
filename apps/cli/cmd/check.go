package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/act"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/preflight"
	"github.com/detent/cli/internal/signal"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/util"
	"github.com/detent/cli/internal/workflow"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

const (
	actTimeout = 35 * time.Minute

	// Channel buffer sizes
	logChannelBufferSize = 100 // Buffer size for streaming act logs to TUI
)

var (
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

func runCheck(cmd *cobra.Command, args []string) error {
	if outputFormat != "text" && outputFormat != "json" && outputFormat != "json-detailed" {
		return fmt.Errorf("invalid output format %q: must be 'text', 'json', or 'json-detailed'", outputFormat)
	}

	absRepoPath, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	workflowPath := filepath.Join(absRepoPath, workflowsDir)

	// Generate run ID early for worktree creation
	runID, err := util.GenerateUUID()
	if err != nil {
		return fmt.Errorf("generating run ID: %w", err)
	}

	baseCtx := cmd.Context()
	ctx, cancel := context.WithTimeout(baseCtx, actTimeout)
	defer cancel()

	// Check if we're in a TTY (use TUI)
	useTUI := isatty.IsTerminal(os.Stderr.Fd())

	var tmpDir string
	var worktreeInfo *git.WorktreeInfo
	var cleanupWorkflows func()
	var cleanupWorktree func()

	if useTUI {
		// Run pre-flight checks with visual feedback
		tmpDir, worktreeInfo, cleanupWorkflows, cleanupWorktree, err = preflight.RunPreflightChecks(ctx, workflowPath, absRepoPath, runID, workflowFile)
		if err != nil {
			return err
		}
		defer cleanupWorkflows()
		defer cleanupWorktree()
	} else {
		// Traditional flow without pre-flight display
		tmpDir, cleanupWorkflows, err = workflow.PrepareWorkflows(workflowPath, workflowFile)
		if err != nil {
			return fmt.Errorf("preparing workflows: %w", err)
		}
		defer cleanupWorkflows()

		// Create worktree for non-TUI mode
		worktreeInfo, cleanupWorktree, err = git.PrepareWorktree(ctx, absRepoPath, runID)
		if err != nil {
			return fmt.Errorf("creating worktree: %w", err)
		}
		defer cleanupWorktree()
	}

	var result *act.RunResult
	var grouped *internalerrors.GroupedErrors
	var cancelled bool

	if useTUI {
		result, grouped, cancelled, err = runWithTUIAndExtract(ctx, worktreeInfo.Path, tmpDir)
		// Check for cancellation in TUI mode
		if cancelled {
			signal.PrintCancellationMessage("check")
			return nil
		}
	} else {
		result, err = runWithoutTUI(ctx, worktreeInfo.Path, tmpDir)
	}

	if err != nil {
		return fmt.Errorf("running act: %w", err)
	}

	// Extract errors if not already done (TUI mode extracts early)
	var extracted []*internalerrors.ExtractedError
	if grouped == nil {
		combinedOutput := result.Stdout + result.Stderr
		extractor := internalerrors.NewExtractor()
		extracted = extractor.Extract(combinedOutput)
		internalerrors.ApplySeverity(extracted)
		grouped = internalerrors.GroupByFileWithBase(extracted, absRepoPath)
	} else {
		// Error flattening: TUI mode already extracted and grouped errors for display.
		// Now we need to flatten the grouped structure back into a linear list for
		// persistence. This reconstructs the original extracted errors from the
		// GroupedErrors structure by iterating through all file groups and combining
		// them with ungrouped errors (those without file locations).
		//
		// grouped.ByFile is a map[string][]*ExtractedError (file path -> errors in that file)
		// grouped.NoFile is []*ExtractedError (errors without file locations)
		for _, errs := range grouped.ByFile {
			extracted = append(extracted, errs...)
		}
		extracted = append(extracted, grouped.NoFile...)
	}

	// Extract workflow name for database
	workflowName := "all"
	if workflowFile != "" {
		workflowName = workflowFile
	}

	// Detect execution mode
	execMode := git.DetectExecutionMode()

	// Persist results to .detent/ directory
	recorder, err := persistence.NewRecorder(absRepoPath, workflowName, worktreeInfo.BaseCommitSHA, execMode, worktreeInfo.IsDirty, worktreeInfo.DirtyFiles)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence storage at %s/.detent: %w", absRepoPath, err)
	}

	// Record all findings
	for i, finding := range extracted {
		if err := recorder.RecordFinding(finding); err != nil {
			return fmt.Errorf("failed to record finding %d/%d to persistence storage: %w", i+1, len(extracted), err)
		}
	}

	// Finalize the run with exit code (this also closes the database connection)
	if err := recorder.Finalize(result.ExitCode); err != nil {
		return fmt.Errorf("failed to finalize persistence storage (run data may be incomplete): %w", err)
	}

	// Inform user of the output location in non-TUI text mode
	if !useTUI && outputFormat == "text" {
		_, _ = fmt.Fprintf(os.Stderr, "\nResults saved to: %s\n", recorder.GetOutputPath())
	}

	// Only print error report in non-TUI mode (TUI shows it in completion view)
	if !useTUI {
		switch outputFormat {
		case "json":
			if err := output.FormatJSON(os.Stdout, grouped); err != nil {
				return fmt.Errorf("formatting JSON output: %w", err)
			}
		case "json-detailed":
			// Use already-extracted errors to create comprehensive grouping
			groupedDetailed := internalerrors.GroupComprehensive(extracted, absRepoPath)
			if err := output.FormatJSONDetailed(os.Stdout, groupedDetailed); err != nil {
				return fmt.Errorf("formatting JSON detailed output: %w", err)
			}
		default:
			output.FormatText(os.Stdout, grouped)
		}
	}

	// Return error if act failed or if there are actual errors (not just warnings)
	if result.ExitCode != 0 {
		return fmt.Errorf("workflow execution failed with exit code %d", result.ExitCode)
	}

	// Check if there are any actual errors (not warnings) using O(1) method
	if grouped.HasErrors() {
		return fmt.Errorf("found errors in workflow execution")
	}

	return nil
}

func runWithTUIAndExtract(ctx context.Context, repoPath, tmpDir string) (*act.RunResult, *internalerrors.GroupedErrors, bool, error) {
	// Create a cancellable context to allow TUI to cancel on 'q'
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	model := tui.NewCheckModel(cancel)
	program := tea.NewProgram(
		&model,
		tea.WithContext(ctx), // Integrate context with Bubble Tea
	)

	logChan := make(chan string, logChannelBufferSize)

	// Result structure to pass data from log processor to main thread
	type tuiResult struct {
		result  *act.RunResult
		grouped *internalerrors.GroupedErrors
		err     error
	}
	tuiResultChan := make(chan tuiResult, 1)

	// Start act in a goroutine
	go func() {
		// Determine workflow path: if specific workflow is requested, use it; otherwise use directory
		workflowPath := tmpDir
		if workflowFile != "" {
			workflowPath = filepath.Join(tmpDir, workflowFile)
		}

		cfg := &act.RunConfig{
			WorkflowPath: workflowPath,
			Event:        event,
			Verbose:      false,
			WorkDir:      repoPath,
			StreamOutput: false,
			LogChan:      logChan,
		}

		result, err := act.Run(ctx, cfg)
		close(logChan)

		// Send result through tuiResultChan after processing
		if err != nil {
			tuiResultChan <- tuiResult{err: err}
			program.Send(tui.ErrMsg(err))
			return
		}

		// Extract errors once before sending to TUI
		combinedOutput := result.Stdout + result.Stderr
		extractor := internalerrors.NewExtractor()
		extracted := extractor.Extract(combinedOutput)
		internalerrors.ApplySeverity(extracted)
		grouped := internalerrors.GroupByFileWithBase(extracted, repoPath)

		// Check if context was cancelled
		cancelled := errors.Is(ctx.Err(), context.Canceled)

		// Send to TUI for display
		program.Send(tui.DoneMsg{
			Duration:  result.Duration,
			ExitCode:  result.ExitCode,
			Errors:    grouped,
			Cancelled: cancelled,
		})

		// Send to result channel for return value
		tuiResultChan <- tuiResult{
			result:  result,
			grouped: grouped,
		}
	}()

	// Log processor forwards logs to TUI
	go func() {
		for line := range logChan {
			program.Send(tui.LogMsg(line))

			// Parse for progress information
			if progress := tui.ParseActProgress(line); progress != nil {
				program.Send(*progress)
			}
		}
	}()

	// Wait for TUI to finish
	finalModel, err := program.Run()
	if err != nil {
		return nil, nil, false, err
	}

	// Extract cancellation status from the model
	checkModel, ok := finalModel.(*tui.CheckModel)
	var wasCancelled bool
	if ok {
		wasCancelled = checkModel.Cancelled
	}

	// Get the result from the channel (already extracted and processed)
	result := <-tuiResultChan
	if result.err != nil {
		return nil, nil, false, result.err
	}

	return result.result, result.grouped, wasCancelled, nil
}

func runWithoutTUI(ctx context.Context, repoPath, tmpDir string) (*act.RunResult, error) {
	_, _ = fmt.Fprintf(os.Stderr, "Running workflows... ")

	// Determine workflow path: if specific workflow is requested, use it; otherwise use directory
	workflowPath := tmpDir
	if workflowFile != "" {
		workflowPath = filepath.Join(tmpDir, workflowFile)
	}

	cfg := &act.RunConfig{
		WorkflowPath: workflowPath,
		Event:        event,
		Verbose:      false,
		WorkDir:      repoPath,
		StreamOutput: false,
	}

	result, err := act.Run(ctx, cfg)
	if err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintf(os.Stderr, "done (%s)\n", result.Duration)

	return result, nil
}
