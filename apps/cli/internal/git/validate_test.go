package git

import (
	"context"
	"os/exec"
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
