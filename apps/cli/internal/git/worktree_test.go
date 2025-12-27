package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupGitRepo creates a temporary git repository with initial commit
func setupGitRepo(t *testing.T) (repoPath string, cleanup func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Initialize git repo
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	// Configure git
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("Failed to configure git email: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("Failed to configure git name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "add", "README.md").Run(); err != nil {
		t.Fatalf("Failed to add README: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	cleanup = func() {
		// Cleanup any leftover worktrees
		_ = exec.Command("git", "-C", tmpDir, "worktree", "prune").Run()
	}

	return tmpDir, cleanup
}

// TestPrepareWorktree_CleanRepo tests worktree creation in a clean repository
func TestPrepareWorktree_CleanRepo(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx := context.Background()

	info, cleanupWorktree, err := PrepareWorktree(ctx, repoPath, "")
	if err != nil {
		t.Fatalf("PrepareWorktree() failed: %v", err)
	}
	defer cleanupWorktree()

	// Verify WorktreeInfo
	if info == nil {
		t.Fatal("WorktreeInfo should not be nil")
	}

	// Path should be in temp directory with our prefix (atomic creation uses random suffix)
	expectedPrefix := filepath.Join(os.TempDir(), "detent-worktree-")
	if !strings.HasPrefix(info.Path, expectedPrefix) {
		t.Errorf("WorktreeInfo.Path = %s, should start with %s", info.Path, expectedPrefix)
	}

	if info.CommitSHA == "" {
		t.Error("WorktreeInfo.CommitSHA should not be empty")
	}

	// Verify worktree exists
	if _, err := os.Stat(info.Path); os.IsNotExist(err) {
		t.Errorf("Worktree directory should exist at %s", info.Path)
	}

	// Verify README exists in worktree
	readmePath := filepath.Join(info.Path, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		t.Error("README.md should exist in worktree")
	}

	// Test cleanup
	cleanupWorktree()

	// Verify worktree is removed
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Errorf("Worktree directory should be removed after cleanup at %s", info.Path)
	}
}

// TestValidateCleanWorktree_Clean tests validation passes on clean worktree
func TestValidateCleanWorktree_Clean(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx := context.Background()

	err := ValidateCleanWorktree(ctx, repoPath)
	if err != nil {
		t.Errorf("ValidateCleanWorktree() should pass for clean repo, got: %v", err)
	}
}

// TestValidateCleanWorktree_Modified tests validation fails on modified files
func TestValidateCleanWorktree_Modified(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Modify README
	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Modified\n"), 0o644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}

	ctx := context.Background()

	err := ValidateCleanWorktree(ctx, repoPath)
	if err == nil {
		t.Error("ValidateCleanWorktree() should fail for modified files")
	}

	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("Error should mention 'uncommitted changes', got: %v", err)
	}

	if !strings.Contains(err.Error(), "README.md") {
		t.Errorf("Error should list modified file, got: %v", err)
	}
}

// TestValidateCleanWorktree_Untracked tests validation fails on untracked files
func TestValidateCleanWorktree_Untracked(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create untracked file
	untrackedPath := filepath.Join(repoPath, "new-file.txt")
	if err := os.WriteFile(untrackedPath, []byte("new content"), 0o644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	ctx := context.Background()

	err := ValidateCleanWorktree(ctx, repoPath)
	if err == nil {
		t.Error("ValidateCleanWorktree() should fail for untracked files")
	}

	if !strings.Contains(err.Error(), "new-file.txt") {
		t.Errorf("Error should list untracked file, got: %v", err)
	}
}

// TestValidateCleanWorktree_Deleted tests validation fails on deleted files
func TestValidateCleanWorktree_Deleted(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Delete README
	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.Remove(readmePath); err != nil {
		t.Fatalf("Failed to delete README: %v", err)
	}

	ctx := context.Background()

	err := ValidateCleanWorktree(ctx, repoPath)
	if err == nil {
		t.Error("ValidateCleanWorktree() should fail for deleted files")
	}

	if !strings.Contains(err.Error(), "README.md") {
		t.Errorf("Error should list deleted file, got: %v", err)
	}
}

// TestValidateCleanWorktree_MultipleChanges tests validation fails with multiple changes
func TestValidateCleanWorktree_MultipleChanges(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Modify README
	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Modified\n"), 0o644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}

	// Create untracked file
	untrackedPath := filepath.Join(repoPath, "new-file.txt")
	if err := os.WriteFile(untrackedPath, []byte("new content"), 0o644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	ctx := context.Background()

	err := ValidateCleanWorktree(ctx, repoPath)
	if err == nil {
		t.Error("ValidateCleanWorktree() should fail for multiple changes")
	}

	// Should list both files
	errStr := err.Error()
	if !strings.Contains(errStr, "README.md") || !strings.Contains(errStr, "new-file.txt") {
		t.Errorf("Error should list all changed files, got: %v", err)
	}
}

// TestValidateCleanWorktree_NotGitRepo tests behavior in non-git directory
func TestValidateCleanWorktree_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	err := ValidateCleanWorktree(ctx, tmpDir)
	if err == nil {
		t.Error("ValidateCleanWorktree should fail in non-git directory")
	}

	if !strings.Contains(err.Error(), "failed to check git status") {
		t.Errorf("Error should mention git status failure, got: %v", err)
	}
}

// TestPrepareWorktree_ContextCancellation tests cleanup on context cancellation
func TestPrepareWorktree_ContextCancellation(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, cleanupWorktree, err := PrepareWorktree(ctx, repoPath, "")
	if err != nil {
		t.Fatalf("PrepareWorktree() failed: %v", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(info.Path); os.IsNotExist(err) {
		t.Fatal("Worktree should exist after PrepareWorktree")
	}

	// Cancel context
	cancel()

	// Cleanup should still work
	cleanupWorktree()

	// Verify worktree is removed
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Error("Worktree should be removed after cleanup even after context cancellation")
	}
}

// TestRemoveWorktree_NonexistentPath tests cleanup of nonexistent worktree
func TestRemoveWorktree_NonexistentPath(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	nonexistentPath := filepath.Join(os.TempDir(), "detent-nonexistent-xyz")
	ctx := context.Background()

	// Should not panic or fail fatally
	err := removeWorktree(ctx, repoPath, nonexistentPath)
	if err == nil {
		// Git might succeed (no-op) or fail - either is acceptable
		t.Log("removeWorktree succeeded on nonexistent path (expected)")
	} else {
		// Error is acceptable too
		t.Logf("removeWorktree failed on nonexistent path (expected): %v", err)
	}
}

// TestPrepareWorktree_ConcurrentCalls tests creating multiple worktrees
func TestPrepareWorktree_ConcurrentCalls(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx := context.Background()

	// Create two worktrees (empty string = temp directory with random suffix)
	info1, cleanup1, err := PrepareWorktree(ctx, repoPath, "")
	if err != nil {
		t.Fatalf("PrepareWorktree(1) failed: %v", err)
	}
	defer cleanup1()

	info2, cleanup2, err := PrepareWorktree(ctx, repoPath, "")
	if err != nil {
		t.Fatalf("PrepareWorktree(2) failed: %v", err)
	}
	defer cleanup2()

	// Verify both worktrees exist and are different
	if info1.Path == info2.Path {
		t.Error("Worktree paths should be different")
	}

	if _, err := os.Stat(info1.Path); os.IsNotExist(err) {
		t.Error("First worktree should exist")
	}

	if _, err := os.Stat(info2.Path); os.IsNotExist(err) {
		t.Error("Second worktree should exist")
	}

	// Cleanup both
	cleanup1()
	cleanup2()

	// Verify both are removed
	if _, err := os.Stat(info1.Path); !os.IsNotExist(err) {
		t.Error("First worktree should be removed")
	}

	if _, err := os.Stat(info2.Path); !os.IsNotExist(err) {
		t.Error("Second worktree should be removed")
	}
}

// TestPrepareWorktree_ErrorWrapping tests that errors are properly wrapped
func TestPrepareWorktree_ErrorWrapping(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	_, _, err := PrepareWorktree(ctx, tmpDir, "")
	if err == nil {
		t.Fatal("Expected error in non-git directory")
	}

	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "getting current commit SHA") {
		t.Errorf("Error should contain context message, got: %s", errorMsg)
	}
}
