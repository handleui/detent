package cmd

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/act"
	"github.com/detent/cli/internal/commands"
	"github.com/detent/cli/internal/docker"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/signal"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/workflow"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

const (
	actTimeout               = 35 * time.Minute
	preflightVisualDelay     = 200 * time.Millisecond
	preflightTransitionDelay = 100 * time.Millisecond
	preflightCompletionPause = 300 * time.Millisecond

	// Channel buffer sizes
	logChannelBufferSize = 100 // Buffer size for streaming act logs to TUI

	// UUID v4 bit manipulation constants
	uuidVersionMask = 0x0f
	uuidVersion4    = 0x40
	uuidVariantMask = 0x3f
	uuidVariantRFC  = 0x80

	// UUID byte positions for version and variant
	uuidVersionByteIndex = 6
	uuidVariantByteIndex = 8

	// UUID byte slice sizes for formatting
	uuidBytesTotal  = 16
	uuidSlice1End   = 4
	uuidSlice2Start = 4
	uuidSlice2End   = 6
	uuidSlice3Start = 6
	uuidSlice3End   = 8
	uuidSlice4Start = 8
	uuidSlice4End   = 10
	uuidSlice5Start = 10
)

var (
	// Command-specific flags
	outputFormat string
	event        string

	// Pre-compiled regex for parseActProgress
	jobStepPattern = regexp.MustCompile(`^\[([^\]]+)\]\s+(.+)`)
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

// generateUUID creates a simple UUID v4 without external dependencies
func generateUUID() (string, error) {
	b := make([]byte, uuidBytesTotal)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes for UUID: %w", err)
	}
	// Set version (4) and variant bits
	b[uuidVersionByteIndex] = (b[uuidVersionByteIndex] & uuidVersionMask) | uuidVersion4
	b[uuidVariantByteIndex] = (b[uuidVariantByteIndex] & uuidVariantMask) | uuidVariantRFC
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:uuidSlice1End],
		b[uuidSlice2Start:uuidSlice2End],
		b[uuidSlice3Start:uuidSlice3End],
		b[uuidSlice4Start:uuidSlice4End],
		b[uuidSlice5Start:]), nil
}

// applySeverity infers severity for all extracted errors based on their category.
// This is done as explicit post-processing after extraction to maintain separation
// of concerns: extraction is pure parsing, severity is business logic.
func applySeverity(extractedErrors []*internalerrors.ExtractedError) {
	for _, err := range extractedErrors {
		err.Severity = internalerrors.InferSeverity(err)
	}
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
	runID, err := generateUUID()
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
		tmpDir, worktreeInfo, cleanupWorkflows, cleanupWorktree, err = runPreflightChecks(ctx, workflowPath, absRepoPath, runID)
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
		applySeverity(extracted)
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

	// Check if there are any actual errors (not warnings)
	hasErrors := false
	for _, issues := range grouped.ByFile {
		for _, issue := range issues {
			if issue.Severity == "error" {
				hasErrors = true
				break
			}
		}
		if hasErrors {
			break
		}
	}

	if !hasErrors {
		for _, issue := range grouped.NoFile {
			if issue.Severity == "error" {
				hasErrors = true
				break
			}
		}
	}

	if hasErrors {
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
		applySeverity(extracted)
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
			if progress := parseActProgress(line); progress != nil {
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

// parseActProgress extracts progress information from act output
func parseActProgress(line string) *tui.ProgressMsg {
	// Pattern: [Job Name/Step Name] or similar
	// act outputs lines like: "[job-name] 🚀  Start image..."
	// "[job-name]   ✅  Success - Step Name"
	// We'll parse these to extract current step info

	// Match job/step patterns using pre-compiled regex
	matches := jobStepPattern.FindStringSubmatch(line)

	if len(matches) >= 3 {
		jobName := matches[1]
		stepInfo := strings.TrimSpace(matches[2])

		// Clean up step info
		stepInfo = strings.TrimPrefix(stepInfo, "🚀  ")
		stepInfo = strings.TrimPrefix(stepInfo, "✅  ")
		stepInfo = strings.TrimPrefix(stepInfo, "❌  ")

		status := fmt.Sprintf("%s: %s", jobName, stepInfo)

		return &tui.ProgressMsg{
			Status:      status,
			CurrentStep: 0,
			TotalSteps:  0,
		}
	}

	return nil
}

// runPreflightChecks performs and displays pre-flight checks before running act
func runPreflightChecks(ctx context.Context, workflowPath, repoRoot, runID string) (tmpDir string, worktreeInfo *git.WorktreeInfo, cleanupWorkflows, cleanupWorktree func(), err error) {
	checks := []string{
		"Checking act installation",
		"Checking Docker availability",
		"Preparing workflows (injecting continue-on-error)",
		"Creating temporary workspace",
		"Creating isolated worktree",
	}

	display := tui.NewPreflightDisplay(checks)
	display.Render()

	// Check 1: Act installation
	time.Sleep(preflightVisualDelay)
	display.UpdateCheck("Checking act installation", "running", nil)
	display.Render()

	err = commands.CheckAct()
	if err != nil {
		display.UpdateCheck("Checking act installation", "error", err)
		display.RenderFinal()
		return "", nil, nil, nil, err
	}

	display.UpdateCheck("Checking act installation", "success", nil)
	display.Render()

	// Check 2: Docker availability
	time.Sleep(preflightTransitionDelay)
	display.UpdateCheck("Checking Docker availability", "running", nil)
	display.Render()

	err = docker.IsAvailable(ctx)
	if err != nil {
		display.UpdateCheck("Checking Docker availability", "error", err)
		display.RenderFinal()
		return "", nil, nil, nil, fmt.Errorf("docker is not available: %w", err)
	}

	display.UpdateCheck("Checking Docker availability", "success", nil)
	display.Render()

	// Check 3: Prepare workflows
	time.Sleep(preflightTransitionDelay)
	display.UpdateCheck("Preparing workflows (injecting continue-on-error)", "running", nil)
	display.Render()

	tmpDir, cleanupWorkflows, err = workflow.PrepareWorkflows(workflowPath, workflowFile)
	if err != nil {
		display.UpdateCheck("Preparing workflows (injecting continue-on-error)", "error", err)
		display.RenderFinal()
		return "", nil, nil, nil, fmt.Errorf("preparing workflows: %w", err)
	}

	display.UpdateCheck("Preparing workflows (injecting continue-on-error)", "success", nil)
	display.UpdateCheck("Creating temporary workspace", "success", nil)
	display.Render()

	// Check 4: Create worktree
	time.Sleep(preflightTransitionDelay)
	display.UpdateCheck("Creating isolated worktree", "running", nil)
	display.Render()

	worktreeInfo, cleanupWorktree, err = git.PrepareWorktree(ctx, repoRoot, runID)
	if err != nil {
		display.UpdateCheck("Creating isolated worktree", "error", err)
		display.RenderFinal()
		cleanupWorkflows()
		return "", nil, nil, nil, fmt.Errorf("creating worktree: %w", err)
	}

	display.UpdateCheck("Creating isolated worktree", "success", nil)
	display.Render()

	// Small pause to show all checks passed
	time.Sleep(preflightCompletionPause)

	// Add a blank line before TUI starts
	fmt.Fprintln(os.Stderr)

	return tmpDir, worktreeInfo, cleanupWorkflows, cleanupWorktree, nil
}
