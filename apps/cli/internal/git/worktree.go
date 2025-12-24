package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeInfo contains metadata about the created worktree
type WorktreeInfo struct {
	Path      string // Absolute path to worktree directory
	CommitSHA string // Commit SHA that worktree is based on
}

// PrepareWorktree creates a temporary git worktree for isolated workflow execution.
// The worktree is created from the current HEAD commit.
// Returns worktree info, cleanup function, and error.
//
// Note: This requires a clean worktree. Call ValidateCleanWorktree() before this.
func PrepareWorktree(ctx context.Context, repoRoot, runID string) (*WorktreeInfo, func(), error) {
	// 1. Get current commit SHA from repoRoot
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("getting current commit SHA: %w", err)
	}
	commitSHA := strings.TrimSpace(string(output))

	// 2. Create worktree path
	worktreePath := filepath.Join(os.TempDir(), fmt.Sprintf("detent-%s", runID))

	// 3. Create worktree in detached HEAD state
	if err := createWorktree(ctx, repoRoot, worktreePath, commitSHA); err != nil {
		return nil, nil, fmt.Errorf("creating worktree: %w", err)
	}

	info := &WorktreeInfo{
		Path:      worktreePath,
		CommitSHA: commitSHA,
	}

	// 4. Return cleanup function
	cleanup := func() {
		if err := removeWorktree(repoRoot, worktreePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree at %s: %v\n", worktreePath, err)
		}
	}

	return info, cleanup, nil
}

// createWorktree creates a new worktree at the specified path
func createWorktree(ctx context.Context, repoRoot, worktreePath, commitSHA string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-d", worktreePath, commitSHA)
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// removeWorktree removes the worktree using git worktree remove --force
func removeWorktree(repoRoot, worktreePath string) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %w (output: %s)", err, string(output))
	}

	return nil
}
