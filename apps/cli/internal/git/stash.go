package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// stashRestoreTimeout is the maximum time allowed for restoring stashed changes.
	// Set higher than git command timeout to handle large stashes with many files.
	stashRestoreTimeout = 30 * time.Second
)

// StashInfo tracks information about a git stash created during preflight checks.
// This allows the stash to be restored during cleanup if the user chose to stash
// their uncommitted changes before running checks.
type StashInfo struct {
	Stashed      bool   // Whether changes were actually stashed
	StashRef     string // The stash SHA (e.g., "a1b2c3d...") for reliable restoration
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
	outputStr := strings.TrimSpace(string(output))
	if err != nil {
		return nil, fmt.Errorf("failed to stash changes: %w (output: %s)", err, outputStr)
	}
	if strings.Contains(outputStr, "No local changes to save") {
		return &StashInfo{Stashed: false}, nil
	}

	// Get the actual stash SHA to ensure we pop the correct one later
	shaCmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "rev-parse", "stash@{0}")
	shaCmd.Env = safeGitEnv()
	shaOutput, err := shaCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get stash SHA: %w", err)
	}

	return &StashInfo{
		Stashed:      true,
		StashRef:     strings.TrimSpace(string(shaOutput)), // Use SHA instead of stash@{0}
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

	if !stashExists(ctx, repoRoot, info.StashRef) {
		return fmt.Errorf("stash not found (it may have been manually popped): %s", info.StashMessage)
	}

	// #nosec G204 - repoRoot is from user's repository, StashRef is our controlled SHA
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "stash", "pop", info.StashRef)
	cmd.Env = safeGitEnv()

	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))
	if err != nil {
		return fmt.Errorf("failed to restore stashed changes: %w (output: %s)", err, outputStr)
	}

	return nil
}

// stashExists checks if a stash SHA exists in the repository.
func stashExists(ctx context.Context, repoRoot, stashRef string) bool {
	// Try to verify the stash SHA exists using git cat-file
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "cat-file", "-e", stashRef)
	cmd.Env = safeGitEnv()
	return cmd.Run() == nil
}

// RestoreStashIfNeeded restores a stash if one was created during preflight.
// This is designed to be called from cleanup functions and handles errors gracefully
// by printing warnings rather than failing.
//
// Note: This function mutates the StashInfo.Stashed field to false after successful
// restoration to prevent double-pop if called multiple times.
func RestoreStashIfNeeded(repoRoot string, info *StashInfo) {
	if info == nil || !info.Stashed {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), stashRestoreTimeout)
	defer cancel()

	if err := UnstashChanges(ctx, repoRoot, info); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to restore stashed changes: %v\n", err)
		fmt.Fprintf(os.Stderr, "Your changes are still in the stash. Run 'git stash pop' to restore them manually.\n")
	}

	// Mark as already restored to prevent double-pop
	info.Stashed = false
}
