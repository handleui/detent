package git

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrWorktreeNotInitialized is returned when operations are attempted before worktree creation
var ErrWorktreeNotInitialized = fmt.Errorf("worktree not initialized - Prepare() must be called before Run()")

// ErrWorktreeDirty is returned when the worktree has uncommitted or untracked changes
var ErrWorktreeDirty = fmt.Errorf("worktree has uncommitted changes - please commit before running")

// ErrNotGitRepository is returned when a path is not a git repository
var ErrNotGitRepository = fmt.Errorf("not a git repository")

// ErrSymlinkEscape is returned when a symlink in the repo points outside the repo root
var ErrSymlinkEscape = fmt.Errorf("symlink escapes repository boundaries")

// ErrSubmodulesNotSupported is returned when a repo contains submodules
var ErrSubmodulesNotSupported = fmt.Errorf("submodules are not yet supported")

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

// ValidateNoEscapingSymlinks walks the repository and checks that no symlinks
// point to targets outside the repository root. This prevents symlink escape attacks.
// Skips .git directory and common dependency directories for performance.
func ValidateNoEscapingSymlinks(ctx context.Context, repoRoot string) error {
	// Resolve repoRoot to its canonical path (resolves symlinks like /var -> /private/var on macOS)
	absRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		// If EvalSymlinks fails, fall back to Abs
		absRepoRoot, err = filepath.Abs(repoRoot)
		if err != nil {
			return fmt.Errorf("resolving repo root: %w", err)
		}
	}

	return filepath.WalkDir(absRepoRoot, func(path string, d fs.DirEntry, err error) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return nil // Skip paths we can't access
		}

		// Skip .git and common dependency directories
		name := d.Name()
		if d.IsDir() && (name == ".git" || name == "node_modules" || name == "vendor" || name == ".venv") {
			return filepath.SkipDir
		}

		// Only check symlinks
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}

		// Resolve the symlink target
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil // Broken symlink - OK, will fail naturally later
		}

		// Ensure target is within repo root
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(absRepoRoot, absTarget)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%w: %s points to %s", ErrSymlinkEscape, path, target)
		}

		return nil
	})
}

// ValidateNoSubmodules checks if the repository contains git submodules.
// Returns ErrSubmodulesNotSupported if .gitmodules file exists.
func ValidateNoSubmodules(repoRoot string) error {
	gitmodulesPath := filepath.Join(repoRoot, ".gitmodules")
	if _, err := os.Stat(gitmodulesPath); err == nil {
		return fmt.Errorf("%w: please remove submodules or use a repository without them", ErrSubmodulesNotSupported)
	}
	return nil
}
