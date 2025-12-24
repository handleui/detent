package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ErrWorktreeNotInitialized is returned when operations are attempted before worktree creation
var ErrWorktreeNotInitialized = fmt.Errorf("worktree not initialized - Prepare() must be called before Run()")

// ErrWorktreeDirty is returned when the worktree has uncommitted or untracked changes
var ErrWorktreeDirty = fmt.Errorf("worktree has uncommitted changes - please commit before running")

// ErrNotGitRepository is returned when a path is not a git repository
var ErrNotGitRepository = fmt.Errorf("not a git repository")

// ValidateWorktreeInitialized checks if worktree info is present
// Returns ErrWorktreeNotInitialized if nil
func ValidateWorktreeInitialized(info *WorktreeInfo) error {
	if info == nil {
		return ErrWorktreeNotInitialized
	}
	return nil
}

// ValidateGitRepository checks if the given path is a git repository.
// Returns ErrNotGitRepository if the path is not a git repository.
func ValidateGitRepository(ctx context.Context, path string) error {
	// #nosec G204 - path is from user's repository, expected behavior
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", ErrNotGitRepository, path)
	}
	return nil
}

// ValidateCleanWorktree checks if the worktree has any uncommitted or untracked changes.
// Returns ErrWorktreeDirty if the worktree is not clean.
// This requires all files to be committed before running checks, ensuring the checked
// state exactly matches what will be pushed to the remote.
func ValidateCleanWorktree(ctx context.Context, repoRoot string) error {
	// #nosec G204 - repoRoot is from user's repository, expected behavior
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "status", "--porcelain", "-uall")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}

	status := strings.TrimSpace(string(output))
	if status != "" {
		return fmt.Errorf("%w:\n%s", ErrWorktreeDirty, status)
	}

	return nil
}
