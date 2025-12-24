package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// StashInfo tracks information about a git stash created during preflight checks.
// This allows the stash to be restored during cleanup if the user chose to stash
// their uncommitted changes before running checks.
type StashInfo struct {
	Stashed      bool   // Whether changes were actually stashed
	StashRef     string // The stash reference (e.g., "stash@{0}")
	StashMessage string // The message used when creating the stash
}

// StashChanges creates a git stash with a timestamped message and returns
// information about the created stash.
// Uses `git stash push -u` to include both tracked and untracked files.
//
// Returns StashInfo with details about the created stash, or error if stashing fails.
func StashChanges(ctx context.Context, repoRoot string) (*StashInfo, error) {
	timestamp := time.Now().Format("2006-01-02T15:04:05")
	message := fmt.Sprintf("detent-auto-stash-%s", timestamp)

	// #nosec G204 - repoRoot is from user's repository, message is controlled
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "stash", "push", "-u", "-m", message)
	cmd.Env = safeGitEnv()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to stash changes: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	outputStr := strings.TrimSpace(string(output))
	if strings.Contains(outputStr, "No local changes to save") {
		return &StashInfo{Stashed: false}, nil
	}

	return &StashInfo{
		Stashed:      true,
		StashRef:     "stash@{0}",
		StashMessage: message,
	}, nil
}

// UnstashChanges pops a previously created stash to restore uncommitted changes.
// It first verifies that the stash reference still exists before attempting to pop.
//
// This is designed to be called during cleanup and will not fail the entire operation
// if unstashing fails (the caller should handle errors gracefully).
func UnstashChanges(ctx context.Context, repoRoot string, info *StashInfo) error {
	if info == nil || !info.Stashed {
		return nil
	}

	if !stashExists(ctx, repoRoot, info.StashMessage) {
		return fmt.Errorf("stash not found (it may have been manually popped): %s", info.StashMessage)
	}

	// #nosec G204 - repoRoot is from user's repository
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "stash", "pop")
	cmd.Env = safeGitEnv()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restore stashed changes: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// stashExists checks if a stash with the given message exists in the stash list.
func stashExists(ctx context.Context, repoRoot, message string) bool {
	// #nosec G204 - repoRoot is from user's repository
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "stash", "list")
	cmd.Env = safeGitEnv()

	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(output), message)
}
