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
	runID := "test-clean-123"

	info, cleanupWorktree, err := PrepareWorktree(ctx, repoPath, runID)
	if err != nil {
		t.Fatalf("PrepareWorktree() failed: %v", err)
	}
	defer cleanupWorktree()

	// Verify WorktreeInfo
	if info == nil {
		t.Fatal("WorktreeInfo should not be nil")
	}

	if info.Path != filepath.Join(os.TempDir(), "detent-"+runID) {
		t.Errorf("WorktreeInfo.Path = %s, want %s", info.Path, filepath.Join(os.TempDir(), "detent-"+runID))
	}

	if info.BaseCommitSHA == "" {
		t.Error("WorktreeInfo.BaseCommitSHA should not be empty")
	}

	if info.IsDirty {
		t.Error("WorktreeInfo.IsDirty should be false for clean repo")
	}

	if len(info.DirtyFiles) != 0 {
		t.Errorf("WorktreeInfo.DirtyFiles should be empty, got %d files", len(info.DirtyFiles))
	}

	if info.WorktreeCommit != info.BaseCommitSHA {
		t.Errorf("WorktreeCommit = %s, want %s (should match BaseCommitSHA for clean repo)", info.WorktreeCommit, info.BaseCommitSHA)
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

// TestPrepareWorktree_DirtyRepo tests worktree creation with uncommitted changes
func TestPrepareWorktree_DirtyRepo(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create dirty files
	modifiedFile := filepath.Join(repoPath, "modified.txt")
	if err := os.WriteFile(modifiedFile, []byte("modified content"), 0o644); err != nil {
		t.Fatalf("Failed to create modified file: %v", err)
	}

	untrackedFile := filepath.Join(repoPath, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked content"), 0o644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	ctx := context.Background()
	runID := "test-dirty-456"

	info, cleanupWorktree, err := PrepareWorktree(ctx, repoPath, runID)
	if err != nil {
		t.Fatalf("PrepareWorktree() failed: %v", err)
	}
	defer cleanupWorktree()

	// Verify WorktreeInfo
	if !info.IsDirty {
		t.Error("WorktreeInfo.IsDirty should be true for dirty repo")
	}

	if len(info.DirtyFiles) != 2 {
		t.Errorf("WorktreeInfo.DirtyFiles should have 2 files, got %d", len(info.DirtyFiles))
	}

	// Verify dirty files are copied to worktree
	worktreeModified := filepath.Join(info.Path, "modified.txt")
	if _, err := os.Stat(worktreeModified); os.IsNotExist(err) {
		t.Error("modified.txt should exist in worktree")
	}

	worktreeUntracked := filepath.Join(info.Path, "untracked.txt")
	if _, err := os.Stat(worktreeUntracked); os.IsNotExist(err) {
		t.Error("untracked.txt should exist in worktree")
	}

	// Verify content matches
	content, err := os.ReadFile(worktreeModified)
	if err != nil {
		t.Fatalf("Failed to read modified.txt from worktree: %v", err)
	}
	if string(content) != "modified content" {
		t.Errorf("modified.txt content = %s, want 'modified content'", string(content))
	}

	// Verify worktree commit is different from base commit (because we committed changes)
	if info.WorktreeCommit == info.BaseCommitSHA {
		t.Error("WorktreeCommit should differ from BaseCommitSHA when dirty files are committed")
	}
}

// TestDetectDirtyState_Clean tests detecting clean repository state
func TestDetectDirtyState_Clean(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	isDirty, dirtyFiles, err := detectDirtyState(repoPath)
	if err != nil {
		t.Fatalf("detectDirtyState() failed: %v", err)
	}

	if isDirty {
		t.Error("isDirty should be false for clean repo")
	}

	if len(dirtyFiles) != 0 {
		t.Errorf("dirtyFiles should be empty, got %d files", len(dirtyFiles))
	}
}

// TestDetectDirtyState_Modified tests detecting modified files
func TestDetectDirtyState_Modified(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Modify README
	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Modified\n"), 0o644); err != nil {
		t.Fatalf("Failed to modify README: %v", err)
	}

	isDirty, dirtyFiles, err := detectDirtyState(repoPath)
	if err != nil {
		t.Fatalf("detectDirtyState() failed: %v", err)
	}

	if !isDirty {
		t.Error("isDirty should be true for modified files")
	}

	if len(dirtyFiles) != 1 {
		t.Errorf("dirtyFiles should have 1 file, got %d", len(dirtyFiles))
	}

	if len(dirtyFiles) > 0 && dirtyFiles[0] != "README.md" {
		t.Errorf("dirtyFiles[0] = %s, want README.md", dirtyFiles[0])
	}
}

// TestDetectDirtyState_Untracked tests detecting untracked files
func TestDetectDirtyState_Untracked(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create untracked file
	untrackedPath := filepath.Join(repoPath, "new-file.txt")
	if err := os.WriteFile(untrackedPath, []byte("new content"), 0o644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	isDirty, dirtyFiles, err := detectDirtyState(repoPath)
	if err != nil {
		t.Fatalf("detectDirtyState() failed: %v", err)
	}

	if !isDirty {
		t.Error("isDirty should be true for untracked files")
	}

	if len(dirtyFiles) != 1 {
		t.Errorf("dirtyFiles should have 1 file, got %d", len(dirtyFiles))
	}

	if len(dirtyFiles) > 0 && dirtyFiles[0] != "new-file.txt" {
		t.Errorf("dirtyFiles[0] = %s, want new-file.txt", dirtyFiles[0])
	}
}

// TestDetectDirtyState_Deleted tests detecting deleted files
func TestDetectDirtyState_Deleted(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Delete README
	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.Remove(readmePath); err != nil {
		t.Fatalf("Failed to delete README: %v", err)
	}

	isDirty, dirtyFiles, err := detectDirtyState(repoPath)
	if err != nil {
		t.Fatalf("detectDirtyState() failed: %v", err)
	}

	if !isDirty {
		t.Error("isDirty should be true for deleted files")
	}

	if len(dirtyFiles) != 1 {
		t.Errorf("dirtyFiles should have 1 file, got %d", len(dirtyFiles))
	}

	if len(dirtyFiles) > 0 && dirtyFiles[0] != "README.md" {
		t.Errorf("dirtyFiles[0] = %s, want README.md", dirtyFiles[0])
	}
}

// TestDetectDirtyState_MultipleChanges tests detecting multiple types of changes
func TestDetectDirtyState_MultipleChanges(t *testing.T) {
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

	// Create nested directory with file
	nestedDir := filepath.Join(repoPath, "src", "utils")
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}
	nestedFile := filepath.Join(nestedDir, "helper.go")
	if err := os.WriteFile(nestedFile, []byte("package utils"), 0o644); err != nil {
		t.Fatalf("Failed to create nested file: %v", err)
	}

	isDirty, dirtyFiles, err := detectDirtyState(repoPath)
	if err != nil {
		t.Fatalf("detectDirtyState() failed: %v", err)
	}

	if !isDirty {
		t.Error("isDirty should be true for multiple changes")
	}

	if len(dirtyFiles) != 3 {
		t.Errorf("dirtyFiles should have 3 files, got %d: %v", len(dirtyFiles), dirtyFiles)
	}
}

// TestPrepareWorktree_ContextCancellation tests cleanup on context cancellation
func TestPrepareWorktree_ContextCancellation(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runID := "test-cancel-789"

	info, cleanupWorktree, err := PrepareWorktree(ctx, repoPath, runID)
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

// TestCopyDirtyFiles_NestedDirectories tests copying files in nested directories
func TestCopyDirtyFiles_NestedDirectories(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create nested structure
	nestedDir := filepath.Join(repoPath, "src", "internal", "utils")
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatalf("Failed to create nested directory: %v", err)
	}

	nestedFile := filepath.Join(nestedDir, "helper.go")
	testContent := "package utils\n\nfunc Helper() {}\n"
	if err := os.WriteFile(nestedFile, []byte(testContent), 0o644); err != nil {
		t.Fatalf("Failed to create nested file: %v", err)
	}

	ctx := context.Background()
	runID := "test-nested-abc"

	info, cleanupWorktree, err := PrepareWorktree(ctx, repoPath, runID)
	if err != nil {
		t.Fatalf("PrepareWorktree() failed: %v", err)
	}
	defer cleanupWorktree()

	// Verify nested file exists in worktree
	worktreeNested := filepath.Join(info.Path, "src", "internal", "utils", "helper.go")
	content, err := os.ReadFile(worktreeNested)
	if err != nil {
		t.Fatalf("Failed to read nested file from worktree: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Nested file content = %s, want %s", string(content), testContent)
	}
}

// TestRemoveWorktree_NonexistentPath tests cleanup of nonexistent worktree
func TestRemoveWorktree_NonexistentPath(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	nonexistentPath := filepath.Join(os.TempDir(), "detent-nonexistent-xyz")

	// Should not panic or fail fatally
	err := removeWorktree(repoPath, nonexistentPath)
	if err == nil {
		// Git might succeed (no-op) or fail - either is acceptable
		t.Log("removeWorktree succeeded on nonexistent path (expected)")
	} else {
		// Error is acceptable too
		t.Logf("removeWorktree failed on nonexistent path (expected): %v", err)
	}
}

// TestPrepareWorktree_ConcurrentCalls tests creating multiple worktrees with different run IDs
func TestPrepareWorktree_ConcurrentCalls(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx := context.Background()

	// Create two worktrees with different run IDs
	runID1 := "concurrent-1"
	runID2 := "concurrent-2"

	info1, cleanup1, err := PrepareWorktree(ctx, repoPath, runID1)
	if err != nil {
		t.Fatalf("PrepareWorktree(runID1) failed: %v", err)
	}
	defer cleanup1()

	info2, cleanup2, err := PrepareWorktree(ctx, repoPath, runID2)
	if err != nil {
		t.Fatalf("PrepareWorktree(runID2) failed: %v", err)
	}
	defer cleanup2()

	// Verify both worktrees exist and are different
	if info1.Path == info2.Path {
		t.Error("Worktree paths should be different for different run IDs")
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

// TestCommitDirtyChanges_EmptyChanges tests committing when there are no changes
func TestCommitDirtyChanges_EmptyChanges(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx := context.Background()

	// Try to commit without any changes (should fail)
	_, err := commitDirtyChanges(ctx, repoPath)
	if err == nil {
		t.Error("commitDirtyChanges should fail when there are no changes to commit")
	}

	if !strings.Contains(err.Error(), "git commit failed") {
		t.Errorf("Error should mention 'git commit failed', got: %v", err)
	}
}

// TestDetectDirtyState_NotGitRepo tests behavior in non-git directory
func TestDetectDirtyState_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	_, _, err := detectDirtyState(tmpDir)
	if err == nil {
		t.Error("detectDirtyState should fail in non-git directory")
	}

	if !strings.Contains(err.Error(), "git") {
		t.Errorf("Error should mention 'git', got: %v", err)
	}
}

// TestPrepareWorktree_ErrorWrapping tests that errors are properly wrapped
func TestPrepareWorktree_ErrorWrapping(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	_, _, err := PrepareWorktree(ctx, tmpDir, "test-error")
	if err == nil {
		t.Fatal("Expected error in non-git directory")
	}

	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "getting current commit SHA") {
		t.Errorf("Error should contain context message, got: %s", errorMsg)
	}
}
