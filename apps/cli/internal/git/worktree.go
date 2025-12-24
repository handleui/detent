package git

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sync/errgroup"
)

// WorktreeInfo contains metadata about the created worktree
type WorktreeInfo struct {
	Path           string   // Absolute path to worktree directory
	BaseCommitSHA  string   // Commit SHA that worktree is based on
	IsDirty        bool     // Whether working directory had uncommitted changes
	DirtyFiles     []string // List of files with uncommitted changes (relative paths)
	WorktreeCommit string   // Commit SHA in worktree after committing dirty changes
}

// PrepareWorktree creates a temporary git worktree for isolated workflow execution.
// Returns worktree info, cleanup function, and error.
func PrepareWorktree(ctx context.Context, repoRoot, runID string) (*WorktreeInfo, func(), error) {
	// 1. Get current commit SHA from repoRoot
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("getting current commit SHA: %w", err)
	}
	baseCommitSHA := strings.TrimSpace(string(output))

	// 2. Detect dirty state
	isDirty, dirtyFiles, err := detectDirtyState(repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("detecting dirty state: %w", err)
	}

	// 3. Create worktree path
	worktreePath := filepath.Join(os.TempDir(), fmt.Sprintf("detent-%s", runID))

	// 4. Create worktree in detached HEAD state
	if err := createWorktree(ctx, repoRoot, worktreePath, baseCommitSHA); err != nil {
		return nil, nil, fmt.Errorf("creating worktree: %w", err)
	}

	// 5. If dirty, copy changes and commit
	worktreeCommit := baseCommitSHA
	if isDirty {
		if copyErr := copyDirtyFiles(repoRoot, worktreePath, dirtyFiles); copyErr != nil {
			_ = removeWorktree(repoRoot, worktreePath)
			return nil, nil, fmt.Errorf("copying dirty files: %w", copyErr)
		}

		var commitErr error
		worktreeCommit, commitErr = commitDirtyChanges(ctx, worktreePath)
		if commitErr != nil {
			_ = removeWorktree(repoRoot, worktreePath)
			return nil, nil, fmt.Errorf("committing dirty changes: %w", commitErr)
		}
	}

	info := &WorktreeInfo{
		Path:           worktreePath,
		BaseCommitSHA:  baseCommitSHA,
		IsDirty:        isDirty,
		DirtyFiles:     dirtyFiles,
		WorktreeCommit: worktreeCommit,
	}

	// 6. Return cleanup function
	cleanup := func() {
		if err := removeWorktree(repoRoot, worktreePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree at %s: %v\n", worktreePath, err)
		}
	}

	return info, cleanup, nil
}

// detectDirtyState checks for uncommitted changes using git status --porcelain
func detectDirtyState(repoRoot string) (bool, []string, error) {
	// Use -uall to show all untracked files, not just directories
	cmd := exec.Command("git", "status", "--porcelain", "-uall")
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return false, nil, fmt.Errorf("running git status: %w", err)
	}

	// Don't trim the output before splitting - git status format depends on exact spacing
	outputStr := string(output)
	if strings.TrimSpace(outputStr) == "" {
		return false, nil, nil // No changes
	}

	lines := strings.Split(outputStr, "\n")

	dirtyFiles := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(line) < 3 {
			continue
		}

		// Git status --porcelain format: XY filename
		// Where X is index status, Y is working tree status
		// Position 0-1: status codes
		// Position 2: always a space
		// Position 3+: filename
		// Examples: " M file.txt", "?? file.txt", " D file.txt", "MM file.txt"
		filename := strings.TrimSpace(line[3:])

		// Handle renames: "R  old -> new"
		if strings.Contains(filename, " -> ") {
			parts := strings.Split(filename, " -> ")
			if len(parts) == 2 {
				filename = parts[1]
			}
		}

		// Skip directory entries (git status shows untracked directories with trailing /)
		// We only want files, not directories themselves
		if strings.HasSuffix(filename, "/") {
			continue
		}

		if filename != "" {
			dirtyFiles = append(dirtyFiles, filename)
		}
	}

	return len(dirtyFiles) > 0, dirtyFiles, nil
}

// createWorktree creates a new worktree at the specified path
func createWorktree(ctx context.Context, repoRoot, worktreePath, commitSHA string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-d", worktreePath, commitSHA)
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// copyDirtyFiles copies uncommitted changes from source repo to worktree in parallel
func copyDirtyFiles(repoRoot, worktreePath string, dirtyFiles []string) error {
	// Determine worker pool size: use number of CPUs, capped between 4-8
	numWorkers := runtime.NumCPU()
	if numWorkers < 4 {
		numWorkers = 4
	}
	if numWorkers > 8 {
		numWorkers = 8
	}

	// Create errgroup for coordinated goroutine execution
	g := new(errgroup.Group)
	g.SetLimit(numWorkers)

	// Process each file in parallel
	for _, relPath := range dirtyFiles {
		// Capture loop variable for goroutine
		relPath := relPath

		g.Go(func() error {
			srcPath := filepath.Join(repoRoot, relPath)
			dstPath := filepath.Join(worktreePath, relPath)

			// Create destination directory if needed
			dstDir := filepath.Dir(dstPath)
			if err := os.MkdirAll(dstDir, 0o700); err != nil {
				return fmt.Errorf("creating directory %s: %w", dstDir, err)
			}

			// Check if source exists and get its info
			// #nosec G304 -- srcPath is derived from git status output and repoRoot, controlled by the application
			srcInfo, err := os.Stat(srcPath)
			if os.IsNotExist(err) {
				// Source file deleted - remove from worktree too
				_ = os.Remove(dstPath)
				return nil
			} else if err != nil {
				return fmt.Errorf("stat %s: %w", relPath, err)
			}

			// Skip directories - we only copy files
			if srcInfo.IsDir() {
				return nil
			}

			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("copying %s: %w", relPath, err)
			}

			return nil
		})
	}

	// Wait for all goroutines to complete and return first error if any
	return g.Wait()
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	// #nosec G304 -- src and dst paths are derived from git status output and controlled by the application
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := srcFile.Close(); closeErr != nil {
			// Log close error but don't override return value
			fmt.Fprintf(os.Stderr, "Warning: failed to close source file %s: %v\n", src, closeErr)
		}
	}()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// #nosec G304 -- dst path is derived from git status output and controlled by the application
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := dstFile.Close(); closeErr != nil {
			// Log close error but don't override return value
			fmt.Fprintf(os.Stderr, "Warning: failed to close destination file %s: %v\n", dst, closeErr)
		}
	}()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// commitDirtyChanges commits all changes in worktree with --no-verify
func commitDirtyChanges(ctx context.Context, worktreePath string) (string, error) {
	// Stage all changes
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = worktreePath

	if output, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %w (output: %s)", err, string(output))
	}

	// Commit with --no-verify
	commitMsg := "[detent] Temporary commit for worktree isolation"
	commitCmd := exec.CommandContext(ctx, "git", "commit", "--no-verify", "-m", commitMsg)
	commitCmd.Dir = worktreePath

	if output, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}

	// Get commit SHA
	shaCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	shaCmd.Dir = worktreePath

	output, err := shaCmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting commit SHA: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// removeWorktree removes the worktree using git worktree remove --force
func removeWorktree(repoRoot, worktreePath string) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %w (output: %s)", err, string(output))
	}

	return nil
}
