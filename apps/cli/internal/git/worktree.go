package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nightlyone/lockfile"
)

const (
	// CleanupTimeout is the maximum time allowed for cleanup operations
	CleanupTimeout = 30 * time.Second

	// maxTmpDiskUsagePct is the maximum percentage of disk usage allowed in /tmp
	// before we refuse to create a worktree (prevents filling the disk)
	maxTmpDiskUsagePct = 80

	// LockFileName is the name of the lockfile used to track worktree ownership.
	// The lockfile library writes the owning process's PID to this file.
	LockFileName = ".detent.lock"

	// lockRetryAttempts is the number of times to retry acquiring a lock on temporary errors.
	lockRetryAttempts = 3

	// lockRetryDelay is the delay between lock acquisition attempts.
	lockRetryDelay = 100 * time.Millisecond
)

// WorktreeInfo contains metadata about the created worktree
type WorktreeInfo struct {
	Path      string            // Absolute path to worktree directory
	CommitSHA string            // Commit SHA that worktree is based on
	lock      lockfile.Lockfile // Lockfile for tracking ownership (unexported)
}

// tryLockWithRetry attempts to acquire a lock with retries on temporary errors.
// Returns nil on success, lockfile.ErrBusy if lock is held by another process,
// or another error if the lock cannot be acquired after retries.
func tryLockWithRetry(lock lockfile.Lockfile) error {
	var lastErr error
	for range lockRetryAttempts {
		lastErr = lock.TryLock()
		if lastErr == nil {
			return nil // Lock acquired
		}

		// Check if error is temporary (ErrBusy, ErrNotExist)
		if te, ok := lastErr.(interface{ Temporary() bool }); ok && te.Temporary() {
			// ErrBusy means another process holds the lock - don't retry
			if lastErr == lockfile.ErrBusy {
				return lastErr
			}
			// ErrNotExist is transient (file disappeared during creation) - retry
			time.Sleep(lockRetryDelay)
			continue
		}

		// Permanent error - don't retry
		return lastErr
	}
	return lastErr
}

// unlockWithLogging releases a lock and logs any errors appropriately.
// ErrRogueDeletion is logged as an error (potential security concern),
// other errors are logged as warnings.
func unlockWithLogging(lock lockfile.Lockfile, worktreeDir string) {
	if unlockErr := lock.Unlock(); unlockErr != nil {
		if unlockErr == lockfile.ErrRogueDeletion {
			fmt.Fprintf(os.Stderr, "Error: worktree lock was unexpectedly deleted: %s\n", worktreeDir)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: failed to unlock worktree at %s: %v\n", worktreeDir, unlockErr)
		}
	}
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
			// Worktree exists and is at the same commit - try to acquire lock for reuse
			lock, lockErr := lockfile.New(filepath.Join(worktreeDir, LockFileName))
			if lockErr == nil {
				lockErr = tryLockWithRetry(lock)
				if lockErr == nil {
					// Lock acquired - we can reuse this worktree
					existingInfo.lock = lock
					cleanup := func() {
						unlockWithLogging(existingInfo.lock, worktreeDir)
						cleanupCtx, cancel := context.WithTimeout(context.Background(), CleanupTimeout)
						defer cancel()
						if rmErr := removeWorktree(cleanupCtx, repoRoot, worktreeDir); rmErr != nil {
							fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree at %s: %v\n", worktreeDir, rmErr)
						}
					}
					return existingInfo, cleanup, nil
				}
				// ErrBusy means another process owns it - fall through to handle
				// Other errors mean we should try recreating the worktree
			}
			// Can't create lock or lock busy - fall through to handle
		}
		// Directory exists but is not a valid worktree (orphaned/broken) - clean it up
		// SECURITY: Use Lstat to detect symlinks before removal
		if info, statErr := os.Lstat(worktreeDir); statErr == nil {
			// Only remove if it's an actual directory, not a symlink
			if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				_ = os.RemoveAll(worktreeDir)
				pruneWorktrees(ctx, repoRoot)
			} else if info.Mode()&os.ModeSymlink != 0 {
				// Symlink found where worktree should be - security issue
				return nil, nil, fmt.Errorf("worktree path %s is a symlink, refusing to proceed", worktreeDir)
			}
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

	// Defense in depth: verify not a symlink after creation
	info, err := os.Lstat(worktreeDir)
	if err != nil {
		return nil, nil, fmt.Errorf("worktree directory security check failed: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("worktree path %s is a symlink, refusing to proceed", worktreeDir)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("worktree path %s is not a directory", worktreeDir)
	}

	if err := createWorktree(ctx, repoRoot, worktreeDir, commitSHA); err != nil {
		_ = safeRemoveDir(worktreeDir)
		pruneWorktrees(ctx, repoRoot)
		return nil, nil, fmt.Errorf("creating worktree: %w", err)
	}

	// Acquire lockfile to track ownership of this worktree.
	// The lockfile library writes our PID to the file, allowing orphan detection.
	lock, lockErr := lockfile.New(filepath.Join(worktreeDir, LockFileName))
	if lockErr != nil {
		_ = safeRemoveDir(worktreeDir)
		pruneWorktrees(ctx, repoRoot)
		return nil, nil, fmt.Errorf("creating lockfile: %w", lockErr)
	}
	if lockErr = tryLockWithRetry(lock); lockErr != nil {
		_ = safeRemoveDir(worktreeDir)
		pruneWorktrees(ctx, repoRoot)
		return nil, nil, fmt.Errorf("acquiring worktree lock: %w", lockErr)
	}

	worktreeInfo := &WorktreeInfo{
		Path:      worktreeDir,
		CommitSHA: commitSHA,
		lock:      lock,
	}

	cleanup := func() {
		// Release lock before removing worktree
		unlockWithLogging(worktreeInfo.lock, worktreeDir)
		cleanupCtx, cancel := context.WithTimeout(context.Background(), CleanupTimeout)
		defer cancel()
		if err := removeWorktree(cleanupCtx, repoRoot, worktreeDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree at %s: %v\n", worktreeDir, err)
		}
	}

	// Sync any uncommitted changes from the source repo to the worktree
	if err := SyncDirtyFiles(ctx, repoRoot, worktreeDir); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("syncing dirty files: %w", err)
	}

	return worktreeInfo, cleanup, nil
}

// SyncDirtyFiles copies uncommitted changes from the source repo to the worktree.
// This allows running checks on dirty worktrees by syncing modified/added files.
// Deleted files are skipped (they shouldn't exist in the worktree).
func SyncDirtyFiles(ctx context.Context, repoRoot, worktreeDir string) error {
	files, err := GetDirtyFilesList(ctx, repoRoot)
	if err != nil || len(files) == 0 {
		return err
	}

	for _, entry := range files {
		if len(entry) < 3 {
			continue // Skip malformed entries
		}

		status := entry[:2]
		filePath := strings.TrimSpace(entry[3:])

		// Skip deleted files - they shouldn't exist in worktree
		if status[0] == 'D' || status[1] == 'D' {
			continue
		}

		// Skip renamed files' old paths (R = renamed, shows as "R  old -> new")
		if strings.Contains(filePath, " -> ") {
			// For renames, we only care about the new path
			parts := strings.Split(filePath, " -> ")
			if len(parts) == 2 {
				filePath = strings.TrimSpace(parts[1])
			}
		}

		src := filepath.Join(repoRoot, filePath)
		dst := filepath.Join(worktreeDir, filePath)

		if err := copyFile(src, dst); err != nil {
			// Non-fatal: file might not exist (deleted after status check)
			continue
		}
	}

	return nil
}

// copyFile copies a file from src to dst, creating parent directories as needed.
// Used to sync uncommitted changes from source repo to worktree.
func copyFile(src, dst string) error {
	// #nosec G304 - src is from git status output, paths within user's repo
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Create destination directory with restrictive permissions
	dstDir := filepath.Dir(dst)
	if mkdirErr := os.MkdirAll(dstDir, 0o700); mkdirErr != nil {
		return mkdirErr
	}

	// #nosec G304 - dst is worktree path, constructed from known safe paths
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	// Copy contents
	_, err = io.Copy(dstFile, srcFile)
	return err
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

// safeRemoveDir removes a directory only if it's a real directory (not a symlink).
// This is a defense-in-depth measure to prevent TOCTOU attacks where an attacker
// might replace a directory with a symlink between security checks.
// Returns nil if path doesn't exist or is a symlink (no action taken).
func safeRemoveDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already gone
		}
		return err
	}

	// Only remove actual directories, never symlinks
	if info.Mode()&os.ModeSymlink != 0 {
		return nil // Don't remove symlinks
	}
	if !info.IsDir() {
		return nil // Don't remove non-directories
	}

	return os.RemoveAll(path)
}

// pruneWorktrees cleans up orphaned git worktree metadata.
// Uses context for cancellation and wraps it with a cleanup timeout.
func pruneWorktrees(ctx context.Context, repoRoot string) {
	// Wrap context with cleanup timeout to ensure bounded execution
	ctx, cancel := context.WithTimeout(ctx, CleanupTimeout)
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

// CreateEphemeralWorktreePath returns a path for an ephemeral worktree in the system temp directory.
// Format: {tempDir}/detent-{runID}
// Validates that the runID is a safe hex string to prevent path traversal attacks.
func CreateEphemeralWorktreePath(runID string) (string, error) {
	// Validate runID is a hex string (as produced by ComputeRunID)
	// This prevents path traversal attacks with "../" or other malicious input
	if !isValidRunID(runID) {
		return "", fmt.Errorf("invalid run ID: must be a hex string")
	}

	tempDir := os.TempDir()
	result := filepath.Join(tempDir, "detent-"+runID)

	// Defense in depth: verify the result is still within tempDir
	if !strings.HasPrefix(filepath.Clean(result), filepath.Clean(tempDir)) {
		return "", fmt.Errorf("path traversal detected in run ID")
	}

	return result, nil
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
