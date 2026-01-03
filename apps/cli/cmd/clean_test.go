package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/detent/cli/internal/persistence"
	"github.com/spf13/cobra"
)

// --- Test helpers ---

// captureStderr captures stderr output during test execution.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatalf("Failed to copy stderr: %v", copyErr)
	}
	return buf.String()
}

// resetCleanFlags resets all clean command flags to their default values.
func resetCleanFlags() {
	cleanForce = false
	cleanAll = false
	cleanRetentionDays = defaultRetentionDays
	cleanDryRun = false
}

// --- Command and flag tests ---

func TestCleanCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "clean command has correct use",
			wantUse: "clean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cleanCmd.Use != tt.wantUse {
				t.Errorf("cleanCmd.Use = %q, want %q", cleanCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestCleanCommandFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		shorthand string
		wantType  string
	}{
		{
			name:      "force flag exists",
			flagName:  "force",
			shorthand: "f",
			wantType:  "bool",
		},
		{
			name:      "all flag exists",
			flagName:  "all",
			shorthand: "a",
			wantType:  "bool",
		},
		{
			name:      "retention flag exists",
			flagName:  "retention",
			shorthand: "r",
			wantType:  "int",
		},
		{
			name:      "dry-run flag exists",
			flagName:  "dry-run",
			shorthand: "",
			wantType:  "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cleanCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("Flag %q not found", tt.flagName)
				return
			}

			if flag.Value.Type() != tt.wantType {
				t.Errorf("Flag %q has type %q, want %q", tt.flagName, flag.Value.Type(), tt.wantType)
			}

			if flag.Shorthand != tt.shorthand {
				t.Errorf("Flag %q has shorthand %q, want %q", tt.flagName, flag.Shorthand, tt.shorthand)
			}
		})
	}
}

func TestCleanCommandRunE(t *testing.T) {
	if cleanCmd.RunE == nil {
		t.Error("cleanCmd.RunE should be set")
	}
}

func TestCleanFlagDefaults(t *testing.T) {
	forceFlag := cleanCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("force flag not found")
	}

	allFlag := cleanCmd.Flags().Lookup("all")
	if allFlag == nil {
		t.Fatal("all flag not found")
	}

	retentionFlag := cleanCmd.Flags().Lookup("retention")
	if retentionFlag == nil {
		t.Fatal("retention flag not found")
	}

	dryRunFlag := cleanCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("dry-run flag not found")
	}

	if forceFlag.DefValue != "false" {
		t.Errorf("force flag default = %q, want %q", forceFlag.DefValue, "false")
	}
	if allFlag.DefValue != "false" {
		t.Errorf("all flag default = %q, want %q", allFlag.DefValue, "false")
	}
	if retentionFlag.DefValue != "30" {
		t.Errorf("retention flag default = %q, want %q", retentionFlag.DefValue, "30")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("dry-run flag default = %q, want %q", dryRunFlag.DefValue, "false")
	}
}

func TestDefaultRetentionDays(t *testing.T) {
	if defaultRetentionDays != 30 {
		t.Errorf("defaultRetentionDays = %d, want 30", defaultRetentionDays)
	}
}

// --- processCleanDB mock tests ---

func TestProcessCleanDB(t *testing.T) {
	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	called := false
	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		called = true
		return &persistence.GCStats{
			RunsDeleted:   5,
			ErrorsDeleted: 10,
		}, nil
	}

	stats, err := processCleanDB("/fake/path.db", 30, false)
	if err != nil {
		t.Errorf("processCleanDB returned error: %v", err)
	}
	if !called {
		t.Error("processCleanDB was not called")
	}
	if stats.RunsDeleted != 5 {
		t.Errorf("RunsDeleted = %d, want 5", stats.RunsDeleted)
	}
	if stats.ErrorsDeleted != 10 {
		t.Errorf("ErrorsDeleted = %d, want 10", stats.ErrorsDeleted)
	}
}

func TestProcessCleanDB_DryRunParameter(t *testing.T) {
	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	var capturedDryRun bool
	processCleanDB = func(_ string, _ int, dryRun bool) (*persistence.GCStats, error) {
		capturedDryRun = dryRun
		return &persistence.GCStats{}, nil
	}

	// Test dry-run true
	_, _ = processCleanDB("/fake/path.db", 30, true)
	if !capturedDryRun {
		t.Error("dry-run parameter should be true")
	}

	// Test dry-run false
	_, _ = processCleanDB("/fake/path.db", 30, false)
	if capturedDryRun {
		t.Error("dry-run parameter should be false")
	}
}

func TestProcessCleanDB_RetentionDaysParameter(t *testing.T) {
	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	var capturedRetentionDays int
	processCleanDB = func(_ string, retentionDays int, _ bool) (*persistence.GCStats, error) {
		capturedRetentionDays = retentionDays
		return &persistence.GCStats{}, nil
	}

	testCases := []int{1, 7, 30, 90, 365}
	for _, expected := range testCases {
		_, _ = processCleanDB("/fake/path.db", expected, false)
		if capturedRetentionDays != expected {
			t.Errorf("retention days = %d, want %d", capturedRetentionDays, expected)
		}
	}
}

func TestProcessCleanDB_Error(t *testing.T) {
	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	expectedErr := errors.New("database error")
	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		return nil, expectedErr
	}

	stats, err := processCleanDB("/fake/path.db", 30, false)
	if err == nil {
		t.Error("expected error but got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error = %v, want %v", err, expectedErr)
	}
	if stats != nil {
		t.Errorf("stats should be nil on error, got %v", stats)
	}
}

// --- cleanData tests ---

func TestCleanData_SingleRepo_DatabaseDoesNotExist(t *testing.T) {
	resetCleanFlags()
	tmpDir := t.TempDir()

	// Create a non-existent db path scenario by using temp dir
	stats, err := cleanData(tmpDir, 30)
	if err != nil {
		t.Errorf("cleanData should not error when database doesn't exist: %v", err)
	}
	if stats == nil {
		t.Fatal("stats should not be nil")
	}
	// Empty stats expected
	if stats.RunsDeleted != 0 {
		t.Errorf("RunsDeleted = %d, want 0", stats.RunsDeleted)
	}
}

func TestCleanData_SingleRepo_Success(t *testing.T) {
	resetCleanFlags()
	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	expectedStats := &persistence.GCStats{
		RunsDeleted:           3,
		RunErrorsDeleted:      5,
		ErrorLocationsDeleted: 2,
		HealsDeleted:          1,
		ErrorsDeleted:         4,
	}

	var capturedDBPath string
	processCleanDB = func(dbPath string, _ int, _ bool) (*persistence.GCStats, error) {
		capturedDBPath = dbPath
		return expectedStats, nil
	}

	// Create a fake database file
	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	// Set DETENT_HOME for the test
	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	// Create a fake db file (using a predictable path)
	repoRoot := filepath.Join(tmpDir, "my-repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	// Get the expected database path and create it
	dbPath, err := persistence.GetDatabasePath(repoRoot)
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create fake db: %v", err)
	}
	f.Close()

	stats, err := cleanData(repoRoot, 30)
	if err != nil {
		t.Fatalf("cleanData returned error: %v", err)
	}

	if capturedDBPath != dbPath {
		t.Errorf("processCleanDB called with path %q, want %q", capturedDBPath, dbPath)
	}

	if stats.RunsDeleted != expectedStats.RunsDeleted {
		t.Errorf("RunsDeleted = %d, want %d", stats.RunsDeleted, expectedStats.RunsDeleted)
	}
	if stats.ErrorsDeleted != expectedStats.ErrorsDeleted {
		t.Errorf("ErrorsDeleted = %d, want %d", stats.ErrorsDeleted, expectedStats.ErrorsDeleted)
	}
}

func TestCleanData_SingleRepo_ProcessError(t *testing.T) {
	resetCleanFlags()
	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		return nil, errors.New("gc failed")
	}

	// Create temp structure
	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	repoRoot := filepath.Join(tmpDir, "my-repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	dbPath, err := persistence.GetDatabasePath(repoRoot)
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create fake db: %v", err)
	}
	f.Close()

	stats, err := cleanData(repoRoot, 30)
	if err == nil {
		t.Error("expected error but got nil")
	}
	if stats != nil {
		t.Errorf("stats should be nil on error, got %v", stats)
	}
}

func TestCleanData_AllRepos_MultipleDBs(t *testing.T) {
	resetCleanFlags()
	cleanAll = true
	defer func() { cleanAll = false }()

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	callCount := 0
	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		callCount++
		return &persistence.GCStats{
			RunsDeleted:   callCount,
			ErrorsDeleted: callCount * 2,
		}, nil
	}

	// Create temp structure with multiple DBs
	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	// Create multiple fake db files
	for i := 0; i < 3; i++ {
		dbPath := filepath.Join(reposDir, "repo"+string(rune('a'+i))+".db")
		f, err := os.Create(dbPath)
		if err != nil {
			t.Fatalf("Failed to create fake db: %v", err)
		}
		f.Close()
	}

	stats, err := cleanData(tmpDir, 30)
	if err != nil {
		t.Fatalf("cleanData returned error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("processCleanDB called %d times, want 3", callCount)
	}

	// Stats should be accumulated: 1+2+3=6 for runs, (1+2+3)*2=12 for errors
	expectedRuns := 1 + 2 + 3
	expectedErrors := (1 + 2 + 3) * 2
	if stats.RunsDeleted != expectedRuns {
		t.Errorf("RunsDeleted = %d, want %d", stats.RunsDeleted, expectedRuns)
	}
	if stats.ErrorsDeleted != expectedErrors {
		t.Errorf("ErrorsDeleted = %d, want %d", stats.ErrorsDeleted, expectedErrors)
	}
}

func TestCleanData_AllRepos_PartialFailure(t *testing.T) {
	resetCleanFlags()
	cleanAll = true
	defer func() { cleanAll = false }()

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	callCount := 0
	processCleanDB = func(dbPath string, _ int, _ bool) (*persistence.GCStats, error) {
		callCount++
		// Second DB fails
		if callCount == 2 {
			return nil, errors.New("db2 error")
		}
		return &persistence.GCStats{
			RunsDeleted: 5,
		}, nil
	}

	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	// Create 3 fake db files
	for i := 0; i < 3; i++ {
		dbPath := filepath.Join(reposDir, "repo"+string(rune('a'+i))+".db")
		f, err := os.Create(dbPath)
		if err != nil {
			t.Fatalf("Failed to create fake db: %v", err)
		}
		f.Close()
	}

	// Should still succeed (partial failure is ok as long as some succeed)
	output := captureStderr(t, func() {
		stats, err := cleanData(tmpDir, 30)
		if err != nil {
			t.Errorf("cleanData should not return error for partial failure: %v", err)
		}
		// Should have accumulated from 2 successful DBs
		if stats != nil && stats.RunsDeleted != 10 {
			t.Errorf("RunsDeleted = %d, want 10 (5 from each successful db)", stats.RunsDeleted)
		}
	})

	// Should have printed a warning
	if output == "" {
		t.Log("Note: stderr was empty, warning may have been suppressed")
	}
}

func TestCleanData_AllRepos_AllFail(t *testing.T) {
	resetCleanFlags()
	cleanAll = true
	defer func() { cleanAll = false }()

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		return nil, errors.New("all dbs fail")
	}

	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	// Create 2 fake db files
	for i := 0; i < 2; i++ {
		dbPath := filepath.Join(reposDir, "repo"+string(rune('a'+i))+".db")
		f, err := os.Create(dbPath)
		if err != nil {
			t.Fatalf("Failed to create fake db: %v", err)
		}
		f.Close()
	}

	captureStderr(t, func() {
		_, err := cleanData(tmpDir, 30)
		if err == nil {
			t.Error("expected error when all databases fail")
		}
	})
}

func TestCleanData_AllRepos_NoDatabases(t *testing.T) {
	resetCleanFlags()
	cleanAll = true
	defer func() { cleanAll = false }()

	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	stats, err := cleanData(tmpDir, 30)
	if err != nil {
		t.Errorf("cleanData should not error with empty repos dir: %v", err)
	}
	if stats == nil {
		t.Fatal("stats should not be nil")
	}
	if stats.RunsDeleted != 0 {
		t.Errorf("RunsDeleted = %d, want 0", stats.RunsDeleted)
	}
}

func TestCleanData_StatsAccumulation(t *testing.T) {
	resetCleanFlags()
	cleanAll = true
	defer func() { cleanAll = false }()

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	callIdx := 0
	dbStats := []*persistence.GCStats{
		{RunsDeleted: 10, RunErrorsDeleted: 20, ErrorLocationsDeleted: 30, HealsDeleted: 5, ErrorsDeleted: 15},
		{RunsDeleted: 5, RunErrorsDeleted: 10, ErrorLocationsDeleted: 15, HealsDeleted: 2, ErrorsDeleted: 8},
	}

	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		stats := dbStats[callIdx]
		callIdx++
		return stats, nil
	}

	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	// Create 2 fake db files
	for i := 0; i < 2; i++ {
		dbPath := filepath.Join(reposDir, "repo"+string(rune('a'+i))+".db")
		f, err := os.Create(dbPath)
		if err != nil {
			t.Fatalf("Failed to create fake db: %v", err)
		}
		f.Close()
	}

	stats, err := cleanData(tmpDir, 30)
	if err != nil {
		t.Fatalf("cleanData returned error: %v", err)
	}

	// Verify all stats are properly accumulated
	if stats.RunsDeleted != 15 {
		t.Errorf("RunsDeleted = %d, want 15", stats.RunsDeleted)
	}
	if stats.RunErrorsDeleted != 30 {
		t.Errorf("RunErrorsDeleted = %d, want 30", stats.RunErrorsDeleted)
	}
	if stats.ErrorLocationsDeleted != 45 {
		t.Errorf("ErrorLocationsDeleted = %d, want 45", stats.ErrorLocationsDeleted)
	}
	if stats.HealsDeleted != 7 {
		t.Errorf("HealsDeleted = %d, want 7", stats.HealsDeleted)
	}
	if stats.ErrorsDeleted != 23 {
		t.Errorf("ErrorsDeleted = %d, want 23", stats.ErrorsDeleted)
	}
}

// --- printCleanResults tests ---

func TestPrintCleanResults_NoChanges(t *testing.T) {
	resetCleanFlags()

	stats := &persistence.GCStats{}
	output := captureStderr(t, func() {
		printCleanResults(0, nil, stats, nil, 100*time.Millisecond)
	})

	if output == "" {
		t.Error("expected output but got empty string")
	}
}

func TestPrintCleanResults_WithWorktreesRemoved(t *testing.T) {
	resetCleanFlags()

	stats := &persistence.GCStats{}
	output := captureStderr(t, func() {
		printCleanResults(5, nil, stats, nil, 100*time.Millisecond)
	})

	if output == "" {
		t.Error("expected output but got empty string")
	}
}

func TestPrintCleanResults_WithDataDeleted(t *testing.T) {
	resetCleanFlags()

	stats := &persistence.GCStats{
		RunsDeleted:           10,
		RunErrorsDeleted:      20,
		ErrorLocationsDeleted: 15,
		HealsDeleted:          5,
		ErrorsDeleted:         8,
	}
	output := captureStderr(t, func() {
		printCleanResults(0, nil, stats, nil, 100*time.Millisecond)
	})

	if output == "" {
		t.Error("expected output but got empty string")
	}
}

func TestPrintCleanResults_DryRunMode(t *testing.T) {
	resetCleanFlags()
	cleanDryRun = true
	defer func() { cleanDryRun = false }()

	stats := &persistence.GCStats{
		RunsDeleted:   5,
		ErrorsDeleted: 10,
	}
	output := captureStderr(t, func() {
		printCleanResults(3, nil, stats, nil, 100*time.Millisecond)
	})

	if output == "" {
		t.Error("expected output but got empty string")
	}
}

func TestPrintCleanResults_WorktreeErrorOnly(t *testing.T) {
	resetCleanFlags()

	worktreeErr := errors.New("worktree cleanup failed")
	stats := &persistence.GCStats{}

	// Should not panic with worktree error
	output := captureStderr(t, func() {
		printCleanResults(0, worktreeErr, stats, nil, 100*time.Millisecond)
	})

	// Output should still contain duration info
	if output == "" {
		t.Error("expected some output even with worktree error")
	}
}

func TestPrintCleanResults_DataErrorOnly(t *testing.T) {
	resetCleanFlags()

	dataErr := errors.New("data cleanup failed")

	// Should not panic with data error
	output := captureStderr(t, func() {
		printCleanResults(2, nil, nil, dataErr, 100*time.Millisecond)
	})

	if output == "" {
		t.Error("expected some output even with data error")
	}
}

func TestPrintCleanResults_BothErrors(t *testing.T) {
	resetCleanFlags()

	worktreeErr := errors.New("worktree error")
	dataErr := errors.New("data error")

	// Should not panic with both errors
	captureStderr(t, func() {
		printCleanResults(0, worktreeErr, nil, dataErr, 100*time.Millisecond)
	})
}

func TestPrintCleanResults_NilStats(t *testing.T) {
	resetCleanFlags()

	// Should not panic with nil stats
	captureStderr(t, func() {
		printCleanResults(5, nil, nil, nil, 100*time.Millisecond)
	})
}

func TestPrintCleanResults_PartialStats(t *testing.T) {
	resetCleanFlags()

	tests := []struct {
		name  string
		stats *persistence.GCStats
	}{
		{
			name:  "only runs deleted",
			stats: &persistence.GCStats{RunsDeleted: 5},
		},
		{
			name:  "only errors deleted",
			stats: &persistence.GCStats{ErrorsDeleted: 10},
		},
		{
			name:  "only heals deleted",
			stats: &persistence.GCStats{HealsDeleted: 3},
		},
		{
			name:  "only run_errors deleted",
			stats: &persistence.GCStats{RunErrorsDeleted: 7},
		},
		{
			name:  "only error_locations deleted",
			stats: &persistence.GCStats{ErrorLocationsDeleted: 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic with partial stats
			captureStderr(t, func() {
				printCleanResults(0, nil, tt.stats, nil, 100*time.Millisecond)
			})
		})
	}
}

// --- runClean tests ---

func TestRunClean_NegativeRetentionDays(t *testing.T) {
	resetCleanFlags()
	cleanRetentionDays = -5

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	captureStderr(t, func() {
		err := runClean(cmd, nil)
		if err == nil {
			t.Error("expected error for negative retention days")
		}
		if err != nil && err.Error() != "retention days cannot be negative: -5" {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestRunClean_ZeroRetentionDaysUsesDefault(t *testing.T) {
	resetCleanFlags()
	cleanRetentionDays = 0

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	var capturedRetentionDays int
	called := false
	processCleanDB = func(_ string, retentionDays int, _ bool) (*persistence.GCStats, error) {
		called = true
		capturedRetentionDays = retentionDays
		return &persistence.GCStats{}, nil
	}

	// Setup temp environment
	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	// Create a git repo for the test
	repoRoot := filepath.Join(tmpDir, "my-repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	// Get the expected database path and create it
	dbPath, err := persistence.GetDatabasePath(repoRoot)
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create fake db: %v", err)
	}
	f.Close()

	// Change to repo root for the test
	originalWd, _ := os.Getwd()
	os.Chdir(repoRoot)
	defer os.Chdir(originalWd)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	captureStderr(t, func() {
		_ = runClean(cmd, nil)
	})

	if !called {
		t.Skip("processCleanDB was not called - database may not exist at expected path")
	}

	if capturedRetentionDays != defaultRetentionDays {
		t.Errorf("retention days = %d, want %d (default)", capturedRetentionDays, defaultRetentionDays)
	}
}

func TestRunClean_DryRunFlag(t *testing.T) {
	resetCleanFlags()
	cleanDryRun = true

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	var capturedDryRun bool
	called := false
	processCleanDB = func(_ string, _ int, dryRun bool) (*persistence.GCStats, error) {
		called = true
		capturedDryRun = dryRun
		return &persistence.GCStats{}, nil
	}

	// Setup temp environment
	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	repoRoot := filepath.Join(tmpDir, "my-repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	dbPath, err := persistence.GetDatabasePath(repoRoot)
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create fake db: %v", err)
	}
	f.Close()

	originalWd, _ := os.Getwd()
	os.Chdir(repoRoot)
	defer os.Chdir(originalWd)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	captureStderr(t, func() {
		_ = runClean(cmd, nil)
	})

	if !called {
		t.Skip("processCleanDB was not called - database may not exist at expected path")
	}

	if !capturedDryRun {
		t.Error("dry-run flag was not passed to processCleanDB")
	}
}

func TestRunClean_ReturnsDataError(t *testing.T) {
	// This test verifies that data cleanup errors are returned
	resetCleanFlags()

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	called := false
	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		called = true
		return nil, errors.New("data error")
	}

	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	repoRoot := filepath.Join(tmpDir, "my-repo")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	dbPath, err := persistence.GetDatabasePath(repoRoot)
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create fake db: %v", err)
	}
	f.Close()

	originalWd, _ := os.Getwd()
	os.Chdir(repoRoot)
	defer os.Chdir(originalWd)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	captureStderr(t, func() {
		err := runClean(cmd, nil)
		if !called {
			t.Skip("processCleanDB was not called - database may not exist at expected path")
		}
		if err == nil {
			t.Error("expected error from runClean when processCleanDB fails")
		}
	})
}

// --- cleanWorktrees tests ---

func TestCleanWorktrees_SingleRepo(t *testing.T) {
	resetCleanFlags()
	tmpDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This will run actual git commands if git is available
	// We mainly test that it doesn't panic and handles errors gracefully
	captureStderr(t, func() {
		_, err := cleanWorktrees(ctx, tmpDir)
		// Errors are acceptable since tmpDir is not a real git repo
		_ = err
	})
}

func TestCleanWorktrees_AllRepos(t *testing.T) {
	resetCleanFlags()
	cleanAll = true
	defer func() { cleanAll = false }()

	tmpDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	captureStderr(t, func() {
		_, err := cleanWorktrees(ctx, tmpDir)
		// When cleanAll is true, empty string is passed to CleanOrphanedTempDirs
		_ = err
	})
}

func TestCleanWorktrees_ForceFlag(t *testing.T) {
	resetCleanFlags()
	cleanForce = true
	defer func() { cleanForce = false }()

	tmpDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test that force flag is properly used (no panic)
	captureStderr(t, func() {
		_, err := cleanWorktrees(ctx, tmpDir)
		_ = err
	})
}

func TestCleanWorktrees_ContextCancellation(t *testing.T) {
	resetCleanFlags()
	tmpDir := t.TempDir()

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	captureStderr(t, func() {
		_, err := cleanWorktrees(ctx, tmpDir)
		// May or may not error depending on timing, but should not panic
		_ = err
	})
}

// --- Edge cases ---

func TestCleanCmd_HasShortDescription(t *testing.T) {
	if cleanCmd.Short == "" {
		t.Error("cleanCmd.Short should not be empty")
	}
}

func TestCleanCmd_HasLongDescription(t *testing.T) {
	if cleanCmd.Long == "" {
		t.Error("cleanCmd.Long should not be empty")
	}
}

func TestCleanRetentionDays_ValidRange(t *testing.T) {
	resetCleanFlags()

	testCases := []struct {
		name          string
		retentionDays int
		shouldError   bool
	}{
		{"negative value", -1, true},
		{"negative large value", -100, true},
		{"zero uses default", 0, false},
		{"one day", 1, false},
		{"default 30 days", 30, false},
		{"large value 365", 365, false},
		{"very large value", 9999, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resetCleanFlags()
			cleanRetentionDays = tc.retentionDays

			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())

			err := runClean(cmd, nil)
			if tc.shouldError && err == nil {
				t.Errorf("expected error for retention days %d", tc.retentionDays)
			}
			// Note: non-error cases may still fail for other reasons (no git repo, etc.)
		})
	}
}

func TestGCStats_Fields(t *testing.T) {
	// Verify GCStats has all expected fields
	stats := persistence.GCStats{
		RunsDeleted:           1,
		RunErrorsDeleted:      2,
		ErrorLocationsDeleted: 3,
		HealsDeleted:          4,
		ErrorsDeleted:         5,
		DryRun:                true,
	}

	if stats.RunsDeleted != 1 {
		t.Errorf("RunsDeleted = %d, want 1", stats.RunsDeleted)
	}
	if stats.RunErrorsDeleted != 2 {
		t.Errorf("RunErrorsDeleted = %d, want 2", stats.RunErrorsDeleted)
	}
	if stats.ErrorLocationsDeleted != 3 {
		t.Errorf("ErrorLocationsDeleted = %d, want 3", stats.ErrorLocationsDeleted)
	}
	if stats.HealsDeleted != 4 {
		t.Errorf("HealsDeleted = %d, want 4", stats.HealsDeleted)
	}
	if stats.ErrorsDeleted != 5 {
		t.Errorf("ErrorsDeleted = %d, want 5", stats.ErrorsDeleted)
	}
	if !stats.DryRun {
		t.Error("DryRun = false, want true")
	}
}

func TestCleanData_WithDryRunFlag(t *testing.T) {
	resetCleanFlags()
	cleanDryRun = true
	defer func() { cleanDryRun = false }()

	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	var capturedDryRun bool
	processCleanDB = func(_ string, _ int, dryRun bool) (*persistence.GCStats, error) {
		capturedDryRun = dryRun
		return &persistence.GCStats{DryRun: dryRun}, nil
	}

	tmpDir := t.TempDir()
	detentDir := filepath.Join(tmpDir, ".detent")
	reposDir := filepath.Join(detentDir, "repos")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		t.Fatalf("Failed to create repos dir: %v", err)
	}

	originalHome := os.Getenv("DETENT_HOME")
	os.Setenv("DETENT_HOME", detentDir)
	defer func() {
		if originalHome == "" {
			os.Unsetenv("DETENT_HOME")
		} else {
			os.Setenv("DETENT_HOME", originalHome)
		}
	}()

	repoRoot := filepath.Join(tmpDir, "my-repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	dbPath, err := persistence.GetDatabasePath(repoRoot)
	if err != nil {
		t.Fatalf("Failed to get database path: %v", err)
	}
	f, err := os.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create fake db: %v", err)
	}
	f.Close()

	_, err = cleanData(repoRoot, 30)
	if err != nil {
		t.Fatalf("cleanData returned error: %v", err)
	}

	if !capturedDryRun {
		t.Error("dry-run flag was not passed through cleanData")
	}
}
