package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetCurrentCommitSHA tests getting the current commit SHA
func TestGetCurrentCommitSHA(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() (cleanup func(), err error)
		wantErr   bool
		validate  func(string) error
	}{
		{
			name: "valid git repository",
			setupFunc: func() (cleanup func(), err error) {
				// Create a temporary git repository
				tmpDir := t.TempDir()
				originalDir, err := os.Getwd()
				if err != nil {
					return nil, err
				}

				if err := os.Chdir(tmpDir); err != nil {
					return nil, err
				}

				// Initialize git repo
				if err := exec.Command("git", "init").Run(); err != nil {
					_ = os.Chdir(originalDir)
					return nil, err
				}

				// Configure git
				if err := exec.Command("git", "config", "user.email", "test@example.com").Run(); err != nil {
					_ = os.Chdir(originalDir)
					return nil, err
				}
				if err := exec.Command("git", "config", "user.name", "Test User").Run(); err != nil {
					_ = os.Chdir(originalDir)
					return nil, err
				}

				// Create a file and commit
				testFile := filepath.Join(tmpDir, "test.txt")
				if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
					_ = os.Chdir(originalDir)
					return nil, err
				}

				if err := exec.Command("git", "add", "test.txt").Run(); err != nil {
					_ = os.Chdir(originalDir)
					return nil, err
				}

				if err := exec.Command("git", "commit", "-m", "Initial commit").Run(); err != nil {
					_ = os.Chdir(originalDir)
					return nil, err
				}

				cleanup = func() {
					_ = os.Chdir(originalDir)
				}

				return cleanup, nil
			},
			wantErr: false,
			validate: func(sha string) error {
				// SHA should be 40 characters (full SHA-1 hash)
				if len(sha) != 40 {
					t.Errorf("SHA length = %d, want 40", len(sha))
				}
				// SHA should only contain hex characters
				for _, c := range sha {
					if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
						t.Errorf("SHA contains non-hex character: %c", c)
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup, err := tt.setupFunc()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}
			if cleanup != nil {
				defer cleanup()
			}

			sha, err := GetCurrentCommitSHA()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCurrentCommitSHA() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if sha == "" {
					t.Error("SHA should not be empty")
				}

				// No leading/trailing whitespace
				if strings.TrimSpace(sha) != sha {
					t.Error("SHA should not have leading/trailing whitespace")
				}

				if tt.validate != nil {
					if err := tt.validate(sha); err != nil {
						t.Errorf("Validation failed: %v", err)
					}
				}
			}
		})
	}
}

// TestGetCurrentCommitSHA_NotGitRepo tests behavior in a non-git directory
func TestGetCurrentCommitSHA_NotGitRepo(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	_, err = GetCurrentCommitSHA()
	if err == nil {
		t.Error("GetCurrentCommitSHA() should fail in non-git directory")
	}

	// Verify error message mentions git
	if err != nil && !strings.Contains(err.Error(), "git") {
		t.Errorf("Error should mention 'git', got: %v", err)
	}
}

// TestGetCurrentCommitSHA_EmptyRepo tests behavior in an empty git repository
func TestGetCurrentCommitSHA_EmptyRepo(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Initialize empty git repo
	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	_, err = GetCurrentCommitSHA()
	if err == nil {
		t.Error("GetCurrentCommitSHA() should fail in empty git repository")
	}
}

// TestGetCurrentCommitSHA_GitNotInstalled tests behavior when git is not available
func TestGetCurrentCommitSHA_GitNotInstalled(t *testing.T) {
	// This test is tricky because we can't actually remove git from the system
	// Instead, we'll test that the error is properly wrapped
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	_, err = GetCurrentCommitSHA()
	if err == nil {
		t.Skip("Skipping test: requires non-git directory and git command to fail")
	}

	// Verify error is wrapped properly
	if !strings.Contains(err.Error(), "failed to get git commit SHA") {
		t.Errorf("Error should be wrapped with context, got: %v", err)
	}
}

// TestGetCurrentCommitSHA_Consistency tests that calling the function twice returns the same SHA
func TestGetCurrentCommitSHA_Consistency(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Initialize git repo with a commit
	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	if err := exec.Command("git", "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("Failed to configure git email: %v", err)
	}
	if err := exec.Command("git", "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("Failed to configure git name: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := exec.Command("git", "add", "test.txt").Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	if err := exec.Command("git", "commit", "-m", "Test commit").Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Call twice and verify consistency
	sha1, err := GetCurrentCommitSHA()
	if err != nil {
		t.Fatalf("First GetCurrentCommitSHA() failed: %v", err)
	}

	sha2, err := GetCurrentCommitSHA()
	if err != nil {
		t.Fatalf("Second GetCurrentCommitSHA() failed: %v", err)
	}

	if sha1 != sha2 {
		t.Errorf("GetCurrentCommitSHA() returned different SHAs: %s vs %s", sha1, sha2)
	}
}

// TestGetCurrentCommitSHA_DetachedHead tests behavior in detached HEAD state
func TestGetCurrentCommitSHA_DetachedHead(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Initialize git repo
	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	if err := exec.Command("git", "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("Failed to configure git email: %v", err)
	}
	if err := exec.Command("git", "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("Failed to configure git name: %v", err)
	}

	// Create and commit a file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := exec.Command("git", "add", "test.txt").Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	if err := exec.Command("git", "commit", "-m", "Test commit").Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Get the commit SHA
	sha, err := GetCurrentCommitSHA()
	if err != nil {
		t.Fatalf("GetCurrentCommitSHA() failed: %v", err)
	}

	// Checkout the commit directly (detached HEAD)
	if err := exec.Command("git", "checkout", sha).Run(); err != nil {
		t.Fatalf("Failed to checkout commit: %v", err)
	}

	// Should still work in detached HEAD state
	detachedSHA, err := GetCurrentCommitSHA()
	if err != nil {
		t.Errorf("GetCurrentCommitSHA() should work in detached HEAD state, got error: %v", err)
	}

	if detachedSHA != sha {
		t.Errorf("SHA in detached HEAD = %s, want %s", detachedSHA, sha)
	}
}

// TestGetCurrentCommitSHA_MultipleCommits tests that the function returns the latest commit
func TestGetCurrentCommitSHA_MultipleCommits(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Initialize git repo
	if err := exec.Command("git", "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	if err := exec.Command("git", "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("Failed to configure git email: %v", err)
	}
	if err := exec.Command("git", "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("Failed to configure git name: %v", err)
	}

	// Create multiple commits
	for i := 1; i <= 3; i++ {
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"+string(rune(i))), 0o644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := exec.Command("git", "add", "test.txt").Run(); err != nil {
			t.Fatalf("Failed to add file: %v", err)
		}

		if err := exec.Command("git", "commit", "-m", "Commit "+string(rune(i))).Run(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	// Get the current commit SHA
	sha, err := GetCurrentCommitSHA()
	if err != nil {
		t.Fatalf("GetCurrentCommitSHA() failed: %v", err)
	}

	// Verify it matches the latest commit
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get HEAD SHA: %v", err)
	}

	expectedSHA := strings.TrimSpace(string(output))
	if sha != expectedSHA {
		t.Errorf("GetCurrentCommitSHA() = %s, want %s", sha, expectedSHA)
	}
}

// TestGetCurrentCommitSHA_ErrorWrapping tests that errors are properly wrapped
func TestGetCurrentCommitSHA_ErrorWrapping(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	_, err = GetCurrentCommitSHA()
	if err == nil {
		t.Fatal("Expected error in non-git directory")
	}

	// Verify error is wrapped with context
	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "failed to get git commit SHA") {
		t.Errorf("Error should contain context message, got: %s", errorMsg)
	}
}

// TestGetCurrentRefs tests getting both commit SHA and tree hash in one call
func TestGetCurrentRefs(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo with a commit
	if err := exec.Command("git", "-C", tmpDir, "init").Run(); err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("Failed to configure git email: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("Failed to configure git name: %v", err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "add", "test.txt").Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}
	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Test commit").Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Get refs using the combined function
	refs, err := GetCurrentRefs(tmpDir)
	if err != nil {
		t.Fatalf("GetCurrentRefs() failed: %v", err)
	}

	// Verify commit SHA
	if len(refs.CommitSHA) != 40 {
		t.Errorf("CommitSHA length = %d, want 40", len(refs.CommitSHA))
	}

	// Verify tree hash
	if len(refs.TreeHash) != 40 {
		t.Errorf("TreeHash length = %d, want 40", len(refs.TreeHash))
	}

	// Verify they are different (commit != tree)
	if refs.CommitSHA == refs.TreeHash {
		t.Error("CommitSHA and TreeHash should be different")
	}

	// Verify consistency with individual functions
	expectedTreeHash, err := GetCurrentTreeHash(tmpDir)
	if err != nil {
		t.Fatalf("GetCurrentTreeHash() failed: %v", err)
	}
	if refs.TreeHash != expectedTreeHash {
		t.Errorf("TreeHash = %s, want %s", refs.TreeHash, expectedTreeHash)
	}
}

// TestGetCurrentRefs_NonGitRepo tests error handling for non-git directory
func TestGetCurrentRefs_NonGitRepo(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := GetCurrentRefs(tmpDir)
	if err == nil {
		t.Error("GetCurrentRefs() should fail in non-git directory")
	}
}
