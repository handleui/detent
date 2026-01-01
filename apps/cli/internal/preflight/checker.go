// Package preflight provides orchestration logic for pre-flight checks
// before running GitHub Actions workflows locally.
package preflight

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/actbin"
	"github.com/detent/cli/internal/docker"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/workflow"
)

// ErrCancelled is returned when the user cancels an operation
var ErrCancelled = errors.New("cancelled")

// Result contains the results and cleanup functions from preflight checks.
type Result struct {
	TmpDir           string
	WorktreeInfo     *git.WorktreeInfo
	CleanupWorkflows func()
	CleanupWorktree  func()
	RepoRoot         string
}

// Cleanup executes both cleanup functions in the correct order.
func (r *Result) Cleanup() {
	if r.CleanupWorkflows != nil {
		r.CleanupWorkflows()
	}
	if r.CleanupWorktree != nil {
		r.CleanupWorktree()
	}
}

// RunPreflightChecks performs pre-flight checks with a single-line shimmer display.
func RunPreflightChecks(ctx context.Context, workflowPath, repoRoot, runID, workflowFile string) (*Result, error) {
	model := tui.NewPreflightModel()
	program := tea.NewProgram(&model)

	type checkResult struct {
		result *Result
		err    error
	}
	resultChan := make(chan checkResult, 1)

	go func() {
		var tmpDir string
		var cleanupWorkflows func()
		var worktreeInfo *git.WorktreeInfo
		var cleanupWorktree func()

		sendError := func(err error) {
			resultChan <- checkResult{err: err}
			program.Send(tui.PreflightDoneMsg{Err: err})
		}

		// Load config and get allowed sensitive jobs for this repo
		var allowedSensitiveJobs []string
		if cfg, cfgErr := persistence.Load(); cfgErr == nil {
			if repoSHA, shaErr := git.GetFirstCommitSHA(repoRoot); shaErr == nil && repoSHA != "" {
				allowedSensitiveJobs = cfg.GetAllowedSensitiveJobs(repoSHA)
			}
		}

		program.Send(tui.PreflightUpdateMsg("Validating repository"))
		err := git.ValidateNoEscapingSymlinks(ctx, repoRoot)
		if err != nil {
			sendError(fmt.Errorf("symlink security: %w", err))
			return
		}
		err = git.ValidateNoSubmodules(repoRoot)
		if err != nil {
			sendError(err)
			return
		}

		program.Send(tui.PreflightUpdateMsg("Checking prerequisites"))
		err = actbin.EnsureInstalled(ctx, nil)
		if err != nil {
			sendError(err)
			return
		}
		err = docker.IsAvailable(ctx)
		if err != nil {
			sendError(fmt.Errorf("docker not available: %w", err))
			return
		}

		program.Send(tui.PreflightUpdateMsg("Preparing workflows"))
		tmpDir, cleanupWorkflows, err = workflow.PrepareWorkflows(workflowPath, workflowFile, allowedSensitiveJobs)
		if err != nil {
			sendError(fmt.Errorf("preparing workflows: %w", err))
			return
		}

		program.Send(tui.PreflightUpdateMsg("Creating workspace"))
		var worktreePath string
		worktreePath, err = git.CreateEphemeralWorktreePath(runID)
		if err != nil {
			cleanupWorkflows()
			sendError(fmt.Errorf("worktree path: %w", err))
			return
		}
		worktreeInfo, cleanupWorktree, err = git.PrepareWorktree(ctx, repoRoot, worktreePath)
		if err != nil {
			cleanupWorkflows()
			sendError(fmt.Errorf("creating worktree: %w", err))
			return
		}

		resultChan <- checkResult{
			result: &Result{
				TmpDir:           tmpDir,
				WorktreeInfo:     worktreeInfo,
				CleanupWorkflows: cleanupWorkflows,
				CleanupWorktree:  cleanupWorktree,
				RepoRoot:         repoRoot,
			},
		}
		program.Send(tui.PreflightDoneMsg{Err: nil})
	}()

	finalModel, err := program.Run()
	if err != nil {
		return nil, err
	}

	if m, ok := finalModel.(*tui.PreflightModel); ok && m.WasCancelled() {
		return nil, ErrCancelled
	}

	res := <-resultChan
	return res.result, res.err
}
