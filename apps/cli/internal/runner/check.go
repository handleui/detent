package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// sendToTUI sends a message to the TUI program in a non-blocking manner.
// This prevents the caller from blocking if the TUI is slow to process messages.
// The send is executed in a separate goroutine to avoid backpressure on the act runner.
func sendToTUI(program *tea.Program, msg tea.Msg) {
	go program.Send(msg)
}

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

// tuiResult encapsulates the result of a TUI-based check run.
type tuiResult struct {
	result    *act.RunResult
	extracted []*internalerrors.ExtractedError
	grouped   *internalerrors.GroupedErrors
	err       error
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
	tmpDir, cleanupWorkflows, err := workflow.PrepareWorkflows(r.config.WorkflowPath, r.config.WorkflowFile)
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
	result, err := preflight.RunPreflightChecks(ctx, r.config.WorkflowPath, r.config.RepoRoot, r.config.RunID, r.config.WorkflowFile)
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
	if err := git.ValidateWorktreeInitialized(r.worktreeInfo); err != nil {
		return err
	}

	r.startTime = time.Now()

	// Configure act execution (matching runWithoutTUI logic from cmd/check.go:476-490)
	actConfig := r.buildActConfig(nil)

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
	if err := git.ValidateWorktreeInitialized(r.worktreeInfo); err != nil {
		return false, err
	}

	// NOTE: Keep context wrapper - needed for proper cancellation
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	r.startTime = time.Now()

	actConfig := r.buildActConfig(logChan)

	resultChan := make(chan tuiResult, 1)

	var wg sync.WaitGroup
	wg.Add(2)

	r.startActRunnerGoroutine(ctx, actConfig, logChan, program, resultChan, &wg)
	r.startLogProcessorGoroutine(ctx, logChan, program, &wg)

	finalModel, err := program.Run()
	if err != nil {
		cancel()
		wg.Wait()
		return false, err
	}

	checkModel, ok := finalModel.(*tui.CheckModel)
	var wasCancelled bool
	if ok {
		wasCancelled = checkModel.Cancelled
	}

	tuiRes := <-resultChan
	if tuiRes.err != nil {
		cancel()
		wg.Wait()
		return false, tuiRes.err
	}

	r.result = &RunResult{
		ActResult:    tuiRes.result,
		Extracted:    tuiRes.extracted,
		Grouped:      tuiRes.grouped,
		WorktreeInfo: r.worktreeInfo,
		RunID:        r.config.RunID,
		StartTime:    r.startTime,
		Duration:     tuiRes.result.Duration,
		Cancelled:    wasCancelled,
		ExitCode:     tuiRes.result.ExitCode,
	}

	defer cancel()
	wg.Wait()

	return wasCancelled, nil
}

// startActRunnerGoroutine starts a goroutine to run act and process results.
func (r *CheckRunner) startActRunnerGoroutine(
	ctx context.Context,
	actConfig *act.RunConfig,
	logChan chan string,
	program *tea.Program,
	resultChan chan tuiResult,
	wg *sync.WaitGroup,
) {
	go func() {
		defer wg.Done()
		defer close(logChan)
		defer func() {
			if rec := recover(); rec != nil {
				err := fmt.Errorf("act.Run panicked: %v", rec)
				resultChan <- tuiResult{err: err}
				sendToTUI(program, tui.ErrMsg(err))
			}
		}()

		result, err := act.Run(ctx, actConfig)

		if err != nil {
			resultChan <- tuiResult{err: err}
			sendToTUI(program, tui.ErrMsg(err))
			return
		}

		extracted, grouped := r.extractAndProcessErrors(result)
		cancelled := errors.Is(ctx.Err(), context.Canceled)

		program.Send(tui.DoneMsg{
			Duration:  result.Duration,
			ExitCode:  result.ExitCode,
			Errors:    grouped,
			Cancelled: cancelled,
		})

		resultChan <- tuiResult{
			result:    result,
			extracted: extracted,
			grouped:   grouped,
		}
	}()
}

// startLogProcessorGoroutine starts a goroutine to process log messages.
func (r *CheckRunner) startLogProcessorGoroutine(
	ctx context.Context,
	logChan chan string,
	program *tea.Program,
	wg *sync.WaitGroup,
) {
	go func() {
		defer wg.Done()
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
					sendToTUI(program, tui.LogMsg(line))
					if progress := tui.ParseActProgress(line); progress != nil {
						sendToTUI(program, *progress)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
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
	)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence storage at %s/.detent: %w", r.config.RepoRoot, err)
	}

	// Record all findings in a single batch operation
	if err := recorder.RecordFindings(r.result.Extracted); err != nil {
		return fmt.Errorf("failed to record findings: %w", err)
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
	if r.cleanupWorktree != nil {
		r.cleanupWorktree()
	}
	if r.cleanupWorkflows != nil {
		r.cleanupWorkflows()
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

// buildActConfig constructs an act.RunConfig with appropriate settings.
// When logChan is nil, StreamOutput is enabled (for non-TUI mode).
// When logChan is provided, output is streamed to the channel (for TUI mode).
func (r *CheckRunner) buildActConfig(logChan chan string) *act.RunConfig {
	git.RequireWorktreeInitialized(r.worktreeInfo)

	return &act.RunConfig{
		WorkflowPath: r.tmpDir,
		Event:        r.config.Event,
		Verbose:      false,
		WorkDir:      r.worktreeInfo.Path,
		StreamOutput: logChan == nil && r.config.StreamOutput,
		LogChan:      logChan,
	}
}
