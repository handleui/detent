package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/detent/cli/internal/act"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
)

// setupTestRepo creates a temporary git repository with a basic workflow for testing
func setupTestRepo(t *testing.T) (repoPath string, cleanup func()) {
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

	// Create .github/workflows directory
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o750); err != nil {
		t.Fatalf("Failed to create workflow directory: %v", err)
	}

	// Create a minimal workflow file
	workflowContent := `name: Test Workflow
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo "test"
`
	workflowFile := filepath.Join(workflowDir, "test.yml")
	if err := os.WriteFile(workflowFile, []byte(workflowContent), 0o644); err != nil {
		t.Fatalf("Failed to create workflow file: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0o644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "add", ".").Run(); err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}

	if err := exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit").Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	cleanup = func() {
		_ = exec.Command("git", "-C", tmpDir, "worktree", "prune").Run()
	}

	return tmpDir, cleanup
}

// TestCheckRunner_Prepare tests successful Prepare flow
func TestCheckRunner_Prepare(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	cfg := &RunConfig{
		RepoRoot:     repoPath,
		WorkflowPath: filepath.Join(repoPath, ".github", "workflows"),
		WorkflowFile: "",
		Event:        "push",
		RunID:        "0123456789abcdef",
		StreamOutput: false,
		UseTUI:       false,
	}

	runner := New(cfg)
	defer runner.Cleanup()

	ctx := context.Background()
	err := runner.Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}

	// Verify tmpDir is set
	if runner.tmpDir == "" {
		t.Error("tmpDir should be set after Prepare")
	}

	// Verify tmpDir exists
	if _, err := os.Stat(runner.tmpDir); os.IsNotExist(err) {
		t.Errorf("tmpDir should exist at %s", runner.tmpDir)
	}

	// Verify worktreeInfo is set
	if runner.worktreeInfo == nil {
		t.Fatal("worktreeInfo should be set after Prepare")
	}

	// Verify worktree path is set
	if runner.worktreeInfo.Path == "" {
		t.Error("worktreeInfo.Path should be set")
	}

	// Verify worktree exists
	if _, err := os.Stat(runner.worktreeInfo.Path); os.IsNotExist(err) {
		t.Errorf("worktree should exist at %s", runner.worktreeInfo.Path)
	}

	// Verify cleanup functions are set
	if runner.cleanupWorkflows == nil {
		t.Error("cleanupWorkflows should be set after Prepare")
	}

	if runner.cleanupWorktree == nil {
		t.Error("cleanupWorktree should be set after Prepare")
	}
}

// TestCheckRunner_PrepareCleanupOnError tests that preflight checks fail early in non-git directory
func TestCheckRunner_PrepareCleanupOnError(t *testing.T) {
	// Use non-git directory to trigger preflight check failure
	tmpDir := t.TempDir()

	cfg := &RunConfig{
		RepoRoot:     tmpDir,
		WorkflowPath: filepath.Join(tmpDir, ".github", "workflows"),
		WorkflowFile: "",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	// Create workflow directory and file (won't be used due to early preflight failure)
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o750); err != nil {
		t.Fatalf("Failed to create workflow directory: %v", err)
	}
	workflowContent := `name: Test
on: [push]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
`
	if err := os.WriteFile(filepath.Join(workflowDir, "test.yml"), []byte(workflowContent), 0o644); err != nil {
		t.Fatalf("Failed to create workflow file: %v", err)
	}

	runner := New(cfg)
	defer runner.Cleanup()

	ctx := context.Background()
	err := runner.Prepare(ctx)

	// Should fail because it's not a git repo
	if err == nil {
		t.Fatal("Prepare() should fail in non-git directory")
	}

	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("Error should mention 'not a git repository', got: %v", err)
	}

	// Verify tmpDir was NOT created (preflight checks failed before workflow preparation)
	if runner.tmpDir != "" {
		t.Error("tmpDir should not be set when preflight checks fail")
	}

	// After Cleanup, tmpDir should be removed
	runner.Cleanup()
	if runner.tmpDir != "" {
		if _, err := os.Stat(runner.tmpDir); !os.IsNotExist(err) {
			t.Error("tmpDir should be cleaned up after error")
		}
	}
}

// TestCheckRunner_RunWithoutPrepare tests Run without calling Prepare first
func TestCheckRunner_RunWithoutPrepare(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/nonexistent",
		WorkflowPath: "/nonexistent/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	ctx := context.Background()

	// Should return error because worktreeInfo is nil
	err := runner.Run(ctx)
	if !errors.Is(err, git.ErrWorktreeNotInitialized) {
		t.Errorf("Run() should return ErrWorktreeNotInitialized when Prepare() hasn't been called, got: %v", err)
	}
}

// TestCheckRunner_PersistWithoutRun tests Persist without calling Run first
func TestCheckRunner_PersistWithoutRun(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	cfg := &RunConfig{
		RepoRoot:     repoPath,
		WorkflowPath: filepath.Join(repoPath, ".github", "workflows"),
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)
	defer runner.Cleanup()

	ctx := context.Background()
	if err := runner.Prepare(ctx); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}

	// Try to persist without running
	err := runner.Persist()

	if err == nil {
		t.Fatal("Persist() should fail when Run() hasn't been called")
	}

	if !strings.Contains(err.Error(), "no result") {
		t.Errorf("Error should mention 'no result', got: %v", err)
	}
}

// TestCheckRunner_Cleanup_Idempotent tests that Cleanup can be called multiple times
func TestCheckRunner_Cleanup_Idempotent(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	cfg := &RunConfig{
		RepoRoot:     repoPath,
		WorkflowPath: filepath.Join(repoPath, ".github", "workflows"),
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	ctx := context.Background()
	if err := runner.Prepare(ctx); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}

	worktreePath := runner.worktreeInfo.Path

	// Call Cleanup multiple times - should not panic
	runner.Cleanup()
	runner.Cleanup()
	runner.Cleanup()

	// Verify worktree is removed
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree should be removed after Cleanup")
	}
}

// TestCheckRunner_Cleanup_WithoutPrepare tests cleanup when Prepare was never called
func TestCheckRunner_Cleanup_WithoutPrepare(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/nonexistent",
		WorkflowPath: "/nonexistent/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	// Should not panic when cleanup functions are nil
	runner.Cleanup()
}

// TestCheckRunner_GetResultBeforeRun tests GetResult before Run is called
func TestCheckRunner_GetResultBeforeRun(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/nonexistent",
		WorkflowPath: "/nonexistent/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	result := runner.GetResult()
	if result != nil {
		t.Error("GetResult() should return nil before Run() is called")
	}
}

// TestCheckRunner_buildActConfig_NonTUI tests buildActConfig with nil logChan
func TestCheckRunner_buildActConfig_NonTUI(t *testing.T) {
	tests := []struct {
		name         string
		streamOutput bool
		wantStream   bool
	}{
		{
			name:         "stream enabled",
			streamOutput: true,
			wantStream:   true,
		},
		{
			name:         "stream disabled",
			streamOutput: false,
			wantStream:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RunConfig{
				RepoRoot:     "/test",
				WorkflowPath: "/test/.github/workflows",
				Event:        "push",
				RunID:        "0123456789abcdef",
				StreamOutput: tt.streamOutput,
			}

			runner := New(cfg)
			runner.tmpDir = "/tmp/test"
			runner.worktreeInfo = &git.WorktreeInfo{
				Path: "/tmp/worktree",
			}

			actConfig := runner.buildActConfig(nil)

			if actConfig.StreamOutput != tt.wantStream {
				t.Errorf("StreamOutput = %v, want %v", actConfig.StreamOutput, tt.wantStream)
			}

			if actConfig.LogChan != nil {
				t.Error("LogChan should be nil when not using TUI")
			}

			if actConfig.WorkflowPath != runner.tmpDir {
				t.Errorf("WorkflowPath = %s, want %s", actConfig.WorkflowPath, runner.tmpDir)
			}

			if actConfig.Event != cfg.Event {
				t.Errorf("Event = %s, want %s", actConfig.Event, cfg.Event)
			}

			if actConfig.WorkDir != runner.worktreeInfo.Path {
				t.Errorf("WorkDir = %s, want %s", actConfig.WorkDir, runner.worktreeInfo.Path)
			}

			if actConfig.Verbose {
				t.Error("Verbose should be false")
			}
		})
	}
}

// TestCheckRunner_buildActConfig_TUI tests buildActConfig with logChan
func TestCheckRunner_buildActConfig_TUI(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/test",
		WorkflowPath: "/test/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
		StreamOutput: true, // Should be ignored when logChan is provided
	}

	runner := New(cfg)
	runner.tmpDir = "/tmp/test"
	runner.worktreeInfo = &git.WorktreeInfo{
		Path: "/tmp/worktree",
	}

	logChan := make(chan string, 10)
	defer close(logChan)

	actConfig := runner.buildActConfig(logChan)

	// When logChan is provided, StreamOutput should be false
	if actConfig.StreamOutput {
		t.Error("StreamOutput should be false when using TUI (logChan provided)")
	}

	if actConfig.LogChan != logChan {
		t.Error("LogChan should be set when using TUI")
	}

	if actConfig.WorkflowPath != runner.tmpDir {
		t.Errorf("WorkflowPath = %s, want %s", actConfig.WorkflowPath, runner.tmpDir)
	}

	if actConfig.Event != cfg.Event {
		t.Errorf("Event = %s, want %s", actConfig.Event, cfg.Event)
	}

	if actConfig.WorkDir != runner.worktreeInfo.Path {
		t.Errorf("WorkDir = %s, want %s", actConfig.WorkDir, runner.worktreeInfo.Path)
	}

	if actConfig.Verbose {
		t.Error("Verbose should be false")
	}
}

// TestCheckRunner_extractAndProcessErrors tests error extraction and processing
func TestCheckRunner_extractAndProcessErrors(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/test/repo",
		WorkflowPath: "/test/repo/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	tests := []struct {
		name             string
		actResult        *act.RunResult
		wantExtractedLen int
		wantErrorCount   bool
	}{
		{
			name: "no errors in output",
			actResult: &act.RunResult{
				Stdout:   "workflow completed successfully",
				Stderr:   "",
				ExitCode: 0,
				Duration: 5 * time.Second,
			},
			wantExtractedLen: 0,
			wantErrorCount:   false,
		},
		{
			name: "with errors in output",
			actResult: &act.RunResult{
				Stdout:   "main.go:10:5: undefined: x\n",
				Stderr:   "Error: Process completed with exit code 1\n",
				ExitCode: 1,
				Duration: 3 * time.Second,
			},
			wantExtractedLen: 2, // Go error + exit code metadata
			wantErrorCount:   true,
		},
		{
			name: "empty output",
			actResult: &act.RunResult{
				Stdout:   "",
				Stderr:   "",
				ExitCode: 0,
				Duration: 1 * time.Second,
			},
			wantExtractedLen: 0,
			wantErrorCount:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extracted, grouped := runner.extractAndProcessErrors(tt.actResult)

			if len(extracted) != tt.wantExtractedLen {
				t.Errorf("extracted length = %d, want %d", len(extracted), tt.wantExtractedLen)
			}

			if grouped == nil {
				t.Fatal("grouped should not be nil")
			}

			hasErrors := grouped.HasErrors()
			if hasErrors != tt.wantErrorCount {
				t.Errorf("has errors = %v, want %v", hasErrors, tt.wantErrorCount)
			}
		})
	}
}

// TestCheckRunner_NewRunner tests creating a new runner
func TestCheckRunner_NewRunner(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/test",
		WorkflowPath: "/test/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	if runner == nil {
		t.Fatal("New() should not return nil")
	}

	if runner.config != cfg {
		t.Error("config should be set")
	}

	if runner.tmpDir != "" {
		t.Error("tmpDir should be empty before Prepare")
	}

	if runner.worktreeInfo != nil {
		t.Error("worktreeInfo should be nil before Prepare")
	}

	if runner.result != nil {
		t.Error("result should be nil before Run")
	}

	if runner.cleanupWorkflows != nil {
		t.Error("cleanupWorkflows should be nil before Prepare")
	}

	if runner.cleanupWorktree != nil {
		t.Error("cleanupWorktree should be nil before Prepare")
	}
}

// TestCheckRunner_PrepareWorkflowInjection tests that workflows are properly prepared
func TestCheckRunner_PrepareWorkflowInjection(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	cfg := &RunConfig{
		RepoRoot:     repoPath,
		WorkflowPath: filepath.Join(repoPath, ".github", "workflows"),
		WorkflowFile: "",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)
	defer runner.Cleanup()

	ctx := context.Background()
	if err := runner.Prepare(ctx); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}

	// Verify that workflow files exist in tmpDir
	entries, err := os.ReadDir(runner.tmpDir)
	if err != nil {
		t.Fatalf("Failed to read tmpDir: %v", err)
	}

	if len(entries) == 0 {
		t.Error("tmpDir should contain workflow files after Prepare")
	}

	// Verify at least one .yml or .yaml file exists
	hasWorkflow := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yml") || strings.HasSuffix(entry.Name(), ".yaml") {
			hasWorkflow = true
			break
		}
	}

	if !hasWorkflow {
		t.Error("tmpDir should contain at least one workflow file")
	}
}

// TestCheckRunner_PrepareWithSpecificWorkflowFile tests preparation with specific workflow file
func TestCheckRunner_PrepareWithSpecificWorkflowFile(t *testing.T) {
	repoPath, cleanup := setupTestRepo(t)
	defer cleanup()

	cfg := &RunConfig{
		RepoRoot:     repoPath,
		WorkflowPath: filepath.Join(repoPath, ".github", "workflows"),
		WorkflowFile: "test.yml",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)
	defer runner.Cleanup()

	ctx := context.Background()
	if err := runner.Prepare(ctx); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}

	// Verify tmpDir is set and contains files
	if runner.tmpDir == "" {
		t.Fatal("tmpDir should be set after Prepare")
	}

	entries, err := os.ReadDir(runner.tmpDir)
	if err != nil {
		t.Fatalf("Failed to read tmpDir: %v", err)
	}

	if len(entries) == 0 {
		t.Error("tmpDir should contain workflow file after Prepare")
	}
}

// TestCheckRunner_PersistWithoutWorktreeInfo tests Persist when worktreeInfo is nil
func TestCheckRunner_PersistWithoutWorktreeInfo(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/test",
		WorkflowPath: "/test/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	// Manually set result without setting worktreeInfo
	runner.result = &RunResult{
		ActResult: &act.RunResult{
			Stdout:   "",
			Stderr:   "",
			ExitCode: 0,
			Duration: 1 * time.Second,
		},
		Extracted: []*internalerrors.ExtractedError{},
		Grouped:   &internalerrors.GroupedErrors{},
		RunID:     cfg.RunID,
	}

	err := runner.Persist()
	if err == nil {
		t.Fatal("Persist() should fail when worktreeInfo is nil")
	}

	if !strings.Contains(err.Error(), "worktree") {
		t.Errorf("Error should mention 'worktree', got: %v", err)
	}
}

// TestCheckRunner_StartTimeTracking tests that start time is properly tracked
func TestCheckRunner_StartTimeTracking(t *testing.T) {
	cfg := &RunConfig{
		RepoRoot:     "/test",
		WorkflowPath: "/test/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	if !runner.startTime.IsZero() {
		t.Error("startTime should be zero before Run")
	}

	// Note: We can't easily test Run() without a full setup,
	// but we've verified the initial state
}
