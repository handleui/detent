package git

import (
	"bytes"
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

// ErrNotGitRepository is returned when a path is not a git repository
var ErrNotGitRepository = fmt.Errorf("not a git repository")

// ErrSymlinkEscape is returned when a symlink in the repo points outside the repo root
var ErrSymlinkEscape = fmt.Errorf("symlink escapes repository boundaries")

// ErrSubmodulesNotSupported is returned when a repo contains submodules
var ErrSubmodulesNotSupported = fmt.Errorf("submodules are not yet supported")

// ErrSymlinkLimitExceeded is returned when symlink validation limits are exceeded
var ErrSymlinkLimitExceeded = fmt.Errorf("symlink validation limit exceeded")

const (
	// maxSymlinkDepth is the maximum directory depth to traverse
	maxSymlinkDepth = 100
	// maxSymlinksChecked is the maximum number of symlinks to check
	maxSymlinksChecked = 10000
)

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
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", path, "rev-parse", "--git-dir")
	cmd.Env = safeGitEnv()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", ErrNotGitRepository, path)
	}
	return nil
}

// executeGitStatus runs git status --porcelain -uall and returns the output.
// This is the single source of truth for checking worktree status.
func executeGitStatus(ctx context.Context, repoRoot string) (string, error) {
	// #nosec G204 - repoRoot is from user's repository, expected behavior
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "status", "--porcelain", "-uall")
	cmd.Env = safeGitEnv()
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to check git status: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ValidateNoEscapingSymlinks walks the repository and checks that no symlinks
// point to targets outside the repository root. This prevents symlink escape attacks.
// Skips .git directory and common dependency directories for performance.
// Enforces limits on traversal depth and number of symlinks to prevent DOS attacks.
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

	// Track symlinks checked to prevent DOS attacks
	symlinksChecked := 0

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

		// Calculate depth relative to repo root
		rel, relErr := filepath.Rel(absRepoRoot, path)
		if relErr != nil {
			return nil // Skip paths we can't calculate relative path for
		}

		// Check depth limit (count path separators + 1)
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(filepath.Separator)) + 1
		}
		if depth > maxSymlinkDepth {
			return fmt.Errorf("%w: maximum traversal depth (%d) exceeded", ErrSymlinkLimitExceeded, maxSymlinkDepth)
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

		// Check symlink count limit
		symlinksChecked++
		if symlinksChecked > maxSymlinksChecked {
			return fmt.Errorf("%w: maximum symlink count (%d) exceeded", ErrSymlinkLimitExceeded, maxSymlinksChecked)
		}

		// Resolve the symlink target
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			// SECURITY: Don't silently ignore broken symlinks - check if they point outside repo
			rawTarget, readErr := os.Readlink(path)
			if readErr != nil {
				return nil // Can't read symlink at all, skip
			}

			// For absolute targets, check if they escape the repo
			if filepath.IsAbs(rawTarget) {
				// Compute the absolute path of where the symlink would resolve to
				// For broken symlinks, we can't use EvalSymlinks, so we normalize the path
				absTarget := filepath.Clean(rawTarget)

				// Try to resolve any symlinks in the parent path (e.g., /var -> /private/var on macOS)
				// This handles cases where the repo is in /private/var but target uses /var
				parentDir := filepath.Dir(absTarget)
				if resolvedParent, resolveErr := filepath.EvalSymlinks(parentDir); resolveErr == nil {
					absTarget = filepath.Join(resolvedParent, filepath.Base(absTarget))
				}

				// Now check if target is within repo root
				relTarget, relErr := filepath.Rel(absRepoRoot, absTarget)
				if relErr != nil || strings.HasPrefix(relTarget, "..") {
					return fmt.Errorf("%w: broken symlink %s points outside repository (target: %s)",
						ErrSymlinkEscape, path, rawTarget)
				}
			}

			// Relative broken symlinks are OK - will fail naturally if accessed
			return nil
		}

		// Ensure target is within repo root
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return nil
		}

		rel, err = filepath.Rel(absRepoRoot, absTarget)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%w: %s points to %s", ErrSymlinkEscape, path, target)
		}

		return nil
	})
}

// ValidateNoSubmodules checks if the repository contains git submodules.
// Returns ErrSubmodulesNotSupported if .gitmodules file exists.
// Also checks for CVE-2025-48384: carriage return characters in .gitmodules that
// can be used for arbitrary file write attacks.
func ValidateNoSubmodules(repoRoot string) error {
	gitmodulesPath := filepath.Join(repoRoot, ".gitmodules")

	// #nosec G304 - gitmodulesPath is constructed from validated repoRoot
	content, err := os.ReadFile(gitmodulesPath)
	if os.IsNotExist(err) {
		return nil // No submodules - OK
	}
	if err != nil {
		return fmt.Errorf("reading .gitmodules: %w", err)
	}

	// CVE-2025-48384: Check for CR characters that can be used for path manipulation attacks
	if bytes.Contains(content, []byte("\r")) {
		return fmt.Errorf("%w: .gitmodules contains carriage return characters (potential CVE-2025-48384 attack)",
			ErrSubmodulesNotSupported)
	}

	// Submodules exist - not supported
	return fmt.Errorf("%w: please remove submodules or use a repository without them", ErrSubmodulesNotSupported)
}

// GetDirtyFilesList returns a list of files with uncommitted or untracked changes.
// Each entry is formatted as git status output (e.g., "M  file.go", "?? newfile.go").
// This is used to display uncommitted files to the user during interactive prompts.
//
// Returns empty slice if worktree is clean, or error if git status command fails.
func GetDirtyFilesList(ctx context.Context, repoRoot string) ([]string, error) {
	status, err := executeGitStatus(ctx, repoRoot)
	if err != nil {
		return nil, err
	}

	if status == "" {
		return []string{}, nil
	}

	return strings.Split(status, "\n"), nil
}
