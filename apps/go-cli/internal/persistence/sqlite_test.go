package persistence

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// setupTestDetentHome sets DETENT_HOME to a temp directory for test isolation.
// Returns a cleanup function that restores the original value.
func setupTestDetentHome(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	original := os.Getenv(DetentHomeEnv)
	if err := os.Setenv(DetentHomeEnv, tmpDir); err != nil {
		t.Fatalf("Failed to set %s: %v", DetentHomeEnv, err)
	}
	t.Cleanup(func() {
		if original == "" {
			os.Unsetenv(DetentHomeEnv)
		} else {
			os.Setenv(DetentHomeEnv, original)
		}
		// Clear repo ID cache to avoid cross-test pollution
		repoIDCache = sync.Map{}
	})
}

// TestNewSQLiteWriter tests the creation of a new SQLite writer
func TestNewSQLiteWriter(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()

	// Create a git repo in tmpDir for ComputeRepoID to work
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	writer, err := NewSQLiteWriter(repoDir)
	if err != nil {
		t.Fatalf("NewSQLiteWriter() error = %v", err)
	}

	if writer == nil {
		t.Fatal("Expected non-nil writer")
	}

	defer func() {
		if closeErr := writer.Close(); closeErr != nil {
			t.Errorf("Failed to close writer: %v", closeErr)
		}
	}()

	// Get the expected detent directory (~/.detent)
	detentDir, err := GetDetentDir()
	if err != nil {
		t.Fatalf("Failed to get detent dir: %v", err)
	}

	// Verify repos directory was created under ~/.detent
	reposDir := filepath.Join(detentDir, "repos")
	if _, err := os.Stat(reposDir); os.IsNotExist(err) {
		t.Error("repos directory was not created")
	}

	// Verify database file was created (path is ~/.detent/repos/<repoID>.db)
	dbPath := writer.Path()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created at %s", dbPath)
	}

	// Verify path is under the repos directory and ends with .db
	if !filepath.HasPrefix(dbPath, reposDir) {
		t.Errorf("Path() = %v, expected to be under %v", dbPath, reposDir)
	}
	if filepath.Ext(dbPath) != ".db" {
		t.Errorf("Path() = %v, expected to end with .db", dbPath)
	}
}

// TestSQLiteWriter_initSchema tests schema initialization
func TestSQLiteWriter_initSchema(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Verify runs table exists
	var tableName string
	err = writer.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='runs'").Scan(&tableName)
	if err != nil {
		t.Errorf("runs table not created: %v", err)
	}
	if tableName != "runs" {
		t.Errorf("Expected table name 'runs', got %v", tableName)
	}

	// Verify errors table exists
	err = writer.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='errors'").Scan(&tableName)
	if err != nil {
		t.Errorf("errors table not created: %v", err)
	}
	if tableName != "errors" {
		t.Errorf("Expected table name 'errors', got %v", tableName)
	}

	// Verify key indices exist (note: some indices were dropped in migration v6)
	indices := []string{
		"idx_errors_content_hash",
		"idx_errors_file_path",
	}

	for _, indexName := range indices {
		var name string
		err = writer.db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", indexName).Scan(&name)
		if err != nil {
			t.Errorf("Index %s not created: %v", indexName, err)
		}
	}
}

// TestSQLiteWriter_RecordRun tests run recording
func TestSQLiteWriter_RecordRun(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "run-123"
	err = writer.RecordRun(runID, "CI", "abc123", "tree123", "github")
	if err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	// Verify run was recorded
	var workflowName, commitSHA, execMode string
	var startedAt int64
	query := "SELECT workflow_name, commit_sha, execution_mode, started_at FROM runs WHERE run_id = ?"
	err = writer.db.QueryRow(query, runID).Scan(&workflowName, &commitSHA, &execMode, &startedAt)
	if err != nil {
		t.Fatalf("Failed to query run: %v", err)
	}

	if workflowName != "CI" {
		t.Errorf("workflow_name = %v, want 'CI'", workflowName)
	}
	if commitSHA != "abc123" {
		t.Errorf("commit_sha = %v, want 'abc123'", commitSHA)
	}
	if execMode != "github" {
		t.Errorf("execution_mode = %v, want 'github'", execMode)
	}
	if startedAt == 0 {
		t.Error("started_at should not be zero")
	}
}

// TestSQLiteWriter_RunExists tests the RunExists method
func TestSQLiteWriter_RunExists(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run-exists"

	// Test: run doesn't exist yet
	exists, err := writer.RunExists(runID)
	if err != nil {
		t.Fatalf("RunExists() error = %v", err)
	}
	if exists {
		t.Error("RunExists() = true, want false for non-existent run")
	}

	// Record the run
	if err := writer.RecordRun(runID, "CI", "abc123", "tree123", "github"); err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	// Test: run now exists
	exists, err = writer.RunExists(runID)
	if err != nil {
		t.Fatalf("RunExists() error = %v", err)
	}
	if !exists {
		t.Error("RunExists() = false, want true for existing run")
	}

	// Test: different run ID doesn't exist
	exists, err = writer.RunExists("non-existent-run")
	if err != nil {
		t.Fatalf("RunExists() error = %v", err)
	}
	if exists {
		t.Error("RunExists() = true, want false for non-existent run")
	}
}

// TestSQLiteWriter_RunExists_EmptyDatabase tests RunExists on fresh database
func TestSQLiteWriter_RunExists_EmptyDatabase(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Test: empty database returns false, no error
	exists, err := writer.RunExists("any-run-id")
	if err != nil {
		t.Fatalf("RunExists() error = %v", err)
	}
	if exists {
		t.Error("RunExists() = true, want false for empty database")
	}
}

// TestSQLiteWriter_RunExists_EmptyRunID tests RunExists with empty string runID
func TestSQLiteWriter_RunExists_EmptyRunID(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Test: empty runID returns false, no error
	exists, err := writer.RunExists("")
	if err != nil {
		t.Fatalf("RunExists('') error = %v", err)
	}
	if exists {
		t.Error("RunExists('') = true, want false for empty runID")
	}

	// Record a run with empty ID (edge case - should work)
	if err := writer.RecordRun("", "CI", "abc123", "tree123", "github"); err != nil {
		t.Fatalf("RecordRun('') error = %v", err)
	}

	// Now it should exist
	exists, err = writer.RunExists("")
	if err != nil {
		t.Fatalf("RunExists('') after record error = %v", err)
	}
	if !exists {
		t.Error("RunExists('') = false, want true after recording")
	}
}

// TestSQLiteWriter_RunExists_VeryLongRunID tests RunExists with very long runID
func TestSQLiteWriter_RunExists_VeryLongRunID(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create a very long runID (10KB)
	longRunID := make([]byte, 10*1024)
	for i := range longRunID {
		longRunID[i] = 'a' + byte(i%26)
	}
	runID := string(longRunID)

	// Test: long runID returns false initially
	exists, err := writer.RunExists(runID)
	if err != nil {
		t.Fatalf("RunExists(longRunID) error = %v", err)
	}
	if exists {
		t.Error("RunExists(longRunID) = true, want false for non-existent run")
	}

	// Record the run with long ID
	if err := writer.RecordRun(runID, "CI", "abc123", "tree123", "github"); err != nil {
		t.Fatalf("RecordRun(longRunID) error = %v", err)
	}

	// Now it should exist
	exists, err = writer.RunExists(runID)
	if err != nil {
		t.Fatalf("RunExists(longRunID) after record error = %v", err)
	}
	if !exists {
		t.Error("RunExists(longRunID) = false, want true after recording")
	}
}

// TestSQLiteWriter_RunExists_SpecialCharacters tests RunExists with special characters in runID
func TestSQLiteWriter_RunExists_SpecialCharacters(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	specialRunIDs := []string{
		"run-with-dashes",
		"run_with_underscores",
		"run.with.dots",
		"run/with/slashes",
		"run\\with\\backslashes",
		"run with spaces",
		"run\twith\ttabs",
		"run\nwith\nnewlines",
		"run'with'quotes",
		"run\"with\"doublequotes",
		"run;with;semicolons",
		"run--with--sql--comment",
		"run/*with*/comment",
		"run%like%wildcards",
		"run\x00with\x00nulls",
		"run🎉with🎉emoji",
		"日本語runID",
	}

	for _, runID := range specialRunIDs {
		t.Run(runID, func(t *testing.T) {
			// Test: doesn't exist initially
			exists, err := writer.RunExists(runID)
			if err != nil {
				t.Fatalf("RunExists(%q) error = %v", runID, err)
			}
			if exists {
				t.Errorf("RunExists(%q) = true, want false", runID)
			}

			// Record the run
			if err := writer.RecordRun(runID, "CI", "abc123", "tree123", "github"); err != nil {
				t.Fatalf("RecordRun(%q) error = %v", runID, err)
			}

			// Now it should exist
			exists, err = writer.RunExists(runID)
			if err != nil {
				t.Fatalf("RunExists(%q) after record error = %v", runID, err)
			}
			if !exists {
				t.Errorf("RunExists(%q) = false, want true after recording", runID)
			}
		})
	}
}

// TestSQLiteWriter_RunExists_Concurrent tests concurrent RunExists calls
func TestSQLiteWriter_RunExists_Concurrent(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create some runs
	runIDs := make([]string, 100)
	for i := 0; i < 100; i++ {
		runIDs[i] = fmt.Sprintf("concurrent-run-%d", i)
		if i%2 == 0 {
			// Only record even-numbered runs
			if err := writer.RecordRun(runIDs[i], "CI", "abc123", "tree123", "github"); err != nil {
				t.Fatalf("RecordRun(%s) error = %v", runIDs[i], err)
			}
		}
	}

	// Run concurrent RunExists calls
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			runID := runIDs[idx]
			expectedExists := idx%2 == 0

			exists, err := writer.RunExists(runID)
			if err != nil {
				errors <- fmt.Errorf("RunExists(%s) error = %v", runID, err)
				return
			}
			if exists != expectedExists {
				errors <- fmt.Errorf("RunExists(%s) = %v, want %v", runID, exists, expectedExists)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestSQLiteWriter_GetErrorsByRunID tests the full cache flow: record → retrieve
func TestSQLiteWriter_GetErrorsByRunID(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-cache-flow"

	// Record run
	if err := writer.RecordRun(runID, "CI", "abc123", "tree123", "github"); err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	// Record some errors
	findings := []*FindingRecord{
		{RunID: runID, FilePath: "src/main.go", Line: 10, Message: "unused variable", Severity: "error"},
		{RunID: runID, FilePath: "src/main.go", Line: 20, Message: "missing return", Severity: "error"},
		{RunID: runID, FilePath: "src/util.go", Line: 5, Message: "undefined func", Severity: "warning"},
	}
	if err := writer.RecordFindings(findings); err != nil {
		t.Fatalf("RecordFindings() error = %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	// Retrieve errors by run ID
	errors, err := writer.GetErrorsByRunID(runID)
	if err != nil {
		t.Fatalf("GetErrorsByRunID() error = %v", err)
	}

	if len(errors) != 3 {
		t.Errorf("GetErrorsByRunID() returned %d errors, want 3", len(errors))
	}

	// Verify error details
	foundMain10 := false
	foundMain20 := false
	foundUtil5 := false
	for _, e := range errors {
		if e.FilePath == "src/main.go" && e.LineNumber == 10 {
			foundMain10 = true
		}
		if e.FilePath == "src/main.go" && e.LineNumber == 20 {
			foundMain20 = true
		}
		if e.FilePath == "src/util.go" && e.LineNumber == 5 {
			foundUtil5 = true
		}
	}
	if !foundMain10 || !foundMain20 || !foundUtil5 {
		t.Errorf("Missing expected errors: main10=%v main20=%v util5=%v", foundMain10, foundMain20, foundUtil5)
	}

	// Test: different run ID returns empty
	otherErrors, err := writer.GetErrorsByRunID("other-run")
	if err != nil {
		t.Fatalf("GetErrorsByRunID(other) error = %v", err)
	}
	if len(otherErrors) != 0 {
		t.Errorf("GetErrorsByRunID(other) returned %d errors, want 0", len(otherErrors))
	}
}

// TestSQLiteWriter_GetErrorsByRunID_EmptyRun tests retrieving errors for run with no errors
func TestSQLiteWriter_GetErrorsByRunID_EmptyRun(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "empty-run"

	// Record run but no errors
	if err := writer.RecordRun(runID, "CI", "abc123", "tree123", "github"); err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	// Retrieve should return empty slice, no error
	errors, err := writer.GetErrorsByRunID(runID)
	if err != nil {
		t.Fatalf("GetErrorsByRunID() error = %v", err)
	}
	if len(errors) != 0 {
		t.Errorf("GetErrorsByRunID() returned %d errors, want 0", len(errors))
	}
}

// TestSQLiteWriter_GetErrorsByRunID_EmptyRunID tests GetErrorsByRunID with empty string
func TestSQLiteWriter_GetErrorsByRunID_EmptyRunID(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Query with empty runID - should return no errors (nil or empty slice is acceptable)
	errors, err := writer.GetErrorsByRunID("")
	if err != nil {
		t.Fatalf("GetErrorsByRunID('') error = %v", err)
	}
	if len(errors) != 0 {
		t.Errorf("GetErrorsByRunID('') returned %d errors, want 0", len(errors))
	}
}

// TestSQLiteWriter_GetErrorsByRunID_SpecialCharacters tests GetErrorsByRunID with special characters
func TestSQLiteWriter_GetErrorsByRunID_SpecialCharacters(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// runID with SQL injection attempt
	runID := "run'; DROP TABLE errors; --"

	// Record run
	if err := writer.RecordRun(runID, "CI", "abc123", "tree123", "github"); err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	// Record error for this run
	finding := &FindingRecord{
		RunID:    runID,
		Message:  "test error",
		FilePath: "/app/test.go",
		Line:     10,
		Category: "compile",
	}
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() error = %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	// Retrieve - should work correctly despite special characters
	errors, err := writer.GetErrorsByRunID(runID)
	if err != nil {
		t.Fatalf("GetErrorsByRunID() error = %v", err)
	}
	if len(errors) != 1 {
		t.Errorf("GetErrorsByRunID() returned %d errors, want 1", len(errors))
	}

	// Verify the errors table still exists (SQL injection did not work)
	var tableName string
	tableErr := writer.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='errors'").Scan(&tableName)
	if tableErr != nil {
		t.Fatalf("errors table was dropped by SQL injection: %v", tableErr)
	}
}

// TestSQLiteWriter_GetErrorsByRunID_Concurrent tests concurrent GetErrorsByRunID calls
func TestSQLiteWriter_GetErrorsByRunID_Concurrent(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create a run with some errors
	runID := "concurrent-cache-test"
	if err := writer.RecordRun(runID, "CI", "abc123", "tree123", "github"); err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	// Record 10 distinct errors
	for i := 0; i < 10; i++ {
		finding := &FindingRecord{
			RunID:    runID,
			Message:  fmt.Sprintf("concurrent error %d", i),
			FilePath: fmt.Sprintf("/app/file%d.go", i),
			Line:     i + 1,
			Category: "compile",
		}
		if err := writer.RecordError(finding); err != nil {
			t.Fatalf("RecordError() error = %v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	// Run 50 concurrent GetErrorsByRunID calls
	var wg sync.WaitGroup
	errChan := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors, err := writer.GetErrorsByRunID(runID)
			if err != nil {
				errChan <- fmt.Errorf("GetErrorsByRunID() error = %v", err)
				return
			}
			if len(errors) != 10 {
				errChan <- fmt.Errorf("GetErrorsByRunID() returned %d errors, want 10", len(errors))
			}
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Error(err)
	}
}

// TestSQLiteWriter_RecordError_WithFlush tests error recording with manual flush
func TestSQLiteWriter_RecordError_WithFlush(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	finding := &FindingRecord{
		RunID:    runID,
		Message:  "test error message",
		FilePath: "/app/main.go",
		Line:     42,
		Category: "compile",
	}

	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed: %v", err)
	}

	// Flush batch to write to database
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// Verify error was recorded
	contentHash := ComputeContentHash(finding.Message)
	var seenCount int
	query := "SELECT seen_count FROM errors WHERE content_hash = ?"
	err = writer.db.QueryRow(query, contentHash).Scan(&seenCount)
	if err != nil {
		t.Fatalf("Failed to query error: %v", err)
	}

	if seenCount != 1 {
		t.Errorf("seen_count = %v, want 1", seenCount)
	}
}

// TestSQLiteWriter_RecordError_Deduplication tests deduplication logic
func TestSQLiteWriter_RecordError_Deduplication(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	finding := &FindingRecord{
		RunID:    runID,
		Message:  "TypeError: cannot read property 'foo' of undefined",
		FilePath: "/app/index.js",
		Line:     25,
		Category: "runtime",
	}

	// Record 5 times
	for i := 0; i < 5; i++ {
		if err := writer.RecordError(finding); err != nil {
			t.Fatalf("RecordError() failed on iteration %d: %v", i, err)
		}
	}

	// Flush to write to database
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// Verify only one error record exists with seen_count = 5
	contentHash := ComputeContentHash(finding.Message)
	var count int
	err = writer.db.QueryRow("SELECT COUNT(*) FROM errors WHERE content_hash = ?", contentHash).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query error count: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 error record, got %d", count)
	}

	var seenCount int
	err = writer.db.QueryRow("SELECT seen_count FROM errors WHERE content_hash = ?", contentHash).Scan(&seenCount)
	if err != nil {
		t.Fatalf("Failed to query seen_count: %v", err)
	}
	if seenCount != 5 {
		t.Errorf("seen_count = %d, want 5", seenCount)
	}
}

// TestSQLiteWriter_FinalizeRun tests run finalization
func TestSQLiteWriter_FinalizeRun(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "run-with-errors"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	// Add 3 errors
	for i := 0; i < 3; i++ {
		finding := &FindingRecord{
			RunID:    runID,
			Message:  "error",
			FilePath: "/app/test.go",
			Line:     i,
			Category: "test",
		}
		if err := writer.RecordError(finding); err != nil {
			t.Fatalf("RecordError() failed: %v", err)
		}
	}

	// Finalize (this should flush the batch)
	if err := writer.FinalizeRun(runID, 3); err != nil {
		t.Fatalf("FinalizeRun() failed: %v", err)
	}

	// Verify run was finalized
	var completedAt int64
	var totalErrors int
	query := "SELECT completed_at, total_errors FROM runs WHERE run_id = ?"
	err = writer.db.QueryRow(query, runID).Scan(&completedAt, &totalErrors)
	if err != nil {
		t.Fatalf("Failed to query run: %v", err)
	}

	if completedAt == 0 {
		t.Error("completed_at should not be zero")
	}
	if totalErrors != 3 {
		t.Errorf("total_errors = %d, want 3", totalErrors)
	}
}

// TestSQLiteWriter_ErrorFields tests that all error fields are properly stored
func TestSQLiteWriter_ErrorFields(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	finding := &FindingRecord{
		RunID:      runID,
		Message:    "complete error message",
		FilePath:   "/app/complete.go",
		Line:       42,
		Category:   "compile",
		StackTrace: "stack trace line 1\nstack trace line 2",
		FileHash:   "abc123hash",
	}

	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed: %v", err)
	}

	// Flush to database
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// Query and verify all fields (note: schema uses error_type, not category)
	contentHash := ComputeContentHash(finding.Message)
	var (
		filePath, message, stackTrace, fileHash, errorType, status string
		line                                                        int
		firstSeenAt, lastSeenAt                                     int64
	)

	query := `
		SELECT file_path, line_number, message, stack_trace, file_hash,
		       error_type, status, first_seen_at, last_seen_at
		FROM errors WHERE content_hash = ?
	`
	err = writer.db.QueryRow(query, contentHash).Scan(
		&filePath, &line, &message, &stackTrace, &fileHash,
		&errorType, &status, &firstSeenAt, &lastSeenAt,
	)
	if err != nil {
		t.Fatalf("Failed to query error: %v", err)
	}

	if filePath != finding.FilePath {
		t.Errorf("file_path = %v, want %v", filePath, finding.FilePath)
	}
	if line != finding.Line {
		t.Errorf("line_number = %v, want %v", line, finding.Line)
	}
	if message != finding.Message {
		t.Errorf("message = %v, want %v", message, finding.Message)
	}
	if stackTrace != finding.StackTrace {
		t.Errorf("stack_trace = %v, want %v", stackTrace, finding.StackTrace)
	}
	if fileHash != finding.FileHash {
		t.Errorf("file_hash = %v, want %v", fileHash, finding.FileHash)
	}
	if errorType != finding.Category {
		t.Errorf("error_type = %v, want %v", errorType, finding.Category)
	}
	if status != "open" {
		t.Errorf("status = %v, want 'open'", status)
	}
	if firstSeenAt == 0 {
		t.Error("first_seen_at should not be zero")
	}
	if lastSeenAt == 0 {
		t.Error("last_seen_at should not be zero")
	}
}

// TestSQLiteWriter_FlushBatch tests manual batch flushing
func TestSQLiteWriter_FlushBatch(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	// Add some errors
	for i := 0; i < 10; i++ {
		finding := &FindingRecord{
			RunID:    runID,
			Message:  "batch test error",
			FilePath: "/app/test.go",
			Line:     i,
			Category: "test",
		}
		if err := writer.RecordError(finding); err != nil {
			t.Fatalf("RecordError() failed: %v", err)
		}
	}

	// Before flush, no errors should be in DB
	var count int
	contentHash := ComputeContentHash("batch test error")
	err = writer.db.QueryRow("SELECT COUNT(*) FROM errors WHERE content_hash = ?", contentHash).Scan(&count)
	if err == nil && count > 0 {
		t.Errorf("Expected 0 errors before flush, got %d", count)
	}

	// Flush batch
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// After flush, errors should be in DB
	var seenCount int
	err = writer.db.QueryRow("SELECT seen_count FROM errors WHERE content_hash = ?", contentHash).Scan(&seenCount)
	if err != nil {
		t.Fatalf("Failed to query after flush: %v", err)
	}
	if seenCount != 10 {
		t.Errorf("seen_count = %d, want 10", seenCount)
	}
}

// TestSQLiteWriter_LastSeenAtUpdates tests that last_seen_at is updated correctly
func TestSQLiteWriter_LastSeenAtUpdates(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "timestamp-test"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	finding := &FindingRecord{
		RunID:    runID,
		Message:  "timestamp test error",
		FilePath: "/app/test.go",
		Line:     1,
		Category: "test",
	}

	// Record first time
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed: %v", err)
	}
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// Get first timestamps and seen_count
	contentHash := ComputeContentHash(finding.Message)
	var firstSeenAt1, lastSeenAt1 int64
	var seenCount1 int
	query := "SELECT first_seen_at, last_seen_at, seen_count FROM errors WHERE content_hash = ?"
	err = writer.db.QueryRow(query, contentHash).Scan(&firstSeenAt1, &lastSeenAt1, &seenCount1)
	if err != nil {
		t.Fatalf("Failed to query timestamps: %v", err)
	}

	if seenCount1 != 1 {
		t.Errorf("Initial seen_count = %d, want 1", seenCount1)
	}

	// Record again (without sleep - we'll verify the update logic via seen_count)
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed on second attempt: %v", err)
	}
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// Get updated timestamps and seen_count
	var firstSeenAt2, lastSeenAt2 int64
	var seenCount2 int
	err = writer.db.QueryRow(query, contentHash).Scan(&firstSeenAt2, &lastSeenAt2, &seenCount2)
	if err != nil {
		t.Fatalf("Failed to query timestamps: %v", err)
	}

	// first_seen_at should not change
	if firstSeenAt1 != firstSeenAt2 {
		t.Errorf("first_seen_at changed from %d to %d", firstSeenAt1, firstSeenAt2)
	}

	// seen_count should be incremented (proves the update path was taken)
	if seenCount2 != 2 {
		t.Errorf("seen_count = %d, want 2", seenCount2)
	}

	// last_seen_at should be >= first value (may be same if within same second)
	if lastSeenAt2 < lastSeenAt1 {
		t.Errorf("last_seen_at went backwards: %d < %d", lastSeenAt2, lastSeenAt1)
	}
}

// TestCreateDirIfNotExists tests directory creation helper
func TestCreateDirIfNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "test-dir")

	// Create directory
	err := createDirIfNotExists(testPath)
	if err != nil {
		t.Fatalf("createDirIfNotExists() error = %v", err)
	}

	// Verify directory exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}

	// Call again on existing directory
	err = createDirIfNotExists(testPath)
	if err != nil {
		t.Errorf("createDirIfNotExists() should not error on existing directory, got: %v", err)
	}
}

// TestSQLiteWriter_SchemaMigration tests schema versioning and migration
func TestSQLiteWriter_SchemaMigration(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()

	// Create initial writer (should apply all migrations)
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Verify schema_version table exists
	var tableName string
	err = writer.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&tableName)
	if err != nil {
		t.Fatalf("schema_version table not created: %v", err)
	}

	// Verify current schema version is recorded
	var version int
	err = writer.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
	if err != nil {
		t.Fatalf("Failed to query schema version: %v", err)
	}
	if version != currentSchemaVersion {
		t.Errorf("Schema version = %d, want %d", version, currentSchemaVersion)
	}

	// Verify is_dirty column exists in runs table (always 0 now since we require clean commits)
	var isDirty int
	runID := "migration-test"
	err = writer.RecordRun(runID, "test", "abc123", "", "github")
	if err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	query := "SELECT is_dirty FROM runs WHERE run_id = ?"
	err = writer.db.QueryRow(query, runID).Scan(&isDirty)
	if err != nil {
		t.Fatalf("Failed to query is_dirty: %v", err)
	}

	if isDirty != 0 {
		t.Errorf("is_dirty = %d, want 0 (clean commits required)", isDirty)
	}

	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("Failed to close writer: %v", closeErr)
	}

	// Reopen database to verify schema persists
	writer2, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer func() { _ = writer2.Close() }()

	// Verify version is still current
	var version2 int
	err = writer2.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version2)
	if err != nil {
		t.Fatalf("Failed to query schema version after reopen: %v", err)
	}
	if version2 != currentSchemaVersion {
		t.Errorf("Schema version after reopen = %d, want %d", version2, currentSchemaVersion)
	}
}

// TestSQLiteWriter_GetPendingHealByFileHash tests heal caching lookup by file hash
func TestSQLiteWriter_GetPendingHealByFileHash(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "heal-cache-test"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	// Record an error and get its error_id
	finding := &FindingRecord{
		RunID:    runID,
		Message:  "test error for heal caching",
		FilePath: "/app/main.go",
		Line:     42,
		Category: "compile",
		FileHash: "filehash123",
	}
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed: %v", err)
	}
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// Get the error_id
	contentHash := ComputeContentHash(finding.Message)
	var errorID string
	err = writer.db.QueryRow("SELECT error_id FROM errors WHERE content_hash = ?", contentHash).Scan(&errorID)
	if err != nil {
		t.Fatalf("Failed to get error_id: %v", err)
	}

	// Record a pending heal with file_hash
	heal := &HealRecord{
		HealID:        "heal-001",
		ErrorID:       errorID,
		RunID:         runID,
		DiffContent:   "--- a/main.go\n+++ b/main.go\n@@ -42,1 +42,1 @@\n-bad code\n+good code",
		FilePath:      "/app/main.go",
		FileHash:      "filehash123",
		Status:        HealStatusPending,
		CreatedAt:     time.Now(),
		AttemptNumber: 1,
	}
	if err := writer.RecordHeal(heal); err != nil {
		t.Fatalf("RecordHeal() failed: %v", err)
	}

	// Test: query returns heal when file_hash matches
	result, err := writer.GetPendingHealByFileHash("/app/main.go", "filehash123")
	if err != nil {
		t.Fatalf("GetPendingHealByFileHash() error = %v", err)
	}
	if result == nil {
		t.Fatal("Expected heal to be returned, got nil")
	}
	if result.HealID != "heal-001" {
		t.Errorf("HealID = %v, want 'heal-001'", result.HealID)
	}
	if result.FileHash != "filehash123" {
		t.Errorf("FileHash = %v, want 'filehash123'", result.FileHash)
	}
	if result.Status != HealStatusPending {
		t.Errorf("Status = %v, want 'pending'", result.Status)
	}

	// Test: query returns nil when file_hash differs
	result, err = writer.GetPendingHealByFileHash("/app/main.go", "differenthash")
	if err != nil {
		t.Fatalf("GetPendingHealByFileHash() error = %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil for different file_hash, got heal_id=%s", result.HealID)
	}

	// Test: query returns nil when file_path differs
	result, err = writer.GetPendingHealByFileHash("/app/other.go", "filehash123")
	if err != nil {
		t.Fatalf("GetPendingHealByFileHash() error = %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil for different file_path, got heal_id=%s", result.HealID)
	}
}

// TestSQLiteWriter_GetPendingHealByFileHash_StatusFilter tests that only pending heals are returned
func TestSQLiteWriter_GetPendingHealByFileHash_StatusFilter(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "status-filter-test"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	// Create an error
	finding := &FindingRecord{
		RunID:    runID,
		Message:  "status filter test error",
		FilePath: "/app/status.go",
		Line:     10,
		Category: "compile",
	}
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed: %v", err)
	}
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	contentHash := ComputeContentHash(finding.Message)
	var errorID string
	err = writer.db.QueryRow("SELECT error_id FROM errors WHERE content_hash = ?", contentHash).Scan(&errorID)
	if err != nil {
		t.Fatalf("Failed to get error_id: %v", err)
	}

	// Record an applied heal (not pending)
	appliedHeal := &HealRecord{
		HealID:        "heal-applied",
		ErrorID:       errorID,
		RunID:         runID,
		DiffContent:   "diff content",
		FilePath:      "/app/status.go",
		FileHash:      "statushash",
		Status:        HealStatusApplied,
		CreatedAt:     time.Now(),
		AttemptNumber: 1,
	}
	if err := writer.RecordHeal(appliedHeal); err != nil {
		t.Fatalf("RecordHeal() failed: %v", err)
	}

	// Test: query returns nil for applied heal
	result, err := writer.GetPendingHealByFileHash("/app/status.go", "statushash")
	if err != nil {
		t.Fatalf("GetPendingHealByFileHash() error = %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil for applied heal, got heal_id=%s", result.HealID)
	}

	// Record a pending heal
	pendingHeal := &HealRecord{
		HealID:        "heal-pending",
		ErrorID:       errorID,
		RunID:         runID,
		DiffContent:   "pending diff",
		FilePath:      "/app/status.go",
		FileHash:      "statushash",
		Status:        HealStatusPending,
		CreatedAt:     time.Now(),
		AttemptNumber: 2,
	}
	if err := writer.RecordHeal(pendingHeal); err != nil {
		t.Fatalf("RecordHeal() failed: %v", err)
	}

	// Test: query now returns the pending heal
	result, err = writer.GetPendingHealByFileHash("/app/status.go", "statushash")
	if err != nil {
		t.Fatalf("GetPendingHealByFileHash() error = %v", err)
	}
	if result == nil {
		t.Fatal("Expected pending heal to be returned")
	}
	if result.HealID != "heal-pending" {
		t.Errorf("HealID = %v, want 'heal-pending'", result.HealID)
	}
}

// TestSQLiteWriter_GetPendingHealByFileHash_MostRecent tests that the most recent pending heal is returned
func TestSQLiteWriter_GetPendingHealByFileHash_MostRecent(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "most-recent-test"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	// Create an error
	finding := &FindingRecord{
		RunID:    runID,
		Message:  "most recent test error",
		FilePath: "/app/recent.go",
		Line:     20,
		Category: "lint",
	}
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed: %v", err)
	}
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	contentHash := ComputeContentHash(finding.Message)
	var errorID string
	err = writer.db.QueryRow("SELECT error_id FROM errors WHERE content_hash = ?", contentHash).Scan(&errorID)
	if err != nil {
		t.Fatalf("Failed to get error_id: %v", err)
	}

	// Record first pending heal
	heal1 := &HealRecord{
		HealID:        "heal-old",
		ErrorID:       errorID,
		RunID:         runID,
		DiffContent:   "old diff",
		FilePath:      "/app/recent.go",
		FileHash:      "recenthash",
		Status:        HealStatusPending,
		CreatedAt:     time.Now().Add(-time.Hour),
		AttemptNumber: 1,
	}
	if err := writer.RecordHeal(heal1); err != nil {
		t.Fatalf("RecordHeal() failed: %v", err)
	}

	// Record second (more recent) pending heal
	heal2 := &HealRecord{
		HealID:        "heal-new",
		ErrorID:       errorID,
		RunID:         runID,
		DiffContent:   "new diff",
		FilePath:      "/app/recent.go",
		FileHash:      "recenthash",
		Status:        HealStatusPending,
		CreatedAt:     time.Now(),
		AttemptNumber: 2,
	}
	if err := writer.RecordHeal(heal2); err != nil {
		t.Fatalf("RecordHeal() failed: %v", err)
	}

	// Test: should return the most recent pending heal
	result, err := writer.GetPendingHealByFileHash("/app/recent.go", "recenthash")
	if err != nil {
		t.Fatalf("GetPendingHealByFileHash() error = %v", err)
	}
	if result == nil {
		t.Fatal("Expected heal to be returned")
	}
	if result.HealID != "heal-new" {
		t.Errorf("HealID = %v, want 'heal-new' (most recent)", result.HealID)
	}
}

// TestSQLiteWriter_RecordHeal_WithFileHash tests that file_hash is properly stored
func TestSQLiteWriter_RecordHeal_WithFileHash(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "file-hash-test"
	if err := writer.RecordRun(runID, "test", "abc123", "", "github"); err != nil {
		t.Fatalf("Failed to record run: %v", err)
	}

	// Create an error
	finding := &FindingRecord{
		RunID:    runID,
		Message:  "file hash storage test",
		FilePath: "/app/hash.go",
		Line:     5,
		Category: "type-check",
	}
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed: %v", err)
	}
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	contentHash := ComputeContentHash(finding.Message)
	var errorID string
	err = writer.db.QueryRow("SELECT error_id FROM errors WHERE content_hash = ?", contentHash).Scan(&errorID)
	if err != nil {
		t.Fatalf("Failed to get error_id: %v", err)
	}

	// Record heal with file_hash
	heal := &HealRecord{
		HealID:        "heal-with-hash",
		ErrorID:       errorID,
		RunID:         runID,
		DiffContent:   "fix content",
		FilePath:      "/app/hash.go",
		FileHash:      "sha256abcdef1234567890",
		Status:        HealStatusPending,
		CreatedAt:     time.Now(),
		AttemptNumber: 1,
	}
	if err := writer.RecordHeal(heal); err != nil {
		t.Fatalf("RecordHeal() failed: %v", err)
	}

	// Verify file_hash was stored correctly
	var storedFileHash string
	err = writer.db.QueryRow("SELECT file_hash FROM heals WHERE heal_id = ?", heal.HealID).Scan(&storedFileHash)
	if err != nil {
		t.Fatalf("Failed to query heal: %v", err)
	}
	if storedFileHash != heal.FileHash {
		t.Errorf("file_hash = %v, want %v", storedFileHash, heal.FileHash)
	}

	// Verify it's returned correctly via GetHealsForError
	heals, err := writer.GetHealsForError(errorID)
	if err != nil {
		t.Fatalf("GetHealsForError() error = %v", err)
	}
	if len(heals) != 1 {
		t.Fatalf("Expected 1 heal, got %d", len(heals))
	}
	if heals[0].FileHash != heal.FileHash {
		t.Errorf("Retrieved heal FileHash = %v, want %v", heals[0].FileHash, heal.FileHash)
	}
}

// --- Heal Lock Tests ---

// TestSQLiteWriter_AcquireHealLock tests basic lock acquisition
func TestSQLiteWriter_AcquireHealLock(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	repoPath := "/test/repo"
	timeout := 10 * time.Minute

	// First acquisition should succeed
	holderID, err := writer.AcquireHealLock(repoPath, timeout)
	if err != nil {
		t.Fatalf("AcquireHealLock() error = %v", err)
	}
	if holderID == "" {
		t.Fatal("Expected non-empty holder ID")
	}

	// Verify lock is held
	held, heldBy, err := writer.IsHealLockHeld(repoPath)
	if err != nil {
		t.Fatalf("IsHealLockHeld() error = %v", err)
	}
	if !held {
		t.Error("Expected lock to be held")
	}
	if heldBy != holderID {
		t.Errorf("Expected holder %s, got %s", holderID, heldBy)
	}

	// Release the lock
	if err := writer.ReleaseHealLock(repoPath, holderID); err != nil {
		t.Fatalf("ReleaseHealLock() error = %v", err)
	}
}

// TestSQLiteWriter_AcquireHealLock_AlreadyHeld tests that second acquisition fails
func TestSQLiteWriter_AcquireHealLock_AlreadyHeld(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	repoPath := "/test/repo"
	timeout := 10 * time.Minute

	// First acquisition should succeed
	holderID1, err := writer.AcquireHealLock(repoPath, timeout)
	if err != nil {
		t.Fatalf("First AcquireHealLock() error = %v", err)
	}
	defer func() { _ = writer.ReleaseHealLock(repoPath, holderID1) }()

	// Second acquisition should fail with ErrHealLockHeld
	_, err = writer.AcquireHealLock(repoPath, timeout)
	if err == nil {
		t.Fatal("Expected error for second acquisition")
	}
	if !errors.Is(err, ErrHealLockHeld) {
		t.Errorf("Expected ErrHealLockHeld, got: %v", err)
	}
}

// TestSQLiteWriter_AcquireHealLock_ExpiredLock tests that expired locks can be re-acquired
func TestSQLiteWriter_AcquireHealLock_ExpiredLock(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	repoPath := "/test/repo"

	// Acquire lock with very short timeout (already expired)
	// We insert directly with an expired timestamp to simulate this
	now := time.Now()
	expiredAt := now.Add(-time.Hour) // Already expired
	_, err = writer.db.Exec(
		"INSERT INTO heal_locks (repo_path, holder_id, acquired_at, expires_at, pid) VALUES (?, ?, ?, ?, ?)",
		repoPath, "expired-holder", now.Add(-2*time.Hour).Unix(), expiredAt.Unix(), 99999,
	)
	if err != nil {
		t.Fatalf("Failed to insert expired lock: %v", err)
	}

	// New acquisition should succeed (expired lock should be cleaned up)
	holderID, err := writer.AcquireHealLock(repoPath, 10*time.Minute)
	if err != nil {
		t.Fatalf("AcquireHealLock() should succeed for expired lock, got error: %v", err)
	}
	if holderID == "" {
		t.Fatal("Expected non-empty holder ID")
	}

	// Clean up
	_ = writer.ReleaseHealLock(repoPath, holderID)
}

// TestSQLiteWriter_ReleaseHealLock tests lock release
func TestSQLiteWriter_ReleaseHealLock(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	repoPath := "/test/repo"
	timeout := 10 * time.Minute

	// Acquire lock
	holderID, err := writer.AcquireHealLock(repoPath, timeout)
	if err != nil {
		t.Fatalf("AcquireHealLock() error = %v", err)
	}

	// Release by correct holder should succeed
	if err := writer.ReleaseHealLock(repoPath, holderID); err != nil {
		t.Fatalf("ReleaseHealLock() error = %v", err)
	}

	// Verify lock is no longer held
	held, _, err := writer.IsHealLockHeld(repoPath)
	if err != nil {
		t.Fatalf("IsHealLockHeld() error = %v", err)
	}
	if held {
		t.Error("Expected lock to be released")
	}

	// Second release should be idempotent (no error)
	if err := writer.ReleaseHealLock(repoPath, holderID); err != nil {
		t.Errorf("Second ReleaseHealLock() should be idempotent, got error: %v", err)
	}
}

// TestSQLiteWriter_ReleaseHealLock_WrongHolder tests that wrong holder cannot release
func TestSQLiteWriter_ReleaseHealLock_WrongHolder(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	repoPath := "/test/repo"
	timeout := 10 * time.Minute

	// Acquire lock
	holderID, err := writer.AcquireHealLock(repoPath, timeout)
	if err != nil {
		t.Fatalf("AcquireHealLock() error = %v", err)
	}
	defer func() { _ = writer.ReleaseHealLock(repoPath, holderID) }()

	// Release by wrong holder should not release the lock
	wrongHolder := "wrong-holder-id"
	if err := writer.ReleaseHealLock(repoPath, wrongHolder); err != nil {
		t.Fatalf("ReleaseHealLock() with wrong holder should not error: %v", err)
	}

	// Lock should still be held
	held, heldBy, err := writer.IsHealLockHeld(repoPath)
	if err != nil {
		t.Fatalf("IsHealLockHeld() error = %v", err)
	}
	if !held {
		t.Error("Lock should still be held after release by wrong holder")
	}
	if heldBy != holderID {
		t.Errorf("Lock holder should be %s, got %s", holderID, heldBy)
	}
}

// TestSQLiteWriter_IsHealLockHeld tests lock status checking
func TestSQLiteWriter_IsHealLockHeld(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	repoPath := "/test/repo"

	// No lock should be held initially
	held, holderID, err := writer.IsHealLockHeld(repoPath)
	if err != nil {
		t.Fatalf("IsHealLockHeld() error = %v", err)
	}
	if held {
		t.Error("Expected no lock to be held initially")
	}
	if holderID != "" {
		t.Errorf("Expected empty holder ID, got %s", holderID)
	}
}

// TestSQLiteWriter_AcquireHealLock_DifferentRepos tests locks on different repos are independent
func TestSQLiteWriter_AcquireHealLock_DifferentRepos(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	repo1 := "/test/repo1"
	repo2 := "/test/repo2"
	timeout := 10 * time.Minute

	// Acquire lock on repo1
	holder1, err := writer.AcquireHealLock(repo1, timeout)
	if err != nil {
		t.Fatalf("AcquireHealLock(repo1) error = %v", err)
	}
	defer func() { _ = writer.ReleaseHealLock(repo1, holder1) }()

	// Acquire lock on repo2 should also succeed
	holder2, err := writer.AcquireHealLock(repo2, timeout)
	if err != nil {
		t.Fatalf("AcquireHealLock(repo2) error = %v", err)
	}
	defer func() { _ = writer.ReleaseHealLock(repo2, holder2) }()

	// Both locks should be held
	held1, _, err := writer.IsHealLockHeld(repo1)
	if err != nil {
		t.Fatalf("IsHealLockHeld(repo1) error = %v", err)
	}
	held2, _, err := writer.IsHealLockHeld(repo2)
	if err != nil {
		t.Fatalf("IsHealLockHeld(repo2) error = %v", err)
	}
	if !held1 || !held2 {
		t.Error("Both repo locks should be held")
	}
}

// ============================================================================
// Assignment CRUD Tests
// ============================================================================

func TestSQLiteWriter_CreateAndGetAssignment(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// First, create a run for the foreign key
	runID := "test-run-123"
	err = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")
	if err != nil {
		t.Fatalf("RecordRun() error = %v", err)
	}

	now := time.Now().Truncate(time.Second)
	expires := now.Add(10 * time.Minute)

	assignment := &Assignment{
		AssignmentID: "assign-001",
		RunID:        runID,
		AgentID:      "agent-local-1",
		WorktreePath: "/tmp/worktree-1",
		ErrorCount:   3,
		ErrorIDs:     []string{"err-1", "err-2", "err-3"},
		Status:       AssignmentStatusAssigned,
		CreatedAt:    now,
		ExpiresAt:    expires,
	}

	// Create assignment
	err = writer.CreateAssignment(assignment)
	if err != nil {
		t.Fatalf("CreateAssignment() error = %v", err)
	}

	// Get assignment back
	got, err := writer.GetAssignment("assign-001")
	if err != nil {
		t.Fatalf("GetAssignment() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetAssignment() returned nil")
	}

	// Verify fields
	if got.AssignmentID != "assign-001" {
		t.Errorf("AssignmentID = %v, want assign-001", got.AssignmentID)
	}
	if got.RunID != runID {
		t.Errorf("RunID = %v, want %v", got.RunID, runID)
	}
	if got.AgentID != "agent-local-1" {
		t.Errorf("AgentID = %v, want agent-local-1", got.AgentID)
	}
	if got.WorktreePath != "/tmp/worktree-1" {
		t.Errorf("WorktreePath = %v, want /tmp/worktree-1", got.WorktreePath)
	}
	if got.ErrorCount != 3 {
		t.Errorf("ErrorCount = %v, want 3", got.ErrorCount)
	}
	if len(got.ErrorIDs) != 3 {
		t.Errorf("ErrorIDs length = %v, want 3", len(got.ErrorIDs))
	}
	if got.Status != AssignmentStatusAssigned {
		t.Errorf("Status = %v, want assigned", got.Status)
	}
}

func TestSQLiteWriter_UpdateAssignmentStatus(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create run and assignment
	runID := "test-run-456"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	assignment := &Assignment{
		AssignmentID: "assign-002",
		RunID:        runID,
		AgentID:      "agent-1",
		ErrorCount:   1,
		ErrorIDs:     []string{"err-1"},
		Status:       AssignmentStatusAssigned,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	_ = writer.CreateAssignment(assignment)

	// Update to in_progress
	err = writer.UpdateAssignmentStatus("assign-002", AssignmentStatusInProgress, "", "")
	if err != nil {
		t.Fatalf("UpdateAssignmentStatus(in_progress) error = %v", err)
	}

	got, _ := writer.GetAssignment("assign-002")
	if got.Status != AssignmentStatusInProgress {
		t.Errorf("Status = %v, want in_progress", got.Status)
	}
	if got.StartedAt == nil {
		t.Error("StartedAt should be set")
	}

	// Update to completed with fix_id
	err = writer.UpdateAssignmentStatus("assign-002", AssignmentStatusCompleted, "fix-abc", "")
	if err != nil {
		t.Fatalf("UpdateAssignmentStatus(completed) error = %v", err)
	}

	got, _ = writer.GetAssignment("assign-002")
	if got.Status != AssignmentStatusCompleted {
		t.Errorf("Status = %v, want completed", got.Status)
	}
	if got.FixID != "fix-abc" {
		t.Errorf("FixID = %v, want fix-abc", got.FixID)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestSQLiteWriter_ListAssignmentsByRun(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run-789"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	// Create multiple assignments
	for i := 1; i <= 3; i++ {
		a := &Assignment{
			AssignmentID: fmt.Sprintf("assign-%d", i),
			RunID:        runID,
			AgentID:      fmt.Sprintf("agent-%d", i),
			ErrorCount:   i,
			ErrorIDs:     []string{fmt.Sprintf("err-%d", i)},
			Status:       AssignmentStatusAssigned,
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Minute),
			ExpiresAt:    time.Now().Add(30 * time.Minute),
		}
		_ = writer.CreateAssignment(a)
	}

	// List assignments
	assignments, err := writer.ListAssignmentsByRun(runID)
	if err != nil {
		t.Fatalf("ListAssignmentsByRun() error = %v", err)
	}

	if len(assignments) != 3 {
		t.Errorf("ListAssignmentsByRun() returned %d assignments, want 3", len(assignments))
	}
}

// ============================================================================
// SuggestedFix CRUD Tests
// ============================================================================

func TestSQLiteWriter_StoreSuggestedFix(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create run and assignment first (foreign keys)
	runID := "test-run-fix"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	assignment := &Assignment{
		AssignmentID: "assign-fix-1",
		RunID:        runID,
		AgentID:      "agent-1",
		ErrorCount:   1,
		ErrorIDs:     []string{"err-1"},
		Status:       AssignmentStatusAssigned,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	_ = writer.CreateAssignment(assignment)

	fix := &SuggestedFix{
		AssignmentID: "assign-fix-1",
		AgentID:      "agent-1",
		WorktreePath: "/tmp/worktree",
		FileChanges: map[string]FileChange{
			"src/main.go": {
				BeforeContent: "func main() {}",
				AfterContent:  "func main() { fmt.Println(\"Hello\") }",
				UnifiedDiff:   "--- a/src/main.go\n+++ b/src/main.go\n@@ -1 +1 @@\n-func main() {}\n+func main() { fmt.Println(\"Hello\") }",
				LinesAdded:    1,
				LinesRemoved:  1,
			},
		},
		Explanation: "Added print statement",
		Confidence:  90,
		Verification: VerificationRecord{
			Command:      "go test ./...",
			ExitCode:     0,
			Output:       "PASS",
			DurationMs:   1500,
			ErrorsBefore: 1,
			ErrorsAfter:  0,
		},
		ModelID:      "claude-sonnet-4-20250514",
		InputTokens:  1000,
		OutputTokens: 200,
		CostUSD:      0.05,
		Status:       FixStatusPending,
		CreatedAt:    time.Now(),
		// Note: ErrorIDs not set - fix_errors junction requires actual errors in DB
	}

	// Store fix
	err = writer.StoreSuggestedFix(fix)
	if err != nil {
		t.Fatalf("StoreSuggestedFix() error = %v", err)
	}

	// Verify fix ID was computed
	if fix.FixID == "" {
		t.Error("FixID should have been computed")
	}

	// Get fix back
	got, err := writer.GetSuggestedFix(fix.FixID)
	if err != nil {
		t.Fatalf("GetSuggestedFix() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetSuggestedFix() returned nil")
	}

	// Verify fields
	if got.AssignmentID != "assign-fix-1" {
		t.Errorf("AssignmentID = %v, want assign-fix-1", got.AssignmentID)
	}
	if got.Confidence != 90 {
		t.Errorf("Confidence = %v, want 90", got.Confidence)
	}
	if got.Verification.Command != "go test ./..." {
		t.Errorf("Verification.Command = %v, want 'go test ./...'", got.Verification.Command)
	}
	if got.Verification.ExitCode != 0 {
		t.Errorf("Verification.ExitCode = %v, want 0", got.Verification.ExitCode)
	}
	if len(got.FileChanges) != 1 {
		t.Errorf("FileChanges length = %v, want 1", len(got.FileChanges))
	}
}

func TestSQLiteWriter_ListPendingFixes(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Setup
	runID := "test-run-pending"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	for i := 1; i <= 3; i++ {
		a := &Assignment{
			AssignmentID: fmt.Sprintf("assign-pend-%d", i),
			RunID:        runID,
			AgentID:      fmt.Sprintf("agent-%d", i),
			ErrorCount:   1,
			ErrorIDs:     []string{fmt.Sprintf("err-%d", i)},
			Status:       AssignmentStatusAssigned,
			CreatedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(10 * time.Minute),
		}
		_ = writer.CreateAssignment(a)

		fix := &SuggestedFix{
			FixID:        fmt.Sprintf("fix-%d", i),
			AssignmentID: fmt.Sprintf("assign-pend-%d", i),
			FileChanges: map[string]FileChange{
				fmt.Sprintf("file%d.go", i): {AfterContent: "content"},
			},
			Verification: VerificationRecord{Command: "test", ExitCode: 0},
			Confidence:   80,
			Status:       FixStatusPending,
			CreatedAt:    time.Now().Add(time.Duration(i) * time.Minute),
		}
		_ = writer.StoreSuggestedFix(fix)
	}

	// List all pending
	fixes, err := writer.ListPendingFixes("")
	if err != nil {
		t.Fatalf("ListPendingFixes() error = %v", err)
	}

	if len(fixes) != 3 {
		t.Errorf("ListPendingFixes() returned %d fixes, want 3", len(fixes))
	}

	// List by run
	fixes, err = writer.ListPendingFixes(runID)
	if err != nil {
		t.Fatalf("ListPendingFixes(runID) error = %v", err)
	}

	if len(fixes) != 3 {
		t.Errorf("ListPendingFixes(runID) returned %d fixes, want 3", len(fixes))
	}
}

func TestSQLiteWriter_UpdateFixStatus(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Setup
	runID := "test-run-status"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	a := &Assignment{
		AssignmentID: "assign-status-1",
		RunID:        runID,
		AgentID:      "agent-1",
		ErrorCount:   1,
		ErrorIDs:     []string{"err-1"},
		Status:       AssignmentStatusAssigned,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	_ = writer.CreateAssignment(a)

	fix := &SuggestedFix{
		FixID:        "fix-status-test",
		AssignmentID: "assign-status-1",
		FileChanges: map[string]FileChange{
			"main.go": {AfterContent: "fixed"},
		},
		Verification: VerificationRecord{Command: "test", ExitCode: 0},
		Confidence:   80,
		Status:       FixStatusPending,
		CreatedAt:    time.Now(),
	}
	if err = writer.StoreSuggestedFix(fix); err != nil {
		t.Fatalf("StoreSuggestedFix() error = %v", err)
	}

	// Update to applied
	err = writer.UpdateFixStatus("fix-status-test", FixStatusApplied, "user@example.com", "abc123sha", "", "")
	if err != nil {
		t.Fatalf("UpdateFixStatus(applied) error = %v", err)
	}

	got, _ := writer.GetSuggestedFix("fix-status-test")
	if got.Status != FixStatusApplied {
		t.Errorf("Status = %v, want applied", got.Status)
	}
	if got.AppliedBy != "user@example.com" {
		t.Errorf("AppliedBy = %v, want user@example.com", got.AppliedBy)
	}
	if got.AppliedCommitSHA != "abc123sha" {
		t.Errorf("AppliedCommitSHA = %v, want abc123sha", got.AppliedCommitSHA)
	}
	if got.AppliedAt == nil {
		t.Error("AppliedAt should be set")
	}
}

func TestComputeFixID_Deterministic(t *testing.T) {
	changes1 := map[string]FileChange{
		"file1.go": {AfterContent: "content1"},
		"file2.go": {AfterContent: "content2"},
	}
	changes2 := map[string]FileChange{
		"file2.go": {AfterContent: "content2"},
		"file1.go": {AfterContent: "content1"},
	}

	id1 := ComputeFixID(changes1)
	id2 := ComputeFixID(changes2)

	if id1 != id2 {
		t.Errorf("ComputeFixID not deterministic: %s != %s", id1, id2)
	}

	// Different content should produce different ID
	changes3 := map[string]FileChange{
		"file1.go": {AfterContent: "different"},
		"file2.go": {AfterContent: "content2"},
	}
	id3 := ComputeFixID(changes3)

	if id1 == id3 {
		t.Error("Different changes should produce different IDs")
	}
}

func TestComputeFixID_EmptyAndNilMaps(t *testing.T) {
	// Empty map should return empty string (not a hash of empty content)
	emptyID := ComputeFixID(map[string]FileChange{})
	if emptyID != "" {
		t.Errorf("ComputeFixID(empty map) = %q, want empty string", emptyID)
	}

	// Nil map should return empty string
	nilID := ComputeFixID(nil)
	if nilID != "" {
		t.Errorf("ComputeFixID(nil) = %q, want empty string", nilID)
	}
}

func TestSQLiteWriter_GetAssignment_NotFound(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Get non-existent assignment should return nil, nil (not an error)
	got, err := writer.GetAssignment("does-not-exist")
	if err != nil {
		t.Errorf("GetAssignment(non-existent) should not error, got: %v", err)
	}
	if got != nil {
		t.Error("GetAssignment(non-existent) should return nil")
	}
}

func TestSQLiteWriter_GetSuggestedFix_NotFound(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Get non-existent fix should return nil, nil (not an error)
	got, err := writer.GetSuggestedFix("does-not-exist")
	if err != nil {
		t.Errorf("GetSuggestedFix(non-existent) should not error, got: %v", err)
	}
	if got != nil {
		t.Error("GetSuggestedFix(non-existent) should return nil")
	}
}

func TestSQLiteWriter_UpdateAssignmentStatus_NotFound(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Update non-existent assignment should return an error
	err = writer.UpdateAssignmentStatus("does-not-exist", AssignmentStatusCompleted, "", "")
	if err == nil {
		t.Error("UpdateAssignmentStatus(non-existent) should return an error")
	}
}

func TestSQLiteWriter_UpdateFixStatus_NotFound(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Update non-existent fix should return an error
	err = writer.UpdateFixStatus("does-not-exist", FixStatusApplied, "user", "sha", "", "")
	if err == nil {
		t.Error("UpdateFixStatus(non-existent) should return an error")
	}
}

func TestSQLiteWriter_UpdateFixStatus_Rejected(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Setup
	runID := "test-run-rejected"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	a := &Assignment{
		AssignmentID: "assign-rejected-1",
		RunID:        runID,
		AgentID:      "agent-1",
		ErrorCount:   1,
		ErrorIDs:     []string{"err-1"},
		Status:       AssignmentStatusAssigned,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	_ = writer.CreateAssignment(a)

	fix := &SuggestedFix{
		FixID:        "fix-rejected-test",
		AssignmentID: "assign-rejected-1",
		FileChanges: map[string]FileChange{
			"main.go": {AfterContent: "fixed"},
		},
		Verification: VerificationRecord{Command: "test", ExitCode: 0},
		Confidence:   80,
		Status:       FixStatusPending,
		CreatedAt:    time.Now(),
	}
	if err = writer.StoreSuggestedFix(fix); err != nil {
		t.Fatalf("StoreSuggestedFix() error = %v", err)
	}

	// Update to rejected
	err = writer.UpdateFixStatus("fix-rejected-test", FixStatusRejected, "", "", "reviewer@example.com", "Introduces regression")
	if err != nil {
		t.Fatalf("UpdateFixStatus(rejected) error = %v", err)
	}

	got, _ := writer.GetSuggestedFix("fix-rejected-test")
	if got.Status != FixStatusRejected {
		t.Errorf("Status = %v, want rejected", got.Status)
	}
	if got.RejectedBy != "reviewer@example.com" {
		t.Errorf("RejectedBy = %v, want reviewer@example.com", got.RejectedBy)
	}
	if got.RejectionReason != "Introduces regression" {
		t.Errorf("RejectionReason = %v, want 'Introduces regression'", got.RejectionReason)
	}
	if got.RejectedAt == nil {
		t.Error("RejectedAt should be set")
	}
}

func TestSQLiteWriter_AssignmentFailed(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create run and assignment
	runID := "test-run-failed"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	assignment := &Assignment{
		AssignmentID: "assign-fail-1",
		RunID:        runID,
		AgentID:      "agent-1",
		ErrorCount:   1,
		ErrorIDs:     []string{"err-1"},
		Status:       AssignmentStatusAssigned,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	_ = writer.CreateAssignment(assignment)

	// Update to failed with failure reason
	err = writer.UpdateAssignmentStatus("assign-fail-1", AssignmentStatusFailed, "", "Agent timeout")
	if err != nil {
		t.Fatalf("UpdateAssignmentStatus(failed) error = %v", err)
	}

	got, _ := writer.GetAssignment("assign-fail-1")
	if got.Status != AssignmentStatusFailed {
		t.Errorf("Status = %v, want failed", got.Status)
	}
	if got.FailureReason != "Agent timeout" {
		t.Errorf("FailureReason = %v, want 'Agent timeout'", got.FailureReason)
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should be set for failed status")
	}
}

func TestSQLiteWriter_StoreSuggestedFix_WithErrorIDs(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create run
	runID := "test-run-errors"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	// Create actual error records first (required for FK constraint on fix_errors)
	finding1 := &FindingRecord{
		RunID:    runID,
		FilePath: "main.go",
		Message:  "Error 1 for fix test",
		Line:     10,
		Column:   5,
		Severity: "error",
		Category: "type-check",
		Source:   "go",
	}
	finding2 := &FindingRecord{
		RunID:    runID,
		FilePath: "main.go",
		Message:  "Error 2 for fix test",
		Line:     20,
		Column:   10,
		Severity: "error",
		Category: "type-check",
		Source:   "go",
	}
	_ = writer.RecordError(finding1)
	_ = writer.RecordError(finding2)
	_ = writer.Flush()

	// Get the actual error IDs from the database
	errors, err := writer.GetErrorsByRunID(runID)
	if err != nil {
		t.Fatalf("GetErrorsByRunID() error = %v", err)
	}
	if len(errors) < 2 {
		t.Fatalf("Expected at least 2 errors, got %d", len(errors))
	}

	errorIDs := []string{errors[0].ErrorID, errors[1].ErrorID}

	// Create assignment with actual error IDs
	a := &Assignment{
		AssignmentID: "assign-errors-1",
		RunID:        runID,
		AgentID:      "agent-1",
		ErrorCount:   2,
		ErrorIDs:     errorIDs,
		Status:       AssignmentStatusAssigned,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	_ = writer.CreateAssignment(a)

	fix := &SuggestedFix{
		FixID:        "fix-with-errors",
		AssignmentID: "assign-errors-1",
		FileChanges: map[string]FileChange{
			"main.go": {AfterContent: "fixed"},
		},
		Verification: VerificationRecord{Command: "test", ExitCode: 0},
		Confidence:   80,
		Status:       FixStatusPending,
		CreatedAt:    time.Now(),
		ErrorIDs:     errorIDs,
	}
	err = writer.StoreSuggestedFix(fix)
	if err != nil {
		t.Fatalf("StoreSuggestedFix() error = %v", err)
	}

	// Retrieve and verify error IDs are persisted
	got, err := writer.GetSuggestedFix("fix-with-errors")
	if err != nil {
		t.Fatalf("GetSuggestedFix() error = %v", err)
	}
	if len(got.ErrorIDs) != 2 {
		t.Errorf("ErrorIDs length = %d, want 2", len(got.ErrorIDs))
	}
}

func TestSQLiteWriter_StoreSuggestedFix_EmptyFileChanges(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// Create run and assignment
	runID := "test-run-empty-changes"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	a := &Assignment{
		AssignmentID: "assign-empty-changes",
		RunID:        runID,
		AgentID:      "agent-1",
		ErrorCount:   1,
		ErrorIDs:     []string{},
		Status:       AssignmentStatusAssigned,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	_ = writer.CreateAssignment(a)

	// Try to store fix with empty file changes - should fail
	fix := &SuggestedFix{
		AssignmentID: "assign-empty-changes",
		FileChanges:  map[string]FileChange{},
		Verification: VerificationRecord{Command: "test", ExitCode: 0},
		Confidence:   80,
		Status:       FixStatusPending,
		CreatedAt:    time.Now(),
	}
	err = writer.StoreSuggestedFix(fix)
	if err == nil {
		t.Error("StoreSuggestedFix() should fail with empty file changes")
	}

	// Try with nil file changes too
	fix2 := &SuggestedFix{
		AssignmentID: "assign-empty-changes",
		FileChanges:  nil,
		Verification: VerificationRecord{Command: "test", ExitCode: 0},
		Confidence:   80,
		Status:       FixStatusPending,
		CreatedAt:    time.Now(),
	}
	err = writer.StoreSuggestedFix(fix2)
	if err == nil {
		t.Error("StoreSuggestedFix() should fail with nil file changes")
	}
}

func TestSQLiteWriter_ListAssignmentsByRun_Empty(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run-empty"
	_ = writer.RecordRun(runID, "test-workflow", "abc123", "tree123", "local")

	// List assignments for run with no assignments
	assignments, err := writer.ListAssignmentsByRun(runID)
	if err != nil {
		t.Fatalf("ListAssignmentsByRun() error = %v", err)
	}
	if assignments == nil {
		t.Error("ListAssignmentsByRun() should return empty slice, not nil")
	}
	if len(assignments) != 0 {
		t.Errorf("ListAssignmentsByRun() returned %d, want 0", len(assignments))
	}
}

func TestSQLiteWriter_ListPendingFixes_Empty(t *testing.T) {
	setupTestDetentHome(t)
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	// List pending fixes when none exist
	fixes, err := writer.ListPendingFixes("")
	if err != nil {
		t.Fatalf("ListPendingFixes() error = %v", err)
	}
	if fixes == nil {
		t.Error("ListPendingFixes() should return empty slice, not nil")
	}
	if len(fixes) != 0 {
		t.Errorf("ListPendingFixes() returned %d, want 0", len(fixes))
	}
}
