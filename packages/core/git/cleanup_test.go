package git

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsWorkflowTempDir(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "valid workflow dir",
			path:     "/tmp/detent-workflows-1234567890",
			expected: true,
		},
		{
			name:     "valid workflow dir with longer suffix",
			path:     "/var/folders/abc/detent-workflows-abcdef123456",
			expected: true,
		},
		{
			name:     "worktree dir not workflow",
			path:     "/tmp/detent-worktree-abc123",
			expected: false,
		},
		{
			name:     "ephemeral worktree dir",
			path:     "/tmp/detent-893f68ea23aab56e",
			expected: false,
		},
		{
			name:     "too short to be workflow",
			path:     "/tmp/detent-workflows",
			expected: false,
		},
		{
			name:     "exactly 17 chars prefix only",
			path:     "/tmp/detent-workflows-",
			expected: false, // needs more than just the prefix
		},
		{
			name:     "different prefix",
			path:     "/tmp/other-workflows-123",
			expected: false,
		},
		{
			name:     "empty path",
			path:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkflowTempDir(tt.path)
			if result != tt.expected {
				t.Errorf("isWorkflowTempDir(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsWorktreeForRepo(t *testing.T) {
	t.Run("matching repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		worktreePath := filepath.Join(tmpDir, "worktree")
		repoRoot := "/home/user/my-repo"

		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			t.Fatalf("Failed to create worktree dir: %v", err)
		}

		// Create .git file that points to the repo
		gitContent := "gitdir: /home/user/my-repo/.git/worktrees/worktree-abc"
		if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte(gitContent), 0o644); err != nil {
			t.Fatalf("Failed to create .git file: %v", err)
		}

		if !isWorktreeForRepo(worktreePath, repoRoot) {
			t.Error("isWorktreeForRepo should return true for matching repo")
		}
	})

	t.Run("non-matching repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		worktreePath := filepath.Join(tmpDir, "worktree")
		repoRoot := "/home/user/other-repo"

		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			t.Fatalf("Failed to create worktree dir: %v", err)
		}

		// Create .git file that points to a different repo
		gitContent := "gitdir: /home/user/my-repo/.git/worktrees/worktree-abc"
		if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte(gitContent), 0o644); err != nil {
			t.Fatalf("Failed to create .git file: %v", err)
		}

		if isWorktreeForRepo(worktreePath, repoRoot) {
			t.Error("isWorktreeForRepo should return false for non-matching repo")
		}
	})

	t.Run("missing git file", func(t *testing.T) {
		tmpDir := t.TempDir()
		worktreePath := filepath.Join(tmpDir, "worktree")

		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			t.Fatalf("Failed to create worktree dir: %v", err)
		}

		// No .git file
		if isWorktreeForRepo(worktreePath, "/some/repo") {
			t.Error("isWorktreeForRepo should return false when .git file is missing")
		}
	})

	t.Run("nonexistent worktree path", func(t *testing.T) {
		if isWorktreeForRepo("/nonexistent/path", "/some/repo") {
			t.Error("isWorktreeForRepo should return false for nonexistent path")
		}
	})
}

// setupTestTempDir creates a temp directory and sets TMPDIR to it.
// Returns the temp directory path and a cleanup function.
func setupTestTempDir(t *testing.T) string {
	t.Helper()

	// Save original TMPDIR
	originalTmpDir := os.Getenv("TMPDIR")

	// Create temp dir using original TMPDIR (before we modify it)
	tmpDir, err := os.MkdirTemp("", "detent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Set TMPDIR to our test temp dir
	os.Setenv("TMPDIR", tmpDir)

	// Register cleanup
	t.Cleanup(func() {
		// Restore original TMPDIR
		if originalTmpDir != "" {
			os.Setenv("TMPDIR", originalTmpDir)
		} else {
			os.Unsetenv("TMPDIR")
		}
		// Clean up our temp dir
		os.RemoveAll(tmpDir)
	})

	return tmpDir
}

func TestCleanOrphanedTempDirs(t *testing.T) {
	t.Run("cleans orphaned worktree without lock", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create an orphaned worktree (old, no lock)
		orphanPath := filepath.Join(tmpDir, "detent-orphan123abc")
		if err := os.MkdirAll(orphanPath, 0o755); err != nil {
			t.Fatalf("Failed to create orphan dir: %v", err)
		}

		// Set modification time to 2 hours ago
		oldTime := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(orphanPath, oldTime, oldTime); err != nil {
			t.Fatalf("Failed to set mtime: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", false)
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 1 {
			t.Errorf("Expected 1 removed, got %d", removed)
		}

		if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
			t.Error("Orphan directory should have been removed")
		}
	})

	t.Run("skips recent worktree without lock", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create a recent worktree (no lock, but recent)
		recentPath := filepath.Join(tmpDir, "detent-recent123abc")
		if err := os.MkdirAll(recentPath, 0o755); err != nil {
			t.Fatalf("Failed to create recent dir: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", false)
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 0 {
			t.Errorf("Expected 0 removed (recent), got %d", removed)
		}

		if _, err := os.Stat(recentPath); os.IsNotExist(err) {
			t.Error("Recent directory should NOT have been removed")
		}
	})

	t.Run("force removes recent worktree without lock", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create a recent worktree (no lock)
		recentPath := filepath.Join(tmpDir, "detent-forcetest123")
		if err := os.MkdirAll(recentPath, 0o755); err != nil {
			t.Fatalf("Failed to create recent dir: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", true) // force=true
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 1 {
			t.Errorf("Expected 1 removed (force), got %d", removed)
		}

		if _, err := os.Stat(recentPath); !os.IsNotExist(err) {
			t.Error("Directory should have been removed with force=true")
		}
	})

	t.Run("skips workflow temp dirs", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create a workflow temp dir
		workflowPath := filepath.Join(tmpDir, "detent-workflows-12345678")
		if err := os.MkdirAll(workflowPath, 0o755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		// Set old time to ensure it would be cleaned if not skipped
		oldTime := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(workflowPath, oldTime, oldTime); err != nil {
			t.Fatalf("Failed to set mtime: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", true) // force=true
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 0 {
			t.Errorf("Expected 0 removed (workflow dir skipped), got %d", removed)
		}

		if _, err := os.Stat(workflowPath); os.IsNotExist(err) {
			t.Error("Workflow directory should NOT have been removed")
		}
	})

	t.Run("skips symlinks", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create a real directory to link to
		realDir := filepath.Join(tmpDir, "real-dir")
		if err := os.MkdirAll(realDir, 0o755); err != nil {
			t.Fatalf("Failed to create real dir: %v", err)
		}

		// Create a symlink with detent prefix
		symlinkPath := filepath.Join(tmpDir, "detent-symlink123")
		if err := os.Symlink(realDir, symlinkPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", true) // force=true
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 0 {
			t.Errorf("Expected 0 removed (symlink skipped), got %d", removed)
		}

		// Symlink should still exist
		if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
			t.Error("Symlink should NOT have been removed")
		}
	})

	t.Run("skips worktree with active lock", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create a worktree with active lock
		worktreePath := filepath.Join(tmpDir, "detent-locked123abc")
		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			t.Fatalf("Failed to create worktree dir: %v", err)
		}

		// Write a lock file with the parent process's PID which is always running.
		// We use PPID instead of our own PID because the lockfile library
		// allows re-acquiring locks owned by the same PID.
		lockPath := filepath.Join(worktreePath, LockFileName)
		ppid := os.Getppid()
		if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", ppid)), 0o644); err != nil {
			t.Fatalf("Failed to create lock file: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", true) // force=true
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 0 {
			t.Errorf("Expected 0 removed (locked), got %d", removed)
		}

		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			t.Error("Locked worktree should NOT have been removed")
		}
	})

	t.Run("cleans worktree with dead owner lock", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create a worktree with a stale lock (invalid PID)
		worktreePath := filepath.Join(tmpDir, "detent-deadowner123")
		if err := os.MkdirAll(worktreePath, 0o755); err != nil {
			t.Fatalf("Failed to create worktree dir: %v", err)
		}

		// Write a lock file with an invalid/dead PID
		lockPath := filepath.Join(worktreePath, LockFileName)
		// PID 99999999 is very unlikely to exist
		if err := os.WriteFile(lockPath, []byte("99999999\n"), 0o644); err != nil {
			t.Fatalf("Failed to create stale lock: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", true)
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 1 {
			t.Errorf("Expected 1 removed (dead owner), got %d", removed)
		}

		if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
			t.Error("Worktree with dead owner should have been removed")
		}
	})

	t.Run("filters by repo when repoRoot provided", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		repoRoot := filepath.Join(tmpDir, "my-repo")
		if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
			t.Fatalf("Failed to create repo: %v", err)
		}

		// Create worktree for this repo
		matchingWT := filepath.Join(tmpDir, "detent-matching123")
		if err := os.MkdirAll(matchingWT, 0o755); err != nil {
			t.Fatalf("Failed to create matching worktree: %v", err)
		}
		gitContent := "gitdir: " + filepath.Join(repoRoot, ".git", "worktrees", "matching")
		if err := os.WriteFile(filepath.Join(matchingWT, ".git"), []byte(gitContent), 0o644); err != nil {
			t.Fatalf("Failed to create .git file: %v", err)
		}

		// Create worktree for different repo
		otherWT := filepath.Join(tmpDir, "detent-other456abc")
		if err := os.MkdirAll(otherWT, 0o755); err != nil {
			t.Fatalf("Failed to create other worktree: %v", err)
		}
		otherGitContent := "gitdir: /some/other/repo/.git/worktrees/other"
		if err := os.WriteFile(filepath.Join(otherWT, ".git"), []byte(otherGitContent), 0o644); err != nil {
			t.Fatalf("Failed to create other .git file: %v", err)
		}

		removed, err := CleanOrphanedTempDirs(repoRoot, true)
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 1 {
			t.Errorf("Expected 1 removed (only matching repo), got %d", removed)
		}

		// Matching worktree should be removed
		if _, err := os.Stat(matchingWT); !os.IsNotExist(err) {
			t.Error("Matching worktree should have been removed")
		}

		// Other repo's worktree should still exist
		if _, err := os.Stat(otherWT); os.IsNotExist(err) {
			t.Error("Other repo's worktree should NOT have been removed")
		}
	})

	t.Run("skips non-directory files", func(t *testing.T) {
		tmpDir := setupTestTempDir(t)

		// Create a regular file with detent prefix
		filePath := filepath.Join(tmpDir, "detent-file123")
		if err := os.WriteFile(filePath, []byte("test"), 0o644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		removed, err := CleanOrphanedTempDirs("", true)
		if err != nil {
			t.Fatalf("CleanOrphanedTempDirs failed: %v", err)
		}

		if removed != 0 {
			t.Errorf("Expected 0 removed (file skipped), got %d", removed)
		}

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("Regular file should NOT have been removed")
		}
	})
}
