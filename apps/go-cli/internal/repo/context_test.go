package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupTestRepo creates a temporary git repository with an initial commit.
// Returns the repository path. The temp directory is automatically cleaned up
// by t.TempDir() after the test completes.
func setupTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize git repo
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	// Configure git user
	if err := exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("Failed to configure git email: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("Failed to configure git name: %v", err)
	}

	// Create a file and commit
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := exec.Command("git", "-C", dir, "add", "test.txt").Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	if err := exec.Command("git", "-C", dir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	return dir
}

// chdir changes to the specified directory and returns a cleanup function
// that restores the original directory.
func chdir(t *testing.T, dir string) func() {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	return func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("Failed to restore directory: %v", err)
		}
	}
}

// isValidSHA checks if a string is a valid 40-character hex SHA
func isValidSHA(sha string) bool {
	if len(sha) != 40 {
		return false
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// TestResolve_NoOptions tests Resolve() with no options - only Path is set
func TestResolve_NoOptions(t *testing.T) {
	dir := setupTestRepo(t)
	cleanup := chdir(t, dir)
	defer cleanup()

	ctx, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}

	// Path should be set
	if ctx.Path == "" {
		t.Error("Path should not be empty")
	}

	// Path should be absolute
	if !filepath.IsAbs(ctx.Path) {
		t.Errorf("Path should be absolute, got: %s", ctx.Path)
	}

	// Path should match the test directory (resolve symlinks for macOS /var -> /private/var)
	expectedPath, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks: %v", err)
	}
	resolvedCtxPath, err := filepath.EvalSymlinks(ctx.Path)
	if err != nil {
		t.Fatalf("Failed to evaluate symlinks for ctx.Path: %v", err)
	}
	if resolvedCtxPath != expectedPath {
		t.Errorf("Path = %s, want %s", resolvedCtxPath, expectedPath)
	}

	// Other fields should be empty (not resolved)
	if ctx.FirstCommitSHA != "" {
		t.Errorf("FirstCommitSHA should be empty, got: %s", ctx.FirstCommitSHA)
	}
	if ctx.RunID != "" {
		t.Errorf("RunID should be empty, got: %s", ctx.RunID)
	}
	if ctx.TreeHash != "" {
		t.Errorf("TreeHash should be empty, got: %s", ctx.TreeHash)
	}
	if ctx.CommitSHA != "" {
		t.Errorf("CommitSHA should be empty, got: %s", ctx.CommitSHA)
	}
}

// TestResolve_WithFirstCommit tests Resolve(WithFirstCommit()) - Path and FirstCommitSHA set
func TestResolve_WithFirstCommit(t *testing.T) {
	dir := setupTestRepo(t)
	cleanup := chdir(t, dir)
	defer cleanup()

	ctx, err := Resolve(WithFirstCommit())
	if err != nil {
		t.Fatalf("Resolve(WithFirstCommit()) failed: %v", err)
	}

	// Path should be set
	if ctx.Path == "" {
		t.Error("Path should not be empty")
	}

	// FirstCommitSHA should be set
	if ctx.FirstCommitSHA == "" {
		t.Error("FirstCommitSHA should not be empty")
	}

	// FirstCommitSHA should be a valid SHA
	if !isValidSHA(ctx.FirstCommitSHA) {
		t.Errorf("FirstCommitSHA is not a valid SHA: %s", ctx.FirstCommitSHA)
	}

	// Other fields should be empty (not resolved)
	if ctx.RunID != "" {
		t.Errorf("RunID should be empty, got: %s", ctx.RunID)
	}
	if ctx.TreeHash != "" {
		t.Errorf("TreeHash should be empty, got: %s", ctx.TreeHash)
	}
	if ctx.CommitSHA != "" {
		t.Errorf("CommitSHA should be empty, got: %s", ctx.CommitSHA)
	}
}

// TestResolve_WithRunID tests Resolve(WithRunID()) - Path, RunID, TreeHash, CommitSHA set
func TestResolve_WithRunID(t *testing.T) {
	dir := setupTestRepo(t)
	cleanup := chdir(t, dir)
	defer cleanup()

	ctx, err := Resolve(WithRunID())
	if err != nil {
		t.Fatalf("Resolve(WithRunID()) failed: %v", err)
	}

	// Path should be set
	if ctx.Path == "" {
		t.Error("Path should not be empty")
	}

	// RunID should be set (16 hex characters)
	if ctx.RunID == "" {
		t.Error("RunID should not be empty")
	}
	if len(ctx.RunID) != 16 {
		t.Errorf("RunID length = %d, want 16", len(ctx.RunID))
	}

	// TreeHash should be set (40 hex characters)
	if ctx.TreeHash == "" {
		t.Error("TreeHash should not be empty")
	}
	if !isValidSHA(ctx.TreeHash) {
		t.Errorf("TreeHash is not a valid SHA: %s", ctx.TreeHash)
	}

	// CommitSHA should be set (40 hex characters)
	if ctx.CommitSHA == "" {
		t.Error("CommitSHA should not be empty")
	}
	if !isValidSHA(ctx.CommitSHA) {
		t.Errorf("CommitSHA is not a valid SHA: %s", ctx.CommitSHA)
	}

	// FirstCommitSHA should be empty (not resolved)
	if ctx.FirstCommitSHA != "" {
		t.Errorf("FirstCommitSHA should be empty, got: %s", ctx.FirstCommitSHA)
	}
}

// TestResolve_WithAll tests Resolve(WithAll()) - all fields set
func TestResolve_WithAll(t *testing.T) {
	dir := setupTestRepo(t)
	cleanup := chdir(t, dir)
	defer cleanup()

	ctx, err := Resolve(WithAll())
	if err != nil {
		t.Fatalf("Resolve(WithAll()) failed: %v", err)
	}

	// Path should be set
	if ctx.Path == "" {
		t.Error("Path should not be empty")
	}

	// FirstCommitSHA should be set
	if ctx.FirstCommitSHA == "" {
		t.Error("FirstCommitSHA should not be empty")
	}
	if !isValidSHA(ctx.FirstCommitSHA) {
		t.Errorf("FirstCommitSHA is not a valid SHA: %s", ctx.FirstCommitSHA)
	}

	// RunID should be set (16 hex characters)
	if ctx.RunID == "" {
		t.Error("RunID should not be empty")
	}
	if len(ctx.RunID) != 16 {
		t.Errorf("RunID length = %d, want 16", len(ctx.RunID))
	}

	// TreeHash should be set (40 hex characters)
	if ctx.TreeHash == "" {
		t.Error("TreeHash should not be empty")
	}
	if !isValidSHA(ctx.TreeHash) {
		t.Errorf("TreeHash is not a valid SHA: %s", ctx.TreeHash)
	}

	// CommitSHA should be set (40 hex characters)
	if ctx.CommitSHA == "" {
		t.Error("CommitSHA should not be empty")
	}
	if !isValidSHA(ctx.CommitSHA) {
		t.Errorf("CommitSHA is not a valid SHA: %s", ctx.CommitSHA)
	}
}

// TestResolve_WithAll_Consistency tests that calling Resolve(WithAll()) twice returns consistent results
func TestResolve_WithAll_Consistency(t *testing.T) {
	dir := setupTestRepo(t)
	cleanup := chdir(t, dir)
	defer cleanup()

	ctx1, err := Resolve(WithAll())
	if err != nil {
		t.Fatalf("First Resolve(WithAll()) failed: %v", err)
	}

	ctx2, err := Resolve(WithAll())
	if err != nil {
		t.Fatalf("Second Resolve(WithAll()) failed: %v", err)
	}

	if ctx1.Path != ctx2.Path {
		t.Errorf("Path mismatch: %s vs %s", ctx1.Path, ctx2.Path)
	}
	if ctx1.FirstCommitSHA != ctx2.FirstCommitSHA {
		t.Errorf("FirstCommitSHA mismatch: %s vs %s", ctx1.FirstCommitSHA, ctx2.FirstCommitSHA)
	}
	if ctx1.RunID != ctx2.RunID {
		t.Errorf("RunID mismatch: %s vs %s", ctx1.RunID, ctx2.RunID)
	}
	if ctx1.TreeHash != ctx2.TreeHash {
		t.Errorf("TreeHash mismatch: %s vs %s", ctx1.TreeHash, ctx2.TreeHash)
	}
	if ctx1.CommitSHA != ctx2.CommitSHA {
		t.Errorf("CommitSHA mismatch: %s vs %s", ctx1.CommitSHA, ctx2.CommitSHA)
	}
}

// TestResolve_CombinedOptions tests combining individual options
func TestResolve_CombinedOptions(t *testing.T) {
	dir := setupTestRepo(t)
	cleanup := chdir(t, dir)
	defer cleanup()

	// Using both WithFirstCommit() and WithRunID() should be equivalent to WithAll()
	ctx, err := Resolve(WithFirstCommit(), WithRunID())
	if err != nil {
		t.Fatalf("Resolve(WithFirstCommit(), WithRunID()) failed: %v", err)
	}

	// All fields should be set
	if ctx.Path == "" {
		t.Error("Path should not be empty")
	}
	if ctx.FirstCommitSHA == "" {
		t.Error("FirstCommitSHA should not be empty")
	}
	if ctx.RunID == "" {
		t.Error("RunID should not be empty")
	}
	if ctx.TreeHash == "" {
		t.Error("TreeHash should not be empty")
	}
	if ctx.CommitSHA == "" {
		t.Error("CommitSHA should not be empty")
	}

	// Compare with WithAll() result
	ctxAll, err := Resolve(WithAll())
	if err != nil {
		t.Fatalf("Resolve(WithAll()) failed: %v", err)
	}

	if ctx.FirstCommitSHA != ctxAll.FirstCommitSHA {
		t.Errorf("FirstCommitSHA mismatch with WithAll(): %s vs %s", ctx.FirstCommitSHA, ctxAll.FirstCommitSHA)
	}
	if ctx.RunID != ctxAll.RunID {
		t.Errorf("RunID mismatch with WithAll(): %s vs %s", ctx.RunID, ctxAll.RunID)
	}
}

// TestResolve_NonGitRepo tests that Resolve fails appropriately in a non-git directory
func TestResolve_NonGitRepo_WithFirstCommit(t *testing.T) {
	dir := t.TempDir()
	cleanup := chdir(t, dir)
	defer cleanup()

	// Resolve without options should still work (just returns path)
	ctx, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() should succeed in non-git directory: %v", err)
	}
	if ctx.Path == "" {
		t.Error("Path should not be empty")
	}

	// Resolve with WithFirstCommit should fail
	_, err = Resolve(WithFirstCommit())
	if err == nil {
		t.Error("Resolve(WithFirstCommit()) should fail in non-git directory")
	}
}

// TestResolve_NonGitRepo_WithRunID tests that Resolve with WithRunID fails in non-git directory
func TestResolve_NonGitRepo_WithRunID(t *testing.T) {
	dir := t.TempDir()
	cleanup := chdir(t, dir)
	defer cleanup()

	_, err := Resolve(WithRunID())
	if err == nil {
		t.Error("Resolve(WithRunID()) should fail in non-git directory")
	}
}

// TestResolve_EmptyRepo tests that Resolve fails appropriately in an empty git repository
func TestResolve_EmptyRepo_WithFirstCommit(t *testing.T) {
	dir := t.TempDir()

	// Initialize git repo without any commits
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	cleanup := chdir(t, dir)
	defer cleanup()

	// Resolve with WithFirstCommit should fail (no commits)
	_, err := Resolve(WithFirstCommit())
	if err == nil {
		t.Error("Resolve(WithFirstCommit()) should fail in empty git repository")
	}
}

// TestResolve_EmptyRepo_WithRunID tests that Resolve with WithRunID fails in empty git repository
func TestResolve_EmptyRepo_WithRunID(t *testing.T) {
	dir := t.TempDir()

	// Initialize git repo without any commits
	if err := exec.Command("git", "-C", dir, "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	cleanup := chdir(t, dir)
	defer cleanup()

	// Resolve with WithRunID should fail (no commits)
	_, err := Resolve(WithRunID())
	if err == nil {
		t.Error("Resolve(WithRunID()) should fail in empty git repository")
	}
}

// TestResolve_MultipleCommits tests that RunID changes when a new commit is made
func TestResolve_MultipleCommits(t *testing.T) {
	dir := setupTestRepo(t)
	cleanup := chdir(t, dir)
	defer cleanup()

	// Get initial context
	ctx1, err := Resolve(WithAll())
	if err != nil {
		t.Fatalf("First Resolve(WithAll()) failed: %v", err)
	}

	// Create a second commit
	testFile := filepath.Join(dir, "test2.txt")
	if err := os.WriteFile(testFile, []byte("more content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := exec.Command("git", "-C", dir, "add", "test2.txt").Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	if err := exec.Command("git", "-C", dir, "commit", "-m", "Second commit").Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Get new context
	ctx2, err := Resolve(WithAll())
	if err != nil {
		t.Fatalf("Second Resolve(WithAll()) failed: %v", err)
	}

	// Path and FirstCommitSHA should be the same
	if ctx1.Path != ctx2.Path {
		t.Errorf("Path should not change: %s vs %s", ctx1.Path, ctx2.Path)
	}
	if ctx1.FirstCommitSHA != ctx2.FirstCommitSHA {
		t.Errorf("FirstCommitSHA should not change: %s vs %s", ctx1.FirstCommitSHA, ctx2.FirstCommitSHA)
	}

	// CommitSHA, TreeHash, and RunID should be different
	if ctx1.CommitSHA == ctx2.CommitSHA {
		t.Error("CommitSHA should change after new commit")
	}
	if ctx1.TreeHash == ctx2.TreeHash {
		t.Error("TreeHash should change after new commit")
	}
	if ctx1.RunID == ctx2.RunID {
		t.Error("RunID should change after new commit")
	}
}
