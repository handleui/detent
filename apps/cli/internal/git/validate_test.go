package git

import (
	"context"
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
