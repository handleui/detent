package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/act"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/preflight"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/workflow"
)

// CheckRunner orchestrates a complete check run lifecycle including:
// - Workflow preparation (injection of continue-on-error and timeouts)
// - Worktree creation and isolation
// - Act execution with proper output capture
// - Error extraction and grouping
// - Result persistence
// - Resource cleanup
//
// The runner implements a clear separation between preparation (Prepare/PrepareWithTUI),
// execution (Run/RunWithTUI), persistence (Persist), and cleanup (Cleanup) phases.
// This allows for flexible usage patterns while maintaining proper resource management.
//
// Usage pattern:
//
//	runner := New(config)
//	defer runner.Cleanup()
//
//	if err := runner.Prepare(ctx); err != nil {
//	    return err
//	}
//
//	if err := runner.Run(ctx); err != nil {
//	    return err
//	}
//
//	if err := runner.Persist(); err != nil {
//	    return err
//	}
//
//	result := runner.GetResult()
type CheckRunner struct {
	config *RunConfig // Configuration for this run

	// Cleanup functions - set during Prepare phase
	tmpDir           string            // Temporary directory for workflow files
	worktreeInfo     *git.WorktreeInfo // Worktree metadata including path and commit info
	cleanupWorkflows func()            // Cleanup function for workflow temp directory
	cleanupWorktree  func()            // Cleanup function for worktree

	// Execution state - set during Run phase
	startTime time.Time  // When execution started
	result    *RunResult // Complete run result including act output, errors, and metadata
}

// New creates a new CheckRunner with the given configuration.
// The runner is not initialized until Prepare or PrepareWithTUI is called.
func New(config *RunConfig) *CheckRunner {
	return &CheckRunner{
		config: config,
	}
}

// Prepare sets up the execution environment including:
// - Workflow file preparation (continue-on-error and timeout injection)
// - Worktree creation for isolated execution
//
// This must be called before Run. All resources are tracked for cleanup.
// Returns error if preparation fails. On error, partial resources are cleaned up.
func (r *CheckRunner) Prepare(ctx context.Context) error {
	// Prepare workflows with continue-on-error injection
	tmpDir, cleanupWorkflows, err := workflow.PrepareWorkflows(r.config.WorkflowPath, "")
	if err != nil {
		return fmt.Errorf("preparing workflows: %w", err)
	}
	r.tmpDir = tmpDir
	r.cleanupWorkflows = cleanupWorkflows

	// Create isolated worktree for execution
	worktreeInfo, cleanupWorktree, err := git.PrepareWorktree(ctx, r.config.RepoRoot, r.config.RunID)
	if err != nil {
		// Cleanup workflows before returning error
		cleanupWorkflows()
		return fmt.Errorf("creating worktree: %w", err)
	}
	r.worktreeInfo = worktreeInfo
	r.cleanupWorktree = cleanupWorktree

	return nil
}

// PrepareWithTUI is like Prepare but sends progress updates to the TUI.
// It performs the same preparation steps but updates the UI with status messages.
//
// This must be called before RunWithTUI. All resources are tracked for cleanup.
// Returns error if preparation fails. On error, partial resources are cleaned up.
func (r *CheckRunner) PrepareWithTUI(ctx context.Context) error {
	// Run preflight checks with visual feedback
	result, err := preflight.RunPreflightChecks(ctx, r.config.WorkflowPath, r.config.RepoRoot, r.config.RunID, "")
	if err != nil {
		return err
	}

	// Store preparation results
	r.tmpDir = result.TmpDir
	r.worktreeInfo = result.WorktreeInfo
	r.cleanupWorkflows = result.CleanupWorkflows
	r.cleanupWorktree = result.CleanupWorktree

	return nil
}

// Run executes the workflow using act and extracts errors from the output.
// This is the main execution phase that:
// - Runs act with proper output capture
// - Extracts errors from workflow output using pattern matching
// - Groups errors by file path
// - Tracks execution duration
//
// Prepare must be called before Run. Returns error if execution setup fails.
// Note: A non-zero exit code from act is not treated as an error - it's captured
// in the result for analysis.
func (r *CheckRunner) Run(ctx context.Context) error {
	r.startTime = time.Now()

	// Configure act execution (matching runWithoutTUI logic from cmd/check.go:476-490)
	actConfig := &act.RunConfig{
		WorkflowPath: r.tmpDir,
		Event:        r.config.Event,
		Verbose:      false,
		WorkDir:      r.worktreeInfo.Path,
		StreamOutput: r.config.StreamOutput,
		LogChan:      nil,
	}

	// Execute workflow using act
	actResult, err := act.Run(ctx, actConfig)
	if err != nil {
		return err
	}

	// Extract and process errors (matching cmd/check.go:266-278)
	extracted, grouped := r.extractAndProcessErrors(actResult)

	// Store result
	r.result = &RunResult{
		ActResult:    actResult,
		Extracted:    extracted,
		Grouped:      grouped,
		WorktreeInfo: r.worktreeInfo,
		RunID:        r.config.RunID,
		StartTime:    r.startTime,
		Duration:     actResult.Duration,
		ExitCode:     actResult.ExitCode,
	}

	return nil
}

// RunWithTUI is like Run but streams output to TUI and sends progress updates.
// It performs the same execution steps but integrates with the Bubble Tea UI:
// - Streams act output to the log channel for real-time display
// - Sends progress updates for status changes
// - Sends final completion message with results
//
// PrepareWithTUI must be called before RunWithTUI. Returns whether the run
// was cancelled and any error that occurred during execution setup.
//
// Note: A non-zero exit code from act is not treated as an error - it's captured
// in the result and sent via DoneMsg.
func (r *CheckRunner) RunWithTUI(ctx context.Context, logChan chan string, program *tea.Program) (bool, error) {
	// Create cancellable context for TUI control (matching runWithTUIAndExtract:462-464)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	r.startTime = time.Now()

	// Configure act with TUI integration
	actConfig := &act.RunConfig{
		WorkflowPath: r.tmpDir,
		Event:        r.config.Event,
		Verbose:      false,
		WorkDir:      r.worktreeInfo.Path,
		StreamOutput: false,
		LogChan:      logChan,
	}

	// Start act runner in goroutine (matching startActRunner:376-408)
	type tuiResult struct {
		result  *act.RunResult
		grouped *internalerrors.GroupedErrors
		err     error
	}
	resultChan := make(chan tuiResult, 1)

	go func() {
		result, err := act.Run(ctx, actConfig)
		close(logChan)

		if err != nil {
			resultChan <- tuiResult{err: err}
			program.Send(tui.ErrMsg(err))
			return
		}

		// Extract errors before sending to TUI
		_, grouped := r.extractAndProcessErrors(result)

		// Check if context was cancelled
		cancelled := errors.Is(ctx.Err(), context.Canceled)

		// Send completion to TUI
		program.Send(tui.DoneMsg{
			Duration:  result.Duration,
			ExitCode:  result.ExitCode,
			Errors:    grouped,
			Cancelled: cancelled,
		})

		resultChan <- tuiResult{
			result:  result,
			grouped: grouped,
		}
	}()

	// Start log processor (matching startLogProcessor:411-435)
	go func() {
		for {
			select {
			case line, ok := <-logChan:
				if !ok {
					return
				}

				select {
				case <-ctx.Done():
					return
				default:
					program.Send(tui.LogMsg(line))

					if progress := tui.ParseActProgress(line); progress != nil {
						program.Send(*progress)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for TUI completion (matching collectTUIResult:438-458)
	finalModel, err := program.Run()
	if err != nil {
		return false, err
	}

	// Extract cancellation status from model
	checkModel, ok := finalModel.(*tui.CheckModel)
	var wasCancelled bool
	if ok {
		wasCancelled = checkModel.Cancelled
	}

	// Get result from channel
	tuiRes := <-resultChan
	if tuiRes.err != nil {
		return false, tuiRes.err
	}

	// Extract errors for full result
	extracted, grouped := r.extractAndProcessErrors(tuiRes.result)

	// Store result
	r.result = &RunResult{
		ActResult:    tuiRes.result,
		Extracted:    extracted,
		Grouped:      grouped,
		WorktreeInfo: r.worktreeInfo,
		RunID:        r.config.RunID,
		StartTime:    r.startTime,
		Duration:     tuiRes.result.Duration,
		Cancelled:    wasCancelled,
		ExitCode:     tuiRes.result.ExitCode,
	}

	return wasCancelled, nil
}

// extractAndProcessErrors extracts errors from act output, applies severity, and groups by file.
// Matching logic from cmd/check.go:266-278
func (r *CheckRunner) extractAndProcessErrors(actResult *act.RunResult) ([]*internalerrors.ExtractedError, *internalerrors.GroupedErrors) {
	var combinedOutput strings.Builder
	combinedOutput.Grow(len(actResult.Stdout) + len(actResult.Stderr))
	combinedOutput.WriteString(actResult.Stdout)
	combinedOutput.WriteString(actResult.Stderr)

	extractor := internalerrors.NewExtractor()
	extracted := extractor.Extract(combinedOutput.String())
	internalerrors.ApplySeverity(extracted)
	grouped := internalerrors.GroupByFileWithBase(extracted, r.config.RepoRoot)

	return extracted, grouped
}

// Persist saves the check run results to the database.
// This writes the complete run result including:
// - Run metadata (ID, timing, exit code)
// - Worktree information (base commit, dirty state)
// - Extracted errors with full context
//
// Returns error if persistence fails. This should be called after Run/RunWithTUI
// completes successfully.
func (r *CheckRunner) Persist() error {
	if r.result == nil {
		return fmt.Errorf("no result to persist (Run/RunWithTUI must be called first)")
	}

	if r.worktreeInfo == nil {
		return fmt.Errorf("no worktree info available (Prepare/PrepareWithTUI must be called first)")
	}

	// Extract workflow name from config.WorkflowPath
	// If WorkflowPath is a file, use its base name; otherwise use "all"
	workflowName := "all"
	fileInfo, err := os.Stat(r.config.WorkflowPath)
	if err == nil && !fileInfo.IsDir() {
		workflowName = filepath.Base(r.config.WorkflowPath)
	}

	// Detect execution mode
	execMode := git.DetectExecutionMode()

	// Initialize persistence recorder
	recorder, err := persistence.NewRecorder(
		r.config.RepoRoot,
		workflowName,
		r.worktreeInfo.BaseCommitSHA,
		execMode,
		r.worktreeInfo.IsDirty,
		r.worktreeInfo.DirtyFiles,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence storage at %s/.detent: %w", r.config.RepoRoot, err)
	}

	// Record all findings
	for i, finding := range r.result.Extracted {
		if err := recorder.RecordFinding(finding); err != nil {
			return fmt.Errorf("failed to record finding %d/%d to persistence storage: %w", i+1, len(r.result.Extracted), err)
		}
	}

	// Finalize the run with exit code (this also closes the database connection)
	if err := recorder.Finalize(r.result.ExitCode); err != nil {
		return fmt.Errorf("failed to finalize persistence storage (run data may be incomplete): %w", err)
	}

	return nil
}

// Cleanup releases all resources allocated during Prepare.
// This includes:
// - Temporary workflow directory (via cleanupWorkflows)
// - Git worktree (via cleanupWorktree)
//
// This should be called via defer after creating the runner to ensure cleanup
// happens even if preparation or execution fails. Cleanup is idempotent and
// safe to call multiple times.
func (r *CheckRunner) Cleanup() {
	if r.cleanupWorkflows != nil {
		r.cleanupWorkflows()
	}
	if r.cleanupWorktree != nil {
		r.cleanupWorktree()
	}
}

// GetResult returns the complete result of the check run.
// This should be called after Run/RunWithTUI completes.
//
// Returns nil if Run/RunWithTUI has not been called yet or if execution failed.
// The result includes all extracted errors, timing information, and act output.
func (r *CheckRunner) GetResult() *RunResult {
	return r.result
}
