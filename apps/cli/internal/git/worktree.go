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

	// If no worktree dir specified, create temp directory (legacy behavior)
	if worktreeDir == "" {
		if checkErr := checkTmpDiskSpace(); checkErr != nil {
			return nil, nil, checkErr
		}
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
