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
	StashInfo        *git.StashInfo
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
	repoRoot := r.RepoRoot
	if repoRoot == "" {
		repoRoot = "."
	}
	git.RestoreStashIfNeeded(repoRoot, r.StashInfo)
}

// EnsureCleanWorktree validates that the worktree is clean, or prompts the user
// to clean it interactively. Returns StashInfo if changes were stashed.
func EnsureCleanWorktree(ctx context.Context, repoRoot string, program *tea.Program) (*git.StashInfo, error) {
	files, err := git.GetDirtyFilesList(ctx, repoRoot)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, nil
	}

	// Quit preflight TUI to show prompt
	if program != nil {
		program.Send(tui.PreflightDoneMsg{Err: nil})
		program.Wait()
	}

	// Launch interactive prompt
	model := tui.NewCleanWorktreePromptModel(files)
	promptProgram := tea.NewProgram(model)
	finalModel, err := promptProgram.Run()
	if err != nil {
		return nil, fmt.Errorf("interactive prompt failed: %w", err)
	}

	promptModel, ok := finalModel.(*tui.CleanWorktreePromptModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}

	result := promptModel.GetResult()
	if result == nil || result.Cancelled {
		return nil, ErrCancelled
	}

	switch result.Action {
	case tui.ActionCommit:
		if err := git.CommitAllChanges(ctx, repoRoot, result.CommitMessage); err != nil {
			return nil, fmt.Errorf("failed to commit changes: %w", err)
		}
		return nil, nil

	case tui.ActionStash:
		stashInfo, err := git.StashChanges(ctx, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to stash changes: %w", err)
		}
		return stashInfo, nil

	case tui.ActionCancel:
		return nil, ErrCancelled

	default:
		return nil, fmt.Errorf("unknown action: %v", result.Action)
	}
}

// RunPreflightChecks performs pre-flight checks with a single-line shimmer display.
func RunPreflightChecks(ctx context.Context, workflowPath, repoRoot, runID, workflowFile string) (*Result, error) {
	// Create single-line shimmer display
	model := tui.NewPreflightModel()
	program := tea.NewProgram(&model)

	// Channel to collect results
	type checkResult struct {
		result *Result
		err    error
	}
	resultChan := make(chan checkResult, 1)

	// Run checks in background goroutine
	go func() {
		var stashInfo *git.StashInfo
		var tmpDir string
		var cleanupWorkflows func()
		var worktreeInfo *git.WorktreeInfo
		var cleanupWorktree func()

		sendError := func(err error) {
			resultChan <- checkResult{err: err}
			program.Send(tui.PreflightDoneMsg{Err: err})
		}

		// Check 1: Validate repository
		program.Send(tui.PreflightUpdateMsg("Validating repository"))
		var err error
		stashInfo, err = EnsureCleanWorktree(ctx, repoRoot, program)
		if err != nil {
			sendError(err)
			return
		}
		err = git.ValidateNoEscapingSymlinks(ctx, repoRoot)
		if err != nil {
			sendError(fmt.Errorf("symlink security: %w", err))
			return
		}
		err = git.ValidateNoSubmodules(repoRoot)
		if err != nil {
			sendError(err)
			return
		}

		// Check 2: Prerequisites
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

		// Check 3: Prepare workflows
		program.Send(tui.PreflightUpdateMsg("Preparing workflows"))
		tmpDir, cleanupWorkflows, err = workflow.PrepareWorkflows(workflowPath, workflowFile)
		if err != nil {
			sendError(fmt.Errorf("preparing workflows: %w", err))
			return
		}

		// Check 4: Create workspace
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

		// Success
		resultChan <- checkResult{
			result: &Result{
				TmpDir:           tmpDir,
				WorktreeInfo:     worktreeInfo,
				CleanupWorkflows: cleanupWorkflows,
				CleanupWorktree:  cleanupWorktree,
				StashInfo:        stashInfo,
				RepoRoot:         repoRoot,
			},
		}
		program.Send(tui.PreflightDoneMsg{Err: nil})
	}()

	// Run TUI
	finalModel, err := program.Run()
	if err != nil {
		return nil, err
	}

	// Check if cancelled
	if m, ok := finalModel.(*tui.PreflightModel); ok && m.WasCancelled() {
		return nil, ErrCancelled
	}

	// Get result from goroutine
	res := <-resultChan
	return res.result, res.err
}
