package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nightlyone/lockfile"
)

const (
	// orphanAgeThreshold is the minimum age for a worktree to be considered orphaned.
	// This prevents cleaning up actively used worktrees.
	orphanAgeThreshold = 1 * time.Hour

	// detentDirPrefix is the base prefix for all detent directories (worktrees and workflows).
	detentDirPrefix = "detent-"
)

// CleanupOrphanedWorktrees removes worktrees that are no longer needed.
// This handles cases where the process was killed (SIGKILL) before cleanup could run.
// Returns the number of orphaned worktrees removed and any error encountered.
//
// Uses lockfile-based detection: worktrees with active locks are never removed.
// Worktrees without lock files (legacy) are removed if older than 1 hour.
// Only cleans worktrees belonging to the specified repository.
func CleanupOrphanedWorktrees(ctx context.Context, repoRoot string) (int, error) {
	// First, prune git worktree metadata for any worktrees that no longer exist on disk
	if err := PruneWorktreeMetadata(ctx, repoRoot); err != nil {
		return 0, err
	}

	// Find and remove orphaned temp directories for this repo only
	return CleanOrphanedTempDirs(repoRoot, false)
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

// isOrphanedWorktree checks if a worktree directory is orphaned and eligible for removal.
// Returns true if the worktree should be cleaned up.
// repoRoot filters to a specific repo (empty string = all repos).
// force=true removes all unlocked worktrees regardless of age.
func isOrphanedWorktree(match, repoRoot string, force bool) bool {
	// Skip workflow temp directories (detent-workflows-*)
	if isWorkflowTempDir(match) {
		return false
	}

	// SECURITY: Use Lstat to detect symlinks without following them
	info, err := os.Lstat(match)
	if err != nil {
		return false // Already gone or inaccessible
	}

	// SECURITY: Skip symlinks - never follow them to prevent escape attacks
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}

	// SECURITY: Must be a directory
	if !info.IsDir() {
		return false
	}

	// If filtering by repo, check if this worktree belongs to it
	if repoRoot != "" && !isWorktreeForRepo(match, repoRoot) {
		return false
	}

	// Check if worktree is actively in use via lockfile
	lockPath := filepath.Join(match, LockFileName)
	if _, statErr := os.Stat(lockPath); statErr == nil {
		// Lock file exists - try to acquire it to check if owner is alive
		lock, lockErr := lockfile.New(lockPath)
		if lockErr != nil {
			return false // Can't create lock handle, skip this worktree
		}

		tryErr := lock.TryLock()
		switch {
		case tryErr == nil:
			// Lock acquired - owner process is dead, safe to remove
			// Unlock before removal (best effort)
			_ = lock.Unlock()
			return true
		case errors.Is(tryErr, lockfile.ErrBusy):
			// Lock is held by a running process - skip this worktree
			return false
		case errors.Is(tryErr, lockfile.ErrDeadOwner), errors.Is(tryErr, lockfile.ErrInvalidPid):
			// Dead owner or invalid PID - library auto-cleaned, safe to remove
			return true
		default:
			// Unknown error (filesystem issue, etc.) - skip to be safe
			return false
		}
	}

	// No lock file (old worktree without lock support)
	if !force {
		// Use age threshold
		if time.Since(info.ModTime()) < orphanAgeThreshold {
			return false
		}
	}
	// force=true with no lock file: remove regardless of age

	return true
}

// CountOrphanedTempDirs counts orphaned detent worktree directories that would be removed.
// This is the dry-run equivalent of CleanOrphanedTempDirs.
// Parameters have the same meaning as CleanOrphanedTempDirs.
func CountOrphanedTempDirs(repoRoot string, force bool) (int, error) {
	pattern := filepath.Join(os.TempDir(), detentDirPrefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, fmt.Errorf("finding orphaned worktrees: %w", err)
	}

	count := 0
	for _, match := range matches {
		if isOrphanedWorktree(match, repoRoot, force) {
			count++
		}
	}

	return count, nil
}

// CleanOrphanedTempDirs finds and removes orphaned detent worktree directories in the temp folder.
// Uses lockfile-based detection to determine if a worktree is actively in use.
//
// If repoRoot is non-empty, only cleans worktrees belonging to that repository.
// If repoRoot is empty, cleans all orphaned detent worktrees globally.
// If force is true, removes all worktrees not currently locked (regardless of age).
// If force is false, also applies the age threshold for worktrees without lock files.
//
// SECURITY: Uses Lstat to detect symlinks and refuses to follow them, preventing TOCTOU attacks
// where an attacker replaces a directory with a symlink between check and removal.
func CleanOrphanedTempDirs(repoRoot string, force bool) (int, error) {
	pattern := filepath.Join(os.TempDir(), detentDirPrefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, fmt.Errorf("finding orphaned worktrees: %w", err)
	}

	removed := 0
	for _, match := range matches {
		if !isOrphanedWorktree(match, repoRoot, force) {
			continue
		}

		// SECURITY: Re-check it's still a directory before removal
		// This is defense-in-depth against race between check and RemoveAll
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

// isWorkflowTempDir checks if the path is a workflow temp directory (not a worktree).
func isWorkflowTempDir(path string) bool {
	base := filepath.Base(path)
	return len(base) > 17 && base[:17] == "detent-workflows-"
}

// isWorktreeForRepo checks if the worktree at path belongs to the given repository.
// Git worktrees have a .git file that points back to the main repository.
// SECURITY: Uses Lstat to verify .git is a regular file, not a symlink.
func isWorktreeForRepo(worktreePath, repoRoot string) bool {
	gitPath := filepath.Join(worktreePath, ".git")

	// SECURITY: Verify .git is a regular file, not a symlink
	info, err := os.Lstat(gitPath)
	if err != nil {
		return false // Can't stat
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false // Symlink - refuse to follow
	}
	if !info.Mode().IsRegular() {
		return false // Not a regular file (could be a directory in non-worktree)
	}

	// #nosec G304 - worktreePath comes from glob pattern matching detent-* in temp dir
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return false // Not a git worktree or can't read
	}

	// .git file format: "gitdir: /path/to/repo/.git/worktrees/name"
	// We check if it contains the repo's .git path
	repoGitDir := filepath.Join(repoRoot, ".git")
	return strings.Contains(string(content), repoGitDir)
}
