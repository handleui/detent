// Package preflight provides orchestration logic for pre-flight checks
// before running GitHub Actions workflows locally. This includes verifying
// act installation, Docker availability, preparing workflows, and creating
// isolated worktrees for execution.
package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/detent/cli/internal/commands"
	"github.com/detent/cli/internal/docker"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/workflow"
)

// ErrCancelled is returned when the user cancels an operation
var ErrCancelled = errors.New("cancelled")

// cancelledMessage is the friendly goodbye shown when user cancels
var cancelledMessage = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("Action cancelled. Maybe next time?")

const (
	// preflightVisualDelay allows the spinner to render before check execution begins.
	// This improves perceived responsiveness in the TUI by giving users visual feedback
	// that the tool is working before any actual work starts.
	preflightVisualDelay = 200 * time.Millisecond

	// preflightTransitionDelay creates a brief pause between checks to prevent
	// visual "flashing" and give users time to process each check result before
	// moving to the next one. This improves the UX of the sequential check display.
	preflightTransitionDelay = 100 * time.Millisecond

	// preflightCompletionPause provides a final pause after all checks pass to
	// display the success state and give visual confirmation before proceeding
	// to the main workflow execution.
	preflightCompletionPause = 300 * time.Millisecond
)

// Result contains the results and cleanup functions from preflight checks.
type Result struct {
	TmpDir           string
	WorktreeInfo     *git.WorktreeInfo
	CleanupWorkflows func()
	CleanupWorktree  func()
	StashInfo        *git.StashInfo // Tracks if changes were stashed during preflight
}

// workflowPrepResult holds the results from workflow preparation.
type workflowPrepResult struct {
	tmpDir  string
	cleanup func()
}

// worktreePrepResult holds the results from worktree preparation.
type worktreePrepResult struct {
	info    *git.WorktreeInfo
	cleanup func()
}

// Cleanup executes both cleanup functions in the correct order (workflows first, then worktree).
// Order matters: workflow temp files should be removed before the git worktree
// to ensure consistent state during cleanup.
// If changes were stashed during preflight, they are restored here.
func (r *Result) Cleanup() {
	if r.CleanupWorkflows != nil {
		r.CleanupWorkflows()
	}
	if r.CleanupWorktree != nil {
		r.CleanupWorktree()
	}

	// Restore stashed changes if we stashed them during preflight
	git.RestoreStashIfNeeded(".", r.StashInfo)
}

// preflightChecker helps execute individual preflight checks with consistent error handling.
type preflightChecker struct {
	display         *tui.PreflightDisplay
	transitionDelay time.Duration
}

// executeCheck runs a single check with standardized error handling and UI updates.
// It updates the display to show "running", executes the check function, and updates
// the display to show "success" or "error" depending on the result.
// For cancellation errors, it shows a friendly goodbye message instead of error details.
func (p *preflightChecker) executeCheck(checkName string, checkFunc func() error) error {
	time.Sleep(p.transitionDelay)
	p.display.UpdateCheck(checkName, "running", nil)
	p.display.Render()

	err := checkFunc()
	if err != nil {
		if errors.Is(err, ErrCancelled) {
			p.display.Clear()
			fmt.Fprintln(os.Stderr, cancelledMessage)
			fmt.Fprintln(os.Stderr)
			return err
		}
		p.display.UpdateCheck(checkName, "error", err)
		p.display.RenderFinal()
		return err
	}

	p.display.UpdateCheck(checkName, "success", nil)
	p.display.Render()
	return nil
}

// executeWorkflowPrep runs the workflow preparation check with standardized error handling.
func (p *preflightChecker) executeWorkflowPrep(checkName string, additionalSuccessChecks []string, checkFunc func() (workflowPrepResult, error)) (workflowPrepResult, error) {
	time.Sleep(p.transitionDelay)
	p.display.UpdateCheck(checkName, "running", nil)
	p.display.Render()

	result, err := checkFunc()
	if err != nil {
		p.display.UpdateCheck(checkName, "error", err)
		p.display.RenderFinal()
		return workflowPrepResult{}, err
	}

	p.display.UpdateCheck(checkName, "success", nil)
	for _, check := range additionalSuccessChecks {
		p.display.UpdateCheck(check, "success", nil)
	}
	p.display.Render()
	return result, nil
}

// executeWorktreePrep runs the worktree preparation check with standardized error handling.
func (p *preflightChecker) executeWorktreePrep(checkName string, checkFunc func() (worktreePrepResult, error)) (worktreePrepResult, error) {
	time.Sleep(p.transitionDelay)
	p.display.UpdateCheck(checkName, "running", nil)
	p.display.Render()

	result, err := checkFunc()
	if err != nil {
		p.display.UpdateCheck(checkName, "error", err)
		p.display.RenderFinal()
		return worktreePrepResult{}, err
	}

	p.display.UpdateCheck(checkName, "success", nil)
	p.display.Render()
	return result, nil
}

// EnsureCleanWorktree validates that the worktree is clean, or prompts the user
// to clean it interactively. Returns StashInfo if changes were stashed, nil otherwise.
//
// The display parameter allows hiding the preflight checks while the prompt is shown.
//
// The function:
// 1. Gets dirty files list (which also checks if worktree is clean)
// 2. If clean, returns nil (no action needed)
// 3. If dirty, clears the check display and launches interactive Bubble Tea prompt with options:
//   - Commit: stages all changes and commits with user-provided message
//   - Stash: stashes changes (with warning) to be restored during cleanup
//   - Cancel: returns error to abort the run
//
// This ensures users understand WHY a clean worktree is required and provides
// convenient ways to achieve it.
func EnsureCleanWorktree(ctx context.Context, repoRoot string, display *tui.PreflightDisplay) (*git.StashInfo, error) {
	// Get dirty files list (this also checks if worktree is clean)
	files, err := git.GetDirtyFilesList(ctx, repoRoot)
	if err != nil {
		return nil, err
	}

	// If no dirty files, worktree is clean
	if len(files) == 0 {
		return nil, nil // Already clean, no action needed
	}

	// Clear the preflight display to show a clean prompt
	if display != nil {
		display.Clear()
	}

	// Launch interactive prompt
	model := tui.NewCleanWorktreePromptModel(files)
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("interactive prompt failed: %w", err)
	}

	// Extract result from final model
	promptModel, ok := finalModel.(*tui.CleanWorktreePromptModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}

	result := promptModel.GetResult()
	if result == nil || result.Cancelled {
		return nil, ErrCancelled
	}

	// Execute user's choice
	switch result.Action {
	case tui.ActionCommit:
		if err := git.CommitAllChanges(ctx, repoRoot, result.CommitMessage); err != nil {
			return nil, fmt.Errorf("failed to commit changes: %w", err)
		}
		return nil, nil // Committed, no stash needed

	case tui.ActionStash:
		stashInfo, err := git.StashChanges(ctx, repoRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to stash changes: %w", err)
		}
		return stashInfo, nil // Return stash info for later restoration

	case tui.ActionCancel:
		return nil, ErrCancelled

	default:
		return nil, fmt.Errorf("unknown action: %v", result.Action)
	}
}

// RunPreflightChecks performs and displays pre-flight checks before running act.
// It verifies system requirements, prepares workflows, and creates an isolated worktree.
func RunPreflightChecks(ctx context.Context, workflowPath, repoRoot, runID, workflowFile string) (*Result, error) {
	checks := []string{
		"Checking for uncommitted changes",
		"Validating repository security",
		"Checking act installation",
		"Checking Docker availability",
		"Preparing workflows (injecting continue-on-error)",
		"Creating temporary workspace",
		"Creating isolated worktree",
	}

	display := tui.NewPreflightDisplay(checks)
	display.Render()

	checker := &preflightChecker{
		display:         display,
		transitionDelay: preflightTransitionDelay,
	}

	// Check 1: Clean worktree (uses visual delay for first check)
	// Uses interactive prompt if uncommitted changes are detected
	checker.transitionDelay = preflightVisualDelay
	var stashInfo *git.StashInfo
	if err := checker.executeCheck("Checking for uncommitted changes", func() error {
		var err error
		stashInfo, err = EnsureCleanWorktree(ctx, repoRoot, display)
		return err
	}); err != nil {
		return nil, err
	}
	checker.transitionDelay = preflightTransitionDelay

	// Check 2: Repository security (symlinks and submodules)
	if err := checker.executeCheck("Validating repository security", func() error {
		// Check for symlinks that escape repository boundaries
		if err := git.ValidateNoEscapingSymlinks(ctx, repoRoot); err != nil {
			return fmt.Errorf("symlink security: %w", err)
		}
		// Check for submodules (not yet supported, includes CVE-2025-48384 check)
		if err := git.ValidateNoSubmodules(repoRoot); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Check 3: Act installation
	if err := checker.executeCheck("Checking act installation", commands.CheckAct); err != nil {
		return nil, err
	}

	// Check 4: Docker availability
	if err := checker.executeCheck("Checking Docker availability", func() error {
		return docker.IsAvailable(ctx)
	}); err != nil {
		return nil, fmt.Errorf("docker is not available: %w", err)
	}

	// Check 5: Prepare workflows
	workflowResult, err := checker.executeWorkflowPrep(
		"Preparing workflows (injecting continue-on-error)",
		[]string{"Creating temporary workspace"},
		func() (workflowPrepResult, error) {
			tmpDir, cleanup, prepErr := workflow.PrepareWorkflows(workflowPath, workflowFile)
			if prepErr != nil {
				return workflowPrepResult{}, fmt.Errorf("preparing workflows: %w", prepErr)
			}
			return workflowPrepResult{tmpDir: tmpDir, cleanup: cleanup}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	// Check 6: Create worktree
	worktreeResult, err := checker.executeWorktreePrep(
		"Creating isolated worktree",
		func() (worktreePrepResult, error) {
			info, cleanup, wtErr := git.PrepareWorktree(ctx, repoRoot, runID)
			if wtErr != nil {
				workflowResult.cleanup()
				return worktreePrepResult{}, fmt.Errorf("creating worktree: %w", wtErr)
			}
			return worktreePrepResult{info: info, cleanup: cleanup}, nil
		},
	)
	if err != nil {
		return nil, err
	}

	// Small pause to show all checks passed
	time.Sleep(preflightCompletionPause)

	// Add a blank line before TUI starts
	fmt.Fprintln(os.Stderr)

	return &Result{
		TmpDir:           workflowResult.tmpDir,
		WorktreeInfo:     worktreeResult.info,
		CleanupWorkflows: workflowResult.cleanup,
		CleanupWorktree:  worktreeResult.cleanup,
		StashInfo:        stashInfo,
	}, nil
}
