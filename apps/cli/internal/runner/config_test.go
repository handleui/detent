package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunConfig_Validate uses table-driven tests to validate RunConfig
func TestRunConfig_Validate(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	validWorkflowPath := filepath.Join(tmpDir, ".github", "workflows")
	err := os.MkdirAll(validWorkflowPath, 0o755)
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	tests := []struct {
		name    string
		config  *RunConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "push",
			},
			wantErr: false,
		},
		{
			name: "valid config with default event",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
			},
			wantErr: false,
		},
		{
			name: "valid config with underscore event",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "pull_request",
			},
			wantErr: false,
		},
		{
			name: "valid config with hyphen event",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "workflow-dispatch",
			},
			wantErr: false,
		},
		{
			name: "missing RepoRoot",
			config: &RunConfig{
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "RepoRoot is required",
		},
		{
			name: "missing WorkflowPath",
			config: &RunConfig{
				RepoRoot: tmpDir,
				RunID:    "0123456789abcdef",
				Event:    "push",
			},
			wantErr: true,
			errMsg:  "WorkflowPath is required",
		},
		{
			name: "missing RunID",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "RunID is required",
		},
		{
			name: "invalid RunID format - too short",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcde", // 15 chars
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "invalid RunID format",
		},
		{
			name: "invalid RunID format - too long",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef0", // 17 chars
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "invalid RunID format",
		},
		{
			name: "invalid RunID format - uppercase",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789ABCDEF", // uppercase
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "invalid RunID format",
		},
		{
			name: "invalid RunID format - not hex",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdeg", // 'g' is not hex
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "invalid RunID format",
		},
		{
			name: "path traversal with ..",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: filepath.Join(tmpDir, "..", "..", "etc", "workflows"),
				RunID:        "0123456789abcdef",
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "WorkflowPath must be within RepoRoot",
		},
		{
			name: "path traversal with relative path",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: "../../../etc/workflows",
				RunID:        "0123456789abcdef",
				Event:        "push",
			},
			wantErr: true,
			errMsg:  "WorkflowPath must be within RepoRoot",
		},
		{
			name: "invalid event with special characters",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "push; rm -rf /",
			},
			wantErr: true,
			errMsg:  "invalid event name",
		},
		{
			name: "invalid event with spaces",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "pull request",
			},
			wantErr: true,
			errMsg:  "invalid event name",
		},
		{
			name: "invalid event with dots",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "push.event",
			},
			wantErr: true,
			errMsg:  "invalid event name",
		},
		{
			name: "invalid event with slashes",
			config: &RunConfig{
				RepoRoot:     tmpDir,
				WorkflowPath: validWorkflowPath,
				RunID:        "0123456789abcdef",
				Event:        "push/test",
			},
			wantErr: true,
			errMsg:  "invalid event name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

// TestRunConfig_Validate_SymlinkRejection tests that symlinks are rejected
func TestRunConfig_Validate_SymlinkRejection(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	symlinkPath := filepath.Join(tmpDir, "symlink")

	err := os.MkdirAll(targetDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	err = os.Symlink(targetDir, symlinkPath)
	if err != nil {
		t.Skipf("failed to create symlink (may not be supported): %v", err)
	}

	cfg := &RunConfig{
		RepoRoot:     tmpDir,
		WorkflowPath: symlinkPath,
		RunID:        "0123456789abcdef",
		Event:        "push",
	}

	err = cfg.Validate()
	if err == nil {
		t.Error("expected symlink to fail validation")
	}
	if !contains(err.Error(), "cannot be a symlink") {
		t.Errorf("expected error about symlink, got: %v", err)
	}
}

// TestRunConfig_Validate_DefaultEvent tests that Event defaults to "push"
func TestRunConfig_Validate_DefaultEvent(t *testing.T) {
	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, ".github", "workflows")
	err := os.MkdirAll(workflowPath, 0o755)
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	cfg := &RunConfig{
		RepoRoot:     tmpDir,
		WorkflowPath: workflowPath,
		RunID:        "0123456789abcdef",
		Event:        "",
	}

	err = cfg.Validate()
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if cfg.Event != "push" {
		t.Errorf("expected Event to default to 'push', got %q", cfg.Event)
	}
}

// TestRunConfig_ResolveWorkflowPath tests the ResolveWorkflowPath method
func TestRunConfig_ResolveWorkflowPath(t *testing.T) {
	tests := []struct {
		name         string
		workflowPath string
		want         string
	}{
		{
			name:         "absolute path",
			workflowPath: "/test/path",
			want:         "/test/path",
		},
		{
			name:         "relative path",
			workflowPath: ".github/workflows",
			want:         ".github/workflows",
		},
		{
			name:         "empty path",
			workflowPath: "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RunConfig{
				WorkflowPath: tt.workflowPath,
			}
			if got := cfg.ResolveWorkflowPath(); got != tt.want {
				t.Errorf("ResolveWorkflowPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || substr == "" || (s != "" && substr != "" && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
