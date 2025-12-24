package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	// orphanAgeThreshold is the minimum age for a worktree to be considered orphaned.
	// This prevents cleaning up actively used worktrees.
	orphanAgeThreshold = 1 * time.Hour

	// worktreeDirPrefix is the prefix used for all detent worktree directories.
	worktreeDirPrefix = "detent-worktree-"
)

// CleanupOrphanedWorktrees removes worktrees that are no longer needed.
// This handles cases where the process was killed (SIGKILL) before cleanup could run.
// Returns the number of orphaned worktrees removed and any error encountered.
func CleanupOrphanedWorktrees(ctx context.Context, repoRoot string) (int, error) {
	// First, prune git worktree metadata for any worktrees that no longer exist on disk
	if err := PruneWorktreeMetadata(ctx, repoRoot); err != nil {
		return 0, err
	}

	// Find and remove orphaned temp directories
	return cleanOrphanedTempDirs()
}

// PruneWorktreeMetadata runs 'git worktree prune' to clean up stale worktree metadata.
func PruneWorktreeMetadata(ctx context.Context, repoRoot string) error {
	// #nosec G204 - repoRoot is validated before this call
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "worktree", "prune")
	cmd.Env = safeGitEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pruning worktree metadata: %w", err)
	}
	return nil
}

// cleanOrphanedTempDirs finds and removes orphaned detent worktree directories in the temp folder.
// SECURITY: Uses Lstat to detect symlinks and refuses to follow them, preventing TOCTOU attacks
// where an attacker replaces a directory with a symlink between check and removal.
func cleanOrphanedTempDirs() (int, error) {
	pattern := filepath.Join(os.TempDir(), worktreeDirPrefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, fmt.Errorf("finding orphaned worktrees: %w", err)
	}

	removed := 0
	for _, match := range matches {
		// SECURITY: Use Lstat to detect symlinks without following them
		info, err := os.Lstat(match)
		if err != nil {
			continue // Already gone or inaccessible
		}

		// SECURITY: Skip symlinks - never follow them to prevent escape attacks
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}

		// SECURITY: Must be a directory
		if !info.IsDir() {
			continue
		}

		// Only remove directories older than the threshold
		if time.Since(info.ModTime()) < orphanAgeThreshold {
			continue
		}

		// SECURITY: Re-check it's still a directory before removal
		// This is defense-in-depth against race between Lstat and RemoveAll
		recheck, err := os.Lstat(match)
		if err != nil || recheck.Mode()&os.ModeSymlink != 0 || !recheck.IsDir() {
			continue
		}

		// Attempt removal
		if err := os.RemoveAll(match); err == nil {
			removed++
		}
	}

	return removed, nil
}
