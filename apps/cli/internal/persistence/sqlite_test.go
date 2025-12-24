package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewSQLiteWriter tests the creation of a new SQLite writer
func TestNewSQLiteWriter(t *testing.T) {
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)

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

	// Verify .detent directory was created
	detentPath := filepath.Join(tmpDir, detentDir)
	if _, err := os.Stat(detentPath); os.IsNotExist(err) {
		t.Error(".detent directory was not created")
	}

	// Verify database file was created
	dbPath := filepath.Join(detentPath, detentDBName)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}

	// Verify path is correct
	if writer.Path() != dbPath {
		t.Errorf("Path() = %v, want %v", writer.Path(), dbPath)
	}
}

// TestSQLiteWriter_initSchema tests schema initialization
func TestSQLiteWriter_initSchema(t *testing.T) {
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

	// Verify key indices exist
	indices := []string{
		"idx_errors_run_id",
		"idx_errors_content_hash",
		"idx_errors_file_path",
		"idx_errors_status",
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
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "run-123"
	err = writer.RecordRun(runID, "CI", "abc123", "github")
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

// TestSQLiteWriter_RecordError_WithFlush tests error recording with manual flush
func TestSQLiteWriter_RecordError_WithFlush(t *testing.T) {
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "github"); err != nil {
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
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "github"); err != nil {
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
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "run-with-errors"
	if err := writer.RecordRun(runID, "test", "abc123", "github"); err != nil {
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
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "github"); err != nil {
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
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "test-run"
	if err := writer.RecordRun(runID, "test", "abc123", "github"); err != nil {
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
	tmpDir := t.TempDir()
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer func() { _ = writer.Close() }()

	runID := "timestamp-test"
	if err := writer.RecordRun(runID, "test", "abc123", "github"); err != nil {
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

	// Get first timestamps
	contentHash := ComputeContentHash(finding.Message)
	var firstSeenAt1, lastSeenAt1 int64
	query := "SELECT first_seen_at, last_seen_at FROM errors WHERE content_hash = ?"
	err = writer.db.QueryRow(query, contentHash).Scan(&firstSeenAt1, &lastSeenAt1)
	if err != nil {
		t.Fatalf("Failed to query timestamps: %v", err)
	}

	// Wait to ensure timestamp difference
	time.Sleep(1100 * time.Millisecond)

	// Record again
	if err := writer.RecordError(finding); err != nil {
		t.Fatalf("RecordError() failed on second attempt: %v", err)
	}
	if err := writer.FlushBatch(); err != nil {
		t.Fatalf("FlushBatch() failed: %v", err)
	}

	// Get updated timestamps
	var firstSeenAt2, lastSeenAt2 int64
	err = writer.db.QueryRow(query, contentHash).Scan(&firstSeenAt2, &lastSeenAt2)
	if err != nil {
		t.Fatalf("Failed to query timestamps: %v", err)
	}

	// first_seen_at should not change
	if firstSeenAt1 != firstSeenAt2 {
		t.Errorf("first_seen_at changed from %d to %d", firstSeenAt1, firstSeenAt2)
	}

	// last_seen_at should be updated (timestamps are in seconds, so need > 1s gap)
	if lastSeenAt2 <= lastSeenAt1 {
		t.Errorf("last_seen_at not updated: %d <= %d", lastSeenAt2, lastSeenAt1)
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
