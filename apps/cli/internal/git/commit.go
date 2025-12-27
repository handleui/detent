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

// GetCurrentTreeHash retrieves the tree hash for the current HEAD commit.
// The tree hash represents the exact state of all files at this commit,
// independent of commit metadata (author, message, parent commits).
// This is useful for cache identity across rebases where content is unchanged.
func GetCurrentTreeHash(repoRoot string) (string, error) {
	// #nosec G204 - repoRoot is from user's repository
	cmd := exec.Command("git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "rev-parse", "HEAD^{tree}")
	cmd.Env = safeGitEnv()
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git tree hash: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GetFirstCommitSHA retrieves the SHA of the first (root) commit in the repository.
// This is immutable and unique per repository, useful for identifying repos.
// Returns empty string if the repository has no commits.
func GetFirstCommitSHA(repoRoot string) (string, error) {
	// #nosec G204 - repoRoot is from user's repository
	cmd := exec.Command("git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "rev-list", "--max-parents=0", "HEAD")
	cmd.Env = safeGitEnv()
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get first commit SHA: %w", err)
	}
	// May return multiple lines for repos with multiple roots (rare); take first
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", nil
	}
	return lines[0], nil
}

// GetRemoteURL retrieves the URL of the origin remote.
// Returns empty string if no origin remote exists (e.g., local-only repo).
func GetRemoteURL(repoRoot string) (string, error) {
	// #nosec G204 - repoRoot is from user's repository
	cmd := exec.Command("git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "remote", "get-url", "origin")
	cmd.Env = safeGitEnv()
	output, err := cmd.Output()
	if err != nil {
		// No origin remote is not an error, just return empty
		return "", nil
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
