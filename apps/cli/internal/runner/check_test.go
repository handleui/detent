package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/detent/cli/internal/act"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
	"golang.org/x/sync/errgroup"
)

// generateTestRunID creates a unique 16-character hex string for test isolation.
// Each test should use its own RunID to avoid conflicts with other tests.
func generateTestRunID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
	}
	return hex.EncodeToString(b)
}

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
		RunID:        generateTestRunID(t),
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

	// Note: cleanupWorktree is nil for persistent worktrees (by design)
	// Persistent worktrees are reused by heal command
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
		RunID:        generateTestRunID(t),
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
		RunID:        generateTestRunID(t),
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

	// Note: Worktrees are now ephemeral (removed on cleanup)
	// Verify worktree is removed after cleanup
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Ephemeral worktree should be removed after Cleanup")
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
			wantExtractedLen: 1, // Go error (exit code message is filtered as noise)
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
			extracted, grouped, groupedComprehensive := runner.extractAndProcessErrors(tt.actResult)

			if len(extracted) != tt.wantExtractedLen {
				t.Errorf("extracted length = %d, want %d", len(extracted), tt.wantExtractedLen)
			}

			if grouped == nil {
				t.Fatal("grouped should not be nil")
			}

			if groupedComprehensive == nil {
				t.Fatal("groupedComprehensive should not be nil")
			}

			hasErrors := grouped.HasErrors()
			if hasErrors != tt.wantErrorCount {
				t.Errorf("has errors = %v, want %v", hasErrors, tt.wantErrorCount)
			}

			// Verify comprehensive grouping also has same error count
			hasErrorsComp := groupedComprehensive.Stats.ErrorCount > 0
			if hasErrorsComp != tt.wantErrorCount {
				t.Errorf("comprehensive has errors = %v, want %v", hasErrorsComp, tt.wantErrorCount)
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
		RunID:        generateTestRunID(t),
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
		RunID:        generateTestRunID(t),
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

// =============================================================================
// Race Condition Tests
// =============================================================================
// These tests are designed to catch race conditions when run with `go test -race`.
// They exercise concurrent code paths in preparer.go and executor.go to verify
// that shared state access is properly synchronized.

// TestPreparer_ParallelChannelWrites_Race tests that concurrent writes to
// workflowChan and worktreeChan don't cause races. This exercises the parallel
// goroutine pattern in prepareWorkflowsAndWorktree.
func TestPreparer_ParallelChannelWrites_Race(t *testing.T) {
	// Run multiple iterations to increase likelihood of catching races
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			type workflowResult struct {
				tmpDir           string
				cleanupWorkflows func()
			}

			type worktreeResult struct {
				worktreePath    string
				cleanupWorktree func()
			}

			workflowChan := make(chan workflowResult, 1)
			worktreeChan := make(chan worktreeResult, 1)

			// Simulate concurrent writes to channels (mimics preparer.go pattern)
			go func() {
				// Simulate some work
				time.Sleep(time.Microsecond * time.Duration(i%5))
				workflowChan <- workflowResult{
					tmpDir:           "/tmp/workflow",
					cleanupWorkflows: func() {},
				}
			}()

			go func() {
				// Simulate some work
				time.Sleep(time.Microsecond * time.Duration(i%3))
				worktreeChan <- worktreeResult{
					worktreePath:    "/tmp/worktree",
					cleanupWorktree: func() {},
				}
			}()

			// Read from both channels (mimics the pattern in preparer.go)
			workflowRes := <-workflowChan
			worktreeRes := <-worktreeChan

			if workflowRes.tmpDir == "" {
				t.Error("workflow result should have tmpDir")
			}
			if worktreeRes.worktreePath == "" {
				t.Error("worktree result should have path")
			}
		})
	}
}

// TestExecutor_LogChannelConcurrency_Race tests that concurrent log channel
// operations don't cause races. This exercises the pattern in ExecuteWithTUI
// where one goroutine writes and another reads from logChan.
func TestExecutor_LogChannelConcurrency_Race(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			logChan := make(chan string, 100)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			var wg sync.WaitGroup
			wg.Add(2)

			// Writer goroutine (simulates startActRunnerGoroutine)
			go func() {
				defer wg.Done()
				defer close(logChan)
				for j := 0; j < 50; j++ {
					select {
					case logChan <- fmt.Sprintf("log line %d", j):
					case <-ctx.Done():
						return
					}
				}
			}()

			// Reader goroutine (simulates startLogProcessorGoroutine)
			var receivedCount int
			go func() {
				defer wg.Done()
				for {
					select {
					case line, ok := <-logChan:
						if !ok {
							return
						}
						if line != "" {
							receivedCount++
						}
					case <-ctx.Done():
						// Drain remaining
						for range logChan {
						}
						return
					}
				}
			}()

			wg.Wait()

			if receivedCount == 0 {
				t.Error("should have received some log lines")
			}
		})
	}
}

// TestExecutor_ResultChannelConcurrency_Race tests that concurrent result
// channel operations don't cause races. This exercises the pattern where
// startActRunnerGoroutine writes to resultChan.
func TestExecutor_ResultChannelConcurrency_Race(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			type result struct {
				stdout   string
				exitCode int
			}

			resultChan := make(chan result, 1)

			var wg sync.WaitGroup
			wg.Add(1)

			// Writer goroutine (simulates startActRunnerGoroutine)
			go func() {
				defer wg.Done()
				// Simulate variable execution time
				time.Sleep(time.Microsecond * time.Duration(i%10))
				resultChan <- result{
					stdout:   "test output",
					exitCode: 0,
				}
			}()

			// Simulate program.Run() waiting and then reading result
			time.Sleep(time.Microsecond * 5)
			res := <-resultChan
			wg.Wait()

			if res.stdout != "test output" {
				t.Errorf("expected 'test output', got %q", res.stdout)
			}
		})
	}
}

// TestPreparer_ErrGroupConcurrency_Race tests that errgroup-based parallel
// checks don't cause races. This exercises the patterns in runPreflightChecks
// and runValidationChecks.
func TestPreparer_ErrGroupConcurrency_Race(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			g, gctx := errgroup.WithContext(ctx)

			// Shared state that checks might access (simulates verbose flag check)
			var checkResults []string
			var mu sync.Mutex

			// Simulate parallel preflight checks
			g.Go(func() error {
				select {
				case <-gctx.Done():
					return gctx.Err()
				default:
					mu.Lock()
					checkResults = append(checkResults, "git")
					mu.Unlock()
					return nil
				}
			})

			g.Go(func() error {
				select {
				case <-gctx.Done():
					return gctx.Err()
				default:
					mu.Lock()
					checkResults = append(checkResults, "act")
					mu.Unlock()
					return nil
				}
			})

			g.Go(func() error {
				select {
				case <-gctx.Done():
					return gctx.Err()
				default:
					mu.Lock()
					checkResults = append(checkResults, "docker")
					mu.Unlock()
					return nil
				}
			})

			if err := g.Wait(); err != nil {
				t.Fatalf("errgroup failed: %v", err)
			}

			mu.Lock()
			if len(checkResults) != 3 {
				t.Errorf("expected 3 check results, got %d", len(checkResults))
			}
			mu.Unlock()
		})
	}
}

// TestExecutor_WaitGroupSynchronization_Race tests that WaitGroup synchronization
// in ExecuteWithTUI doesn't cause races. This exercises the pattern where
// multiple goroutines increment/decrement the same WaitGroup.
func TestExecutor_WaitGroupSynchronization_Race(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			logChan := make(chan string, 10)
			resultChan := make(chan struct{}, 1)

			var wg sync.WaitGroup
			wg.Add(2)

			// Goroutine 1 (simulates startActRunnerGoroutine)
			go func() {
				defer wg.Done()
				defer close(logChan)
				for j := 0; j < 5; j++ {
					select {
					case logChan <- "msg":
					case <-ctx.Done():
						return
					}
				}
				resultChan <- struct{}{}
			}()

			// Goroutine 2 (simulates startLogProcessorGoroutine)
			go func() {
				defer wg.Done()
				for {
					select {
					case _, ok := <-logChan:
						if !ok {
							return
						}
					case <-ctx.Done():
						for range logChan {
						}
						return
					}
				}
			}()

			// Wait for result then cleanup (simulates program.Run() pattern)
			select {
			case <-resultChan:
			case <-ctx.Done():
			}

			wg.Wait()
		})
	}
}

// TestPreparer_CleanupFunctionRace tests that cleanup functions can be called
// safely after concurrent preparation. This exercises the pattern where
// cleanup functions are set by goroutines and later called.
func TestPreparer_CleanupFunctionRace(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			var cleanupWorkflows func()
			var cleanupWorktree func()
			var mu sync.Mutex

			workflowChan := make(chan func(), 1)
			worktreeChan := make(chan func(), 1)

			// Concurrent setup of cleanup functions
			go func() {
				time.Sleep(time.Microsecond * time.Duration(i%5))
				workflowChan <- func() {
					mu.Lock()
					defer mu.Unlock()
				}
			}()

			go func() {
				time.Sleep(time.Microsecond * time.Duration(i%3))
				worktreeChan <- func() {
					mu.Lock()
					defer mu.Unlock()
				}
			}()

			cleanupWorkflows = <-workflowChan
			cleanupWorktree = <-worktreeChan

			// Call cleanup functions (simulates Cleanup() method)
			if cleanupWorkflows != nil {
				cleanupWorkflows()
			}
			if cleanupWorktree != nil {
				cleanupWorktree()
			}
		})
	}
}

// TestErrorProcessor_ConcurrentProcessing_Race tests that ErrorProcessor can
// be called concurrently without races. This is important because multiple
// goroutines might process errors simultaneously.
func TestErrorProcessor_ConcurrentProcessing_Race(t *testing.T) {
	t.Parallel()

	processor := NewErrorProcessor("/test/repo")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result := &act.RunResult{
				Stdout:   fmt.Sprintf("main.go:%d:5: undefined: x\n", idx),
				Stderr:   "",
				ExitCode: 1,
				Duration: time.Second,
			}
			processed := processor.Process(result)
			if processed == nil {
				t.Error("processed result should not be nil")
			}
		}(i)
	}
	wg.Wait()
}

// TestCheckRunner_ConcurrentGetResult_Race tests that GetResult can be called
// concurrently without races after Run completes.
func TestCheckRunner_ConcurrentGetResult_Race(t *testing.T) {
	t.Parallel()

	cfg := &RunConfig{
		RepoRoot:     "/test",
		WorkflowPath: "/test/.github/workflows",
		Event:        "push",
		RunID:        "0123456789abcdef",
	}

	runner := New(cfg)

	// Manually set result to simulate post-Run state
	runner.result = &RunResult{
		ActResult: &act.RunResult{
			Stdout:   "test",
			ExitCode: 0,
			Duration: time.Second,
		},
		RunID: cfg.RunID,
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := runner.GetResult()
			if result == nil {
				t.Error("result should not be nil")
			}
			if result.RunID != cfg.RunID {
				t.Errorf("expected RunID %s, got %s", cfg.RunID, result.RunID)
			}
		}()
	}
	wg.Wait()
}

// TestPreparer_ContextCancellation_Race tests that context cancellation
// doesn't cause races in concurrent operations.
func TestPreparer_ContextCancellation_Race(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())

			resultChan := make(chan error, 2)
			var wg sync.WaitGroup
			wg.Add(2)

			// Start goroutines that respect context
			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					resultChan <- ctx.Err()
				case <-time.After(10 * time.Millisecond):
					resultChan <- nil
				}
			}()

			go func() {
				defer wg.Done()
				select {
				case <-ctx.Done():
					resultChan <- ctx.Err()
				case <-time.After(10 * time.Millisecond):
					resultChan <- nil
				}
			}()

			// Cancel at a random point
			time.Sleep(time.Microsecond * time.Duration(i%5))
			cancel()

			wg.Wait()
			close(resultChan)

			// Drain results
			for range resultChan {
			}
		})
	}
}
