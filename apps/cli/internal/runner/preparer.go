package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/detent/cli/internal/debug"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/preflight"
	"github.com/detent/cli/internal/tui"
	"github.com/detentsh/core/git"
	"github.com/detentsh/core/workflow"
	"golang.org/x/sync/errgroup"
)

// PrepareResult contains the results of workflow and worktree preparation.
type PrepareResult struct {
	TmpDir           string
	WorktreeInfo     *git.WorktreeInfo
	CleanupWorkflows func()
	CleanupWorktree  func()
	Jobs             []workflow.JobInfo
}

// WorkflowPreparer handles workflow file injection and worktree setup.
// It performs preflight validation, workflow preparation, and worktree creation
// in an optimized parallel execution pattern.
type WorkflowPreparer struct {
	config *RunConfig
}

// NewWorkflowPreparer creates a new WorkflowPreparer with the given configuration.
func NewWorkflowPreparer(config *RunConfig) *WorkflowPreparer {
	return &WorkflowPreparer{config: config}
}

// Prepare sets up the execution environment including:
// - Parallel preflight validation (git repository, act availability, docker availability)
// - Validation that worktree is clean (no uncommitted changes)
// - Parallel preparation (workflow files and worktree creation)
//
// When verbose is true (non-TUI mode), prints status lines to stderr showing progress.
// Returns PrepareResult on success or error on failure.
func (p *WorkflowPreparer) Prepare(ctx context.Context, verbose bool) (*PrepareResult, error) {
	if err := p.runPreflightChecks(ctx, verbose); err != nil {
		return nil, err
	}

	if err := p.runValidationChecks(ctx, verbose); err != nil {
		return nil, err
	}

	return p.prepareWorkflowsAndWorktree(ctx, verbose)
}

// PrepareWithTUI is like Prepare but uses the preflight TUI for visual feedback.
// It delegates to the preflight package for interactive preparation.
func (p *WorkflowPreparer) PrepareWithTUI(ctx context.Context) (*PrepareResult, error) {
	result, err := preflight.RunPreflightChecks(ctx, p.config.WorkflowPath, p.config.RepoRoot, p.config.RunID, p.config.WorkflowFile)
	if err != nil {
		return nil, err
	}

	jobs, _ := workflow.ExtractJobInfoFromDir(p.config.WorkflowPath)
	p.initDebugLogging(jobs)

	return &PrepareResult{
		TmpDir:           result.TmpDir,
		WorktreeInfo:     result.WorktreeInfo,
		CleanupWorkflows: result.CleanupWorkflows,
		CleanupWorktree:  result.CleanupWorktree,
		Jobs:             jobs,
	}, nil
}

// runPreflightChecks runs parallel validation for git, act, and docker availability.
func (p *WorkflowPreparer) runPreflightChecks(ctx context.Context, verbose bool) error {
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return git.ValidateGitRepository(gctx, p.config.RepoRoot)
	})

	g.Go(func() error {
		return p.ensureActInstalled(gctx)
	})

	g.Go(func() error {
		return p.checkDockerAvailable(gctx)
	})

	if err := g.Wait(); err != nil {
		if verbose {
			p.printStatus("Checking prerequisites", false)
		}
		return err
	}
	if verbose {
		p.printStatus("Checking prerequisites", true)
	}

	// Best-effort cleanup of orphaned worktrees from previous runs (SIGKILL recovery)
	if _, err := git.CleanupOrphanedWorktrees(ctx, p.config.RepoRoot); err != nil {
		debug.Log("Failed to cleanup orphaned worktrees: %v", err)
	}

	return nil
}

// runValidationChecks runs parallel validation for repository state.
func (p *WorkflowPreparer) runValidationChecks(ctx context.Context, verbose bool) error {
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return git.ValidateNoSubmodules(p.config.RepoRoot)
	})

	g.Go(func() error {
		return git.ValidateNoEscapingSymlinks(gctx, p.config.RepoRoot)
	})

	if err := g.Wait(); err != nil {
		if verbose {
			p.printStatus("Validating repository", false)
		}
		return err
	}
	if verbose {
		p.printStatus("Validating repository", true)
	}

	return nil
}

// prepareWorkflowsAndWorktree prepares workflow files and creates worktree in parallel.
func (p *WorkflowPreparer) prepareWorkflowsAndWorktree(ctx context.Context, verbose bool) (*PrepareResult, error) {
	// Load config and get job overrides for this repo (before goroutines)
	var jobOverrides map[string]string
	if cfg, cfgErr := persistence.Load(); cfgErr == nil {
		if repoSHA, shaErr := git.GetFirstCommitSHA(p.config.RepoRoot); shaErr == nil && repoSHA != "" {
			jobOverrides = cfg.GetJobOverrides(repoSHA)
		}
	}

	type workflowResult struct {
		tmpDir           string
		cleanupWorkflows func()
		err              error
	}

	type worktreeResult struct {
		worktreeInfo    *git.WorktreeInfo
		cleanupWorktree func()
		err             error
	}

	workflowChan := make(chan workflowResult, 1)
	worktreeChan := make(chan worktreeResult, 1)

	go func() {
		tmpDir, cleanupWorkflows, err := workflow.PrepareWorkflows(p.config.WorkflowPath, p.config.WorkflowFile, jobOverrides)
		workflowChan <- workflowResult{
			tmpDir:           tmpDir,
			cleanupWorkflows: cleanupWorkflows,
			err:              err,
		}
	}()

	go func() {
		worktreePath, pathErr := git.CreateEphemeralWorktreePath(p.config.RunID)
		if pathErr != nil {
			worktreeChan <- worktreeResult{err: pathErr}
			return
		}
		worktreeInfo, cleanup, err := git.PrepareWorktree(ctx, p.config.RepoRoot, worktreePath)
		worktreeChan <- worktreeResult{
			worktreeInfo:    worktreeInfo,
			cleanupWorktree: cleanup,
			err:             err,
		}
	}()

	workflowRes := <-workflowChan
	worktreeRes := <-worktreeChan

	if workflowRes.err != nil {
		if worktreeRes.cleanupWorktree != nil {
			worktreeRes.cleanupWorktree()
		}
		if verbose {
			p.printStatus("Preparing workflows", false)
		}
		return nil, fmt.Errorf("preparing workflows: %w", workflowRes.err)
	}
	if verbose {
		p.printStatus("Preparing workflows", true)
	}

	if worktreeRes.err != nil {
		if workflowRes.cleanupWorkflows != nil {
			workflowRes.cleanupWorkflows()
		}
		if verbose {
			p.printStatus("Creating workspace", false)
		}
		return nil, fmt.Errorf("creating worktree: %w", worktreeRes.err)
	}
	if verbose {
		p.printStatus("Creating workspace", true)
	}

	jobs, _ := workflow.ExtractJobInfoFromDir(p.config.WorkflowPath)
	p.initDebugLogging(jobs)

	return &PrepareResult{
		TmpDir:           workflowRes.tmpDir,
		WorktreeInfo:     worktreeRes.worktreeInfo,
		CleanupWorkflows: workflowRes.cleanupWorkflows,
		CleanupWorktree:  worktreeRes.cleanupWorktree,
		Jobs:             jobs,
	}, nil
}

// initDebugLogging initializes debug logging and logs job information.
func (p *WorkflowPreparer) initDebugLogging(jobs []workflow.JobInfo) {
	_ = debug.Init(p.config.RepoRoot)
	for _, job := range jobs {
		debug.Log("Registered: ID=%q Name=%q", job.ID, job.Name)
	}
}

// printStatus prints a status line to stderr for non-TUI mode.
func (p *WorkflowPreparer) printStatus(label string, success bool) {
	_, _ = fmt.Fprintf(os.Stderr, "  %s... %s\n", label, tui.StatusIcon(success))
}

// ensureActInstalled checks that act is installed and available.
func (p *WorkflowPreparer) ensureActInstalled(ctx context.Context) error {
	// Import dynamically to avoid circular dependency
	return ensureActInstalled(ctx)
}

// checkDockerAvailable checks that Docker is running and available.
func (p *WorkflowPreparer) checkDockerAvailable(ctx context.Context) error {
	return checkDockerAvailable(ctx)
}
