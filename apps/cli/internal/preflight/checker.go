// Package preflight provides orchestration logic for pre-flight checks
// before running GitHub Actions workflows locally. This includes verifying
// act installation, Docker availability, preparing workflows, and creating
// isolated worktrees for execution.
package preflight

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/detent/cli/internal/commands"
	"github.com/detent/cli/internal/docker"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/workflow"
)

const (
	preflightVisualDelay     = 200 * time.Millisecond
	preflightTransitionDelay = 100 * time.Millisecond
	preflightCompletionPause = 300 * time.Millisecond
)

// RunPreflightChecks performs and displays pre-flight checks before running act.
// It verifies system requirements, prepares workflows, and creates an isolated worktree.
func RunPreflightChecks(ctx context.Context, workflowPath, repoRoot, runID, workflowFile string) (tmpDir string, worktreeInfo *git.WorktreeInfo, cleanupWorkflows, cleanupWorktree func(), err error) {
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
