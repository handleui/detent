package runner

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/debug"
	"github.com/detentsh/core/act"
	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/git"
	"github.com/detentsh/core/workflow"
)

// CheckRunner orchestrates a complete check run lifecycle including:
// - Workflow preparation (injection of continue-on-error and timeouts)
// - Worktree creation and isolation
// - Act execution with proper output capture
// - Error extraction and grouping
// - Result persistence
// - Resource cleanup
//
// CheckRunner is a thin orchestrator that delegates to focused components:
// - WorkflowPreparer: handles workflow file injection and worktree setup
// - ActExecutor: handles running the act process and capturing output
// - ErrorProcessor: handles error extraction from output
// - ResultPersister: handles database persistence
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
	tmpDir           string               // Temporary directory for workflow files
	worktreeInfo     *git.WorktreeInfo    // Worktree metadata including path and commit info
	cleanupWorkflows func()               // Cleanup function for workflow temp directory
	cleanupWorktree  func()               // Cleanup function for worktree
	jobs             []workflow.JobInfo   // Job information extracted from workflows

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
// - Parallel preflight validation (git repository, act availability, docker availability)
// - Parallel preparation (workflow files and worktree creation)
//
// This must be called before Run. All resources are tracked for cleanup.
// Returns error if preparation fails. On error, partial resources are cleaned up.
//
// When verbose is true (non-TUI mode), prints status lines to stderr showing progress.
func (r *CheckRunner) Prepare(ctx context.Context) error {
	preparer := NewWorkflowPreparer(r.config)
	verbose := !r.config.UseTUI

	result, err := preparer.Prepare(ctx, verbose)
	if err != nil {
		return err
	}

	r.storePreparationResult(result)
	return nil
}

// PrepareWithTUI is like Prepare but sends progress updates to the TUI.
// It performs the same preparation steps but updates the UI with status messages.
//
// This must be called before RunWithTUI. All resources are tracked for cleanup.
// Returns error if preparation fails. On error, partial resources are cleaned up.
func (r *CheckRunner) PrepareWithTUI(ctx context.Context) error {
	preparer := NewWorkflowPreparer(r.config)

	result, err := preparer.PrepareWithTUI(ctx)
	if err != nil {
		return err
	}

	r.storePreparationResult(result)
	return nil
}

func (r *CheckRunner) storePreparationResult(result *PrepareResult) {
	r.tmpDir = result.TmpDir
	r.worktreeInfo = result.WorktreeInfo
	r.cleanupWorkflows = result.CleanupWorkflows
	r.cleanupWorktree = result.CleanupWorktree
	r.jobs = result.Jobs
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
	executor := NewActExecutor(r.config, r.tmpDir, r.worktreeInfo)

	execResult, err := executor.Execute(ctx)
	if err != nil {
		return err
	}

	processor := NewErrorProcessor(r.config.RepoRoot)
	processed := processor.Process(execResult.ActResult)

	r.startTime = execResult.StartTime
	r.result = &RunResult{
		ActResult:            execResult.ActResult,
		Extracted:            processed.Extracted,
		Grouped:              processed.Grouped,
		GroupedComprehensive: processed.GroupedComprehensive,
		WorktreeInfo:         r.worktreeInfo,
		RunID:                r.config.RunID,
		StartTime:            execResult.StartTime,
		Duration:             execResult.Duration,
		ExitCode:             execResult.ActResult.ExitCode,
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
	executor := NewActExecutor(r.config, r.tmpDir, r.worktreeInfo)

	execResult, wasCancelled, err := executor.ExecuteWithTUI(ctx, logChan, program)
	if err != nil {
		return false, err
	}

	r.startTime = execResult.StartTime
	r.result = &RunResult{
		ActResult:            execResult.ActResult,
		Extracted:            execResult.Extracted,
		Grouped:              execResult.Grouped,
		GroupedComprehensive: execResult.GroupedComprehensive,
		WorktreeInfo:         r.worktreeInfo,
		RunID:                r.config.RunID,
		StartTime:            execResult.StartTime,
		Duration:             execResult.Duration,
		Cancelled:            wasCancelled,
		ExitCode:             execResult.ActResult.ExitCode,
	}

	if execResult.CompletionOutput != "" {
		fmt.Fprint(os.Stderr, execResult.CompletionOutput)
	}

	return wasCancelled, nil
}

// Persist saves the check run results to the database.
// This writes the complete run result including:
// - Run metadata (ID, timing, exit code)
// - Worktree information (commit SHA)
// - Extracted errors with full context
//
// Returns error if persistence fails. This should be called after Run/RunWithTUI
// completes successfully.
func (r *CheckRunner) Persist() error {
	if r.result == nil {
		return fmt.Errorf("no result to persist (Run/RunWithTUI must be called first)")
	}

	persister := NewResultPersister(r.config.RepoRoot, r.config.WorkflowPath, r.worktreeInfo)
	return persister.Persist(r.result.Extracted, r.result.ExitCode)
}

// Cleanup releases all resources allocated during Prepare.
// Order matters: workflow temp files should be removed before the git worktree
// to ensure consistent state during cleanup.
func (r *CheckRunner) Cleanup() {
	debug.Close()

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

// GetJobs returns the job information extracted from workflow files.
// This should be called after Prepare/PrepareWithTUI completes.
func (r *CheckRunner) GetJobs() []workflow.JobInfo {
	return r.jobs
}

// buildActConfig constructs an act.RunConfig with appropriate settings.
// When logChan is nil, StreamOutput is enabled (for non-TUI mode).
// When logChan is provided, output is streamed to the channel (for TUI mode).
//
// This method is kept for backward compatibility with tests.
// New code should use ActExecutor.buildActConfig instead.
func (r *CheckRunner) buildActConfig(logChan chan string) *act.RunConfig {
	executor := NewActExecutor(r.config, r.tmpDir, r.worktreeInfo)
	return executor.buildActConfig(logChan)
}

// extractAndProcessErrors extracts errors from act output, applies severity, and groups.
// Returns extracted errors and both simple (by-file) and comprehensive (by-category) groupings.
//
// This method is kept for backward compatibility with tests.
// New code should use ErrorProcessor.Process instead.
func (r *CheckRunner) extractAndProcessErrors(actResult *act.RunResult) ([]*errors.ExtractedError, *errors.GroupedErrors, *errors.ComprehensiveErrorGroup) {
	processor := NewErrorProcessor(r.config.RepoRoot)
	processed := processor.Process(actResult)
	return processed.Extracted, processed.Grouped, processed.GroupedComprehensive
}
