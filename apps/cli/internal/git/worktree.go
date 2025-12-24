package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	cmd.Env = append(os.Environ(), secureGitEnv()...)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("getting current commit SHA: %w", err)
	}
	commitSHA := strings.TrimSpace(string(output))

	// 2. Create temp directory atomically - prevents TOCTOU attacks
	worktreeDir, err := os.MkdirTemp("", "detent-worktree-")
	if err != nil {
		return nil, nil, fmt.Errorf("creating temp directory: %w", err)
	}

	// Defense in depth: verify not a symlink
	info, err := os.Lstat(worktreeDir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		_ = os.RemoveAll(worktreeDir)
		return nil, nil, fmt.Errorf("temp directory security check failed")
	}

	// 3. Create worktree in detached HEAD state
	if err := createWorktree(ctx, repoRoot, worktreeDir, commitSHA); err != nil {
		_ = os.RemoveAll(worktreeDir)
		pruneWorktrees(repoRoot)
		return nil, nil, fmt.Errorf("creating worktree: %w", err)
	}

	worktreeInfo := &WorktreeInfo{
		Path:      worktreeDir,
		CommitSHA: commitSHA,
	}

	// 4. Return cleanup function
	cleanup := func() {
		if err := removeWorktree(repoRoot, worktreeDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree at %s: %v\n", worktreeDir, err)
		}
	}

	return worktreeInfo, cleanup, nil
}

// secureGitEnv returns environment variables that harden git against config injection attacks.
func secureGitEnv() []string {
	return []string{
		"GIT_CONFIG_NOSYSTEM=1",  // Ignore /etc/gitconfig
		"GIT_CONFIG_NOGLOBAL=1",  // Ignore ~/.gitconfig
		"GIT_TERMINAL_PROMPT=0",  // Never prompt for credentials
	}
}

// createWorktree creates a new worktree at the specified path
func createWorktree(ctx context.Context, repoRoot, worktreePath, commitSHA string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-d", worktreePath, commitSHA)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), secureGitEnv()...)

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
	cmd.Env = append(os.Environ(), secureGitEnv()...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// pruneWorktrees cleans up orphaned git worktree metadata.
func pruneWorktrees(repoRoot string) {
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "prune")
	cmd.Env = append(os.Environ(), secureGitEnv()...)
	_ = cmd.Run() // Best effort
}
