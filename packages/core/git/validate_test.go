package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateGitRepository_ValidRepo tests validation passes on valid git repository
func TestValidateGitRepository_ValidRepo(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx := context.Background()

	err := ValidateGitRepository(ctx, repoPath)
	if err != nil {
		t.Errorf("ValidateGitRepository() should pass for valid git repo, got: %v", err)
	}
}

// TestValidateGitRepository_NotGitRepo tests validation fails on non-git directory
func TestValidateGitRepository_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	err := ValidateGitRepository(ctx, tmpDir)
	if err == nil {
		t.Error("ValidateGitRepository() should fail for non-git directory")
	}

	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("Error should mention 'not a git repository', got: %v", err)
	}

	if !strings.Contains(err.Error(), tmpDir) {
		t.Errorf("Error should include the path, got: %v", err)
	}
}

// TestValidateGitRepository_Subdir tests validation passes on subdirectory of git repo
func TestValidateGitRepository_Subdir(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a subdirectory
	subdirPath := repoPath + "/subdir"
	if err := exec.Command("mkdir", "-p", subdirPath).Run(); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	ctx := context.Background()

	err := ValidateGitRepository(ctx, subdirPath)
	if err != nil {
		t.Errorf("ValidateGitRepository() should pass for subdirectory of git repo, got: %v", err)
	}
}

// TestValidateGitRepository_NonexistentPath tests validation fails on nonexistent path
func TestValidateGitRepository_NonexistentPath(t *testing.T) {
	ctx := context.Background()

	err := ValidateGitRepository(ctx, "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("ValidateGitRepository() should fail for nonexistent path")
	}

	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("Error should mention 'not a git repository', got: %v", err)
	}
}

// TestValidateGitRepository_ContextCancellation tests validation respects context cancellation
func TestValidateGitRepository_ContextCancellation(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := ValidateGitRepository(ctx, repoPath)
	if err == nil {
		t.Error("ValidateGitRepository() should fail with cancelled context")
	}
}

// TestValidateNoSubmodules_NoSubmodules tests validation passes when no submodules exist
func TestValidateNoSubmodules_NoSubmodules(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	err := ValidateNoSubmodules(repoPath)
	if err != nil {
		t.Errorf("ValidateNoSubmodules() should pass when no .gitmodules exists, got: %v", err)
	}
}

// TestValidateNoSubmodules_WithSubmodules tests validation fails when submodules exist
func TestValidateNoSubmodules_WithSubmodules(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a .gitmodules file to simulate submodules
	gitmodulesPath := filepath.Join(repoPath, ".gitmodules")
	if err := os.WriteFile(gitmodulesPath, []byte("[submodule \"test\"]"), 0o644); err != nil {
		t.Fatalf("Failed to create .gitmodules: %v", err)
	}

	err := ValidateNoSubmodules(repoPath)
	if err == nil {
		t.Error("ValidateNoSubmodules() should fail when .gitmodules exists")
	}

	if !strings.Contains(err.Error(), "submodules") {
		t.Errorf("Error should mention 'submodules', got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_NoSymlinks tests validation passes with no symlinks
func TestValidateNoEscapingSymlinks_NoSymlinks(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err != nil {
		t.Errorf("ValidateNoEscapingSymlinks() should pass with no symlinks, got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_InternalSymlink tests validation passes with internal symlink
func TestValidateNoEscapingSymlinks_InternalSymlink(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create an internal symlink (README.md -> link.md)
	linkPath := filepath.Join(repoPath, "link.md")
	targetPath := filepath.Join(repoPath, "README.md")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err != nil {
		t.Errorf("ValidateNoEscapingSymlinks() should pass with internal symlink, got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_EscapingSymlink tests validation fails with escaping symlink
func TestValidateNoEscapingSymlinks_EscapingSymlink(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a symlink pointing outside the repo
	linkPath := filepath.Join(repoPath, "escape.txt")
	if err := os.Symlink("/etc/passwd", linkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err == nil {
		t.Error("ValidateNoEscapingSymlinks() should fail with escaping symlink")
	}

	if !strings.Contains(err.Error(), "escapes repository") {
		t.Errorf("Error should mention 'escapes repository', got: %v", err)
	}

	if !strings.Contains(err.Error(), "/etc/passwd") {
		t.Errorf("Error should mention the target path, got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_BrokenSymlink tests validation passes with broken symlink
func TestValidateNoEscapingSymlinks_BrokenSymlink(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a broken symlink (target doesn't exist)
	linkPath := filepath.Join(repoPath, "broken.txt")
	if err := os.Symlink(filepath.Join(repoPath, "nonexistent.txt"), linkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err != nil {
		t.Errorf("ValidateNoEscapingSymlinks() should pass with broken symlink (will fail naturally), got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_SkipsGitDir tests validation skips .git directory
func TestValidateNoEscapingSymlinks_SkipsGitDir(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Git internally uses symlinks in some cases, they should be skipped
	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err != nil {
		t.Errorf("ValidateNoEscapingSymlinks() should skip .git directory, got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_DepthLimit tests validation fails when depth limit exceeded
func TestValidateNoEscapingSymlinks_DepthLimit(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a deeply nested directory structure (exceeding maxSymlinkDepth = 100)
	deepPath := repoPath
	for i := 0; i <= 101; i++ {
		deepPath = filepath.Join(deepPath, "dir")
	}
	if err := os.MkdirAll(deepPath, 0o755); err != nil {
		t.Fatalf("Failed to create deep directory: %v", err)
	}

	// Create a file in the deep directory
	testFile := filepath.Join(deepPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err == nil {
		t.Error("ValidateNoEscapingSymlinks() should fail when depth limit exceeded")
	}

	if !strings.Contains(err.Error(), "symlink validation limit exceeded") {
		t.Errorf("Error should mention 'symlink validation limit exceeded', got: %v", err)
	}

	if !strings.Contains(err.Error(), "maximum traversal depth") {
		t.Errorf("Error should mention 'maximum traversal depth', got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_SymlinkCountLimit tests validation fails when symlink count exceeded
func TestValidateNoEscapingSymlinks_SymlinkCountLimit(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create more symlinks than maxSymlinksChecked (10000)
	// Create 10001 symlinks to exceed the limit
	targetFile := filepath.Join(repoPath, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target"), 0o644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	for i := 0; i <= 10001; i++ {
		linkPath := filepath.Join(repoPath, "links", filepath.Join("dir", strings.Repeat("0", 10-len(fmt.Sprintf("%d", i))))+fmt.Sprintf("%d.txt", i))
		linkDir := filepath.Dir(linkPath)
		if err := os.MkdirAll(linkDir, 0o755); err != nil {
			t.Fatalf("Failed to create link directory: %v", err)
		}
		if err := os.Symlink(targetFile, linkPath); err != nil {
			t.Fatalf("Failed to create symlink %d: %v", i, err)
		}
	}

	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err == nil {
		t.Error("ValidateNoEscapingSymlinks() should fail when symlink count limit exceeded")
	}

	if !strings.Contains(err.Error(), "symlink validation limit exceeded") {
		t.Errorf("Error should mention 'symlink validation limit exceeded', got: %v", err)
	}

	if !strings.Contains(err.Error(), "maximum symlink count") {
		t.Errorf("Error should mention 'maximum symlink count', got: %v", err)
	}
}

// TestValidateNoEscapingSymlinks_WithinLimits tests validation passes when within limits
func TestValidateNoEscapingSymlinks_WithinLimits(t *testing.T) {
	repoPath, cleanup := setupGitRepo(t)
	defer cleanup()

	// Create a moderately deep directory structure (depth = 50)
	deepPath := repoPath
	for i := 0; i < 50; i++ {
		deepPath = filepath.Join(deepPath, "dir")
	}
	if err := os.MkdirAll(deepPath, 0o755); err != nil {
		t.Fatalf("Failed to create deep directory: %v", err)
	}

	// Create 100 internal symlinks (well under the limit)
	targetFile := filepath.Join(repoPath, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target"), 0o644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	for i := 0; i < 100; i++ {
		linkPath := filepath.Join(deepPath, fmt.Sprintf("link%d.txt", i))
		if err := os.Symlink(targetFile, linkPath); err != nil {
			t.Fatalf("Failed to create symlink %d: %v", i, err)
		}
	}

	ctx := context.Background()
	err := ValidateNoEscapingSymlinks(ctx, repoPath)
	if err != nil {
		t.Errorf("ValidateNoEscapingSymlinks() should pass when within limits, got: %v", err)
	}
}
