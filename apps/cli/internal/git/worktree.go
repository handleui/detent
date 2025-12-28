package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// cleanupTimeout is the maximum time allowed for cleanup operations
	cleanupTimeout = 30 * time.Second

	// maxTmpDiskUsagePct is the maximum percentage of disk usage allowed in /tmp
	// before we refuse to create a worktree (prevents filling the disk)
	maxTmpDiskUsagePct = 80
)

// WorktreeInfo contains metadata about the created worktree
type WorktreeInfo struct {
	Path      string // Absolute path to worktree directory
	CommitSHA string // Commit SHA that worktree is based on
}

// PrepareWorktree creates a git worktree for isolated workflow execution.
// The worktree is created from the current HEAD commit.
// If worktreeDir is empty, creates a temp directory. Otherwise uses the provided path.
// If worktreeDir already exists and is a valid worktree at the same commit, it is reused.
// Returns worktree info, cleanup function, and error.
//
// Note: This requires a clean worktree. Call ValidateCleanWorktree() before this.
func PrepareWorktree(ctx context.Context, repoRoot, worktreeDir string) (*WorktreeInfo, func(), error) {
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	cmd.Env = safeGitEnv()
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("getting current commit SHA: %w", err)
	}
	commitSHA := strings.TrimSpace(string(output))

	// Check if worktree already exists at the specified path and can be reused
	if worktreeDir != "" {
		if existingInfo, ok := isExistingWorktree(ctx, worktreeDir, commitSHA); ok {
			// Worktree exists and is at the same commit - reuse it
			cleanup := func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
				defer cancel()
				if rmErr := removeWorktree(cleanupCtx, repoRoot, worktreeDir); rmErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree at %s: %v\n", worktreeDir, rmErr)
				}
			}
			return existingInfo, cleanup, nil
		}
	}

	if checkErr := checkTmpDiskSpace(); checkErr != nil {
		return nil, nil, checkErr
	}

	if worktreeDir == "" {
		worktreeDir, err = os.MkdirTemp("", "detent-worktree-")
		if err != nil {
			return nil, nil, fmt.Errorf("creating temp directory: %w", err)
		}
	} else {
		// Create the specified directory
		if mkdirErr := os.MkdirAll(worktreeDir, 0o700); mkdirErr != nil {
			return nil, nil, fmt.Errorf("creating worktree directory: %w", mkdirErr)
		}
	}

	// Defense in depth: verify not a symlink
	info, err := os.Lstat(worktreeDir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		_ = os.RemoveAll(worktreeDir)
		return nil, nil, fmt.Errorf("worktree directory security check failed")
	}

	if err := createWorktree(ctx, repoRoot, worktreeDir, commitSHA); err != nil {
		_ = os.RemoveAll(worktreeDir)
		pruneWorktrees(ctx, repoRoot)
		return nil, nil, fmt.Errorf("creating worktree: %w", err)
	}

	worktreeInfo := &WorktreeInfo{
		Path:      worktreeDir,
		CommitSHA: commitSHA,
	}

	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if err := removeWorktree(cleanupCtx, repoRoot, worktreeDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree at %s: %v\n", worktreeDir, err)
		}
	}

	return worktreeInfo, cleanup, nil
}

// isExistingWorktree checks if a valid git worktree exists at the given path
// and is checked out at the expected commit SHA. Returns the worktree info if valid.
func isExistingWorktree(ctx context.Context, worktreePath, expectedCommitSHA string) (*WorktreeInfo, bool) {
	// Use Lstat to detect symlinks (security check)
	info, err := os.Lstat(worktreePath)
	if err != nil || !info.IsDir() {
		return nil, false
	}
	// Reject symlinks - only accept actual directories
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}

	// Check if it's a git worktree by verifying HEAD
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", worktreePath, "rev-parse", "HEAD")
	cmd.Env = safeGitEnv()
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	currentSHA := strings.TrimSpace(string(output))
	if currentSHA != expectedCommitSHA {
		// Worktree exists but at different commit - needs recreation
		return nil, false
	}

	return &WorktreeInfo{
		Path:      worktreePath,
		CommitSHA: currentSHA,
	}, true
}

// safeGitEnv returns a minimal, safe environment for executing git commands.
// Uses an allowlist approach to prevent inheritance of dangerous GIT_* variables like:
// - GIT_OBJECT_DIRECTORY, GIT_ALTERNATE_OBJECT_DIRECTORIES (arbitrary file access)
// - GIT_AUTHOR_NAME/EMAIL, GIT_COMMITTER_NAME/EMAIL (identity manipulation)
// - GIT_WORK_TREE, GIT_DIR (repository path manipulation)
// - GIT_INDEX_FILE (index manipulation)
// - etc.
//
// This function:
// 1. Starts with ONLY essential system variables (PATH, HOME, USER, TMPDIR, etc.)
// 2. Adds secure git overrides to harden against config injection
// 3. Does NOT inherit ANY GIT_* variables from parent environment
func safeGitEnv() []string {
	// Essential system environment variables (allowlist)
	essentialVars := []string{
		"PATH",    // Required to find git and other executables
		"HOME",    // Required for git to find default config locations
		"USER",    // Used by git for default author/committer info
		"TMPDIR",  // Temp directory location
		"TEMP",    // Windows temp directory
		"TMP",     // Alternative temp directory
		"LANG",    // Locale settings for proper output encoding
		"LC_ALL",  // Locale override
		"LC_CTYPE", // Character type locale
		"SHELL",   // Shell for git pager and commands
		"TERM",    // Terminal type
	}

	env := make([]string, 0, len(essentialVars)+8)

	// Add essential variables from current environment (if set)
	for _, key := range essentialVars {
		if value, exists := os.LookupEnv(key); exists {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Add secure git overrides
	env = append(env,
		// Ignore system and global git configuration
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_NOGLOBAL=1",

		// Never prompt for credentials or input
		"GIT_TERMINAL_PROMPT=0",

		// Prevent SSH credential prompts and MITM attacks
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new",

		// Disable credential/editor prompts with no-op executables
		"GIT_ASKPASS=/bin/true",
		"GIT_EDITOR=/bin/true",
		"GIT_PAGER=cat",

		// Ignore system gitattributes
		"GIT_ATTR_NOSYSTEM=1",
	)

	return env
}

// createWorktree creates a new worktree at the specified path
func createWorktree(ctx context.Context, repoRoot, worktreePath, commitSHA string) error {
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "worktree", "add", "-d", worktreePath, commitSHA)
	cmd.Dir = repoRoot
	cmd.Env = safeGitEnv()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// removeWorktree removes the worktree using git worktree remove --force.
// Uses context for cancellation and timeout support.
func removeWorktree(ctx context.Context, repoRoot, worktreePath string) error {
	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoRoot
	cmd.Env = safeGitEnv()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// pruneWorktrees cleans up orphaned git worktree metadata.
// Uses context for cancellation and wraps it with a cleanup timeout.
func pruneWorktrees(ctx context.Context, repoRoot string) {
	// Wrap context with cleanup timeout to ensure bounded execution
	ctx, cancel := context.WithTimeout(ctx, cleanupTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-c", "core.hooksPath=/dev/null", "-C", repoRoot, "worktree", "prune")
	cmd.Env = safeGitEnv()
	_ = cmd.Run() // Best effort
}

// ComputeRunID generates a deterministic run ID from tree and commit hashes.
// The run ID is the first 16 characters of sha256(treeHash + commitSHA).
// This ensures the same codebase state always produces the same run ID.
func ComputeRunID(treeHash, commitSHA string) string {
	h := sha256.Sum256([]byte(treeHash + commitSHA))
	return hex.EncodeToString(h[:])[:16]
}

// ComputeCurrentRunID computes a deterministic run ID for the current codebase state.
// This is a convenience wrapper that retrieves the tree hash and commit SHA in a single
// git call, then computes the run ID. Returns the run ID, tree hash, commit SHA, and any error.
func ComputeCurrentRunID(repoRoot string) (runID, treeHash, commitSHA string, err error) {
	refs, err := GetCurrentRefs(repoRoot)
	if err != nil {
		return "", "", "", err
	}
	runID = ComputeRunID(refs.TreeHash, refs.CommitSHA)
	return runID, refs.TreeHash, refs.CommitSHA, nil
}

// GetDetentWorktreesDir returns the directory where persistent worktrees are stored.
// Returns ~/.detent/worktrees/
func GetDetentWorktreesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, ".detent", "worktrees"), nil
}

// GetWorktreePath returns the persistent worktree path for a given run ID.
// Returns ~/.detent/worktrees/{runID}/
// Validates that the runID is a safe hex string to prevent path traversal attacks.
func GetWorktreePath(runID string) (string, error) {
	// Validate runID is a hex string (as produced by ComputeRunID)
	// This prevents path traversal attacks with "../" or other malicious input
	if !isValidRunID(runID) {
		return "", fmt.Errorf("invalid run ID: must be a hex string")
	}

	worktreesDir, err := GetDetentWorktreesDir()
	if err != nil {
		return "", err
	}

	result := filepath.Join(worktreesDir, runID)

	// Defense in depth: verify the result is still within worktreesDir
	// This protects against edge cases in filepath.Join behavior
	if !strings.HasPrefix(filepath.Clean(result), filepath.Clean(worktreesDir)) {
		return "", fmt.Errorf("path traversal detected in run ID")
	}

	return result, nil
}

// WorktreeExists checks if a worktree exists for the given run ID.
// Uses Lstat to prevent symlink attacks - a symlink to a directory
// is not considered a valid worktree.
func WorktreeExists(runID string) bool {
	path, err := GetWorktreePath(runID)
	if err != nil {
		return false
	}
	// Use Lstat instead of Stat to detect symlinks
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	// Reject symlinks - only accept actual directories
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return info.IsDir()
}

// isValidRunID validates that a run ID contains only hexadecimal characters.
// This prevents path traversal attacks since ComputeRunID produces hex strings.
func isValidRunID(runID string) bool {
	if runID == "" || len(runID) > 64 {
		return false
	}
	for _, c := range runID {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return false
		}
	}
	return true
}
