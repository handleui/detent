package persistence

import (
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

