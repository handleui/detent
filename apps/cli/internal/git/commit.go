package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GetCurrentCommitSHA retrieves the current git commit SHA.
// Returns an error if not in a git repository or git is not available.
func GetCurrentCommitSHA() (string, error) {
	cmd := exec.Command("git", "-c", "core.hooksPath=/dev/null", "rev-parse", "HEAD")
	cmd.Env = safeGitEnv()
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git commit SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// CommitAllChanges stages all changes and creates a commit with the given message.
// This is used when the user chooses to commit their changes during preflight checks.
//
// The function:
// 1. Runs `git add .` to stage all changes (tracked and untracked)
// 2. Runs `git commit -m "message"` to create the commit
//
// Returns error if staging or committing fails. We trust git commit's exit code
// for validation rather than running an additional git status check.
func CommitAllChanges(ctx context.Context, repoRoot, message string) error {
	// #nosec G204 - repoRoot is from user's repository
	addCmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "add", ".")
	addCmd.Env = safeGitEnv()

	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage changes: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	// #nosec G204 - repoRoot and message are user-provided
	commitCmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "commit", "-m", message)
	commitCmd.Env = safeGitEnv()

	output, err := commitCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	// Trust that git commit succeeded - it would have returned error otherwise
	return nil
}
