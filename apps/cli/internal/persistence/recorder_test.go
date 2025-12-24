package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/detent/cli/internal/errors"
)

// TestNewRecorder tests recorder creation
func TestNewRecorder(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "CI", "abc123", "github", false, nil)

	if err != nil {
		t.Fatalf("NewRecorder() error = %v", err)
	}

	if recorder == nil {
		t.Fatal("Expected non-nil recorder")
	}

	defer func() {
		if closeErr := recorder.Finalize(0); closeErr != nil {
			t.Errorf("Failed to finalize recorder: %v", closeErr)
		}
	}()

	// Verify recorder fields
	if recorder.runID == "" {
		t.Error("runID should not be empty")
	}
	if recorder.repoRoot != tmpDir {
		t.Errorf("repoRoot = %v, want %v", recorder.repoRoot, tmpDir)
	}
	if recorder.workflowName != "CI" {
		t.Errorf("workflowName = %v, want 'CI'", recorder.workflowName)
	}
	if recorder.commitSHA != "abc123" {
		t.Errorf("commitSHA = %v, want 'abc123'", recorder.commitSHA)
	}
	if recorder.execMode != "github" {
		t.Errorf("execMode = %v, want 'github'", recorder.execMode)
	}
	if recorder.sqlite == nil {
		t.Error("SQLite writer should not be nil")
	}

	// Verify run was recorded in database
	dbPath := filepath.Join(tmpDir, detentDir, detentDBName)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

// TestRecorder_RecordFinding tests finding recording
func TestRecorder_RecordFinding(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "test", "abc123", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() { _ = recorder.Finalize(0) }()

	err = recorder.RecordFinding(&errors.ExtractedError{
		Message:  "test error",
		File:     "/app/test.go",
		Line:     42,
		Severity: "error",
		Category: errors.CategoryCompile,
	})
	if err != nil {
		t.Fatalf("RecordFinding() error = %v", err)
	}

	// Verify in-memory tracking
	if len(recorder.errors) != 1 {
		t.Errorf("In-memory errors count = %d, want 1", len(recorder.errors))
	}

	// Verify file-level counts
	if recorder.errorCounts["/app/test.go"] != 1 {
		t.Errorf("errorCounts = %d, want 1", recorder.errorCounts["/app/test.go"])
	}
}

// TestRecorder_RecordFinding_FileCounts tests file-level error/warning counting
func TestRecorder_RecordFinding_FileCounts(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "test", "abc123", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() { _ = recorder.Finalize(0) }()

	file1 := "/app/file1.go"

	// Record 3 errors
	for i := 0; i < 3; i++ {
		err := recorder.RecordFinding(&errors.ExtractedError{
			Message:  "error in file1",
			File:     file1,
			Line:     i,
			Severity: "error",
			Category: errors.CategoryCompile,
		})
		if err != nil {
			t.Fatalf("Failed to record error: %v", err)
		}
	}

	// Record 2 warnings
	for i := 0; i < 2; i++ {
		err := recorder.RecordFinding(&errors.ExtractedError{
			Message:  "warning in file1",
			File:     file1,
			Line:     i + 10,
			Severity: "warning",
			Category: errors.CategoryLint,
		})
		if err != nil {
			t.Fatalf("Failed to record warning: %v", err)
		}
	}

	// Verify counts
	if recorder.errorCounts[file1] != 3 {
		t.Errorf("errorCounts[%s] = %d, want 3", file1, recorder.errorCounts[file1])
	}
	if recorder.warningCounts[file1] != 2 {
		t.Errorf("warningCounts[%s] = %d, want 2", file1, recorder.warningCounts[file1])
	}
}

// TestRecorder_Finalize tests recorder finalization
func TestRecorder_Finalize(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "test", "abc123", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}

	// Add some errors
	for i := 0; i < 5; i++ {
		err := recorder.RecordFinding(&errors.ExtractedError{
			Message:  "test error",
			File:     "/app/test.go",
			Line:     i,
			Severity: "error",
			Category: errors.CategoryTest,
		})
		if err != nil {
			t.Fatalf("RecordFinding() failed: %v", err)
		}
	}

	// Finalize
	if err := recorder.Finalize(0); err != nil {
		t.Fatalf("Finalize() failed: %v", err)
	}

	// Verify the run was finalized in the database
	// Reopen the database to check
	writer, err := NewSQLiteWriter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = writer.Close() }()

	var completedAt int64
	query := "SELECT completed_at FROM runs WHERE run_id = ?"
	err = writer.db.QueryRow(query, recorder.runID).Scan(&completedAt)
	if err != nil {
		t.Fatalf("Failed to query run: %v", err)
	}

	if completedAt == 0 {
		t.Error("completed_at should not be zero after finalization")
	}
}

// TestRecorder_GetOutputPath tests output path retrieval
func TestRecorder_GetOutputPath(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "test", "abc123", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() { _ = recorder.Finalize(0) }()

	outputPath := recorder.GetOutputPath()
	expectedPath := filepath.Join(tmpDir, detentDir, detentDBName)

	if outputPath != expectedPath {
		t.Errorf("GetOutputPath() = %v, want %v", outputPath, expectedPath)
	}

	// Verify the file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Output path does not exist")
	}
}

// TestGenerateUUID tests UUID generation
func TestGenerateUUID(t *testing.T) {
	uuids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		uuid, err := generateUUID()
		if err != nil {
			t.Fatalf("generateUUID() failed: %v", err)
		}

		// Verify format (8-4-4-4-12)
		if len(uuid) != 36 {
			t.Errorf("UUID length = %d, want 36", len(uuid))
		}

		// Check for dashes in correct positions
		if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
			t.Errorf("UUID format incorrect: %s", uuid)
		}

		// Check for uniqueness
		if uuids[uuid] {
			t.Errorf("Duplicate UUID generated: %s", uuid)
		}
		uuids[uuid] = true
	}

	if len(uuids) != 100 {
		t.Errorf("Generated %d unique UUIDs, want 100", len(uuids))
	}
}

// TestRecorder_WorkflowContextPersistence tests that workflow context is tracked
func TestRecorder_WorkflowContextPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "test-workflow", "abc123def", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() { _ = recorder.Finalize(0) }()

	// Record error with workflow context
	err = recorder.RecordFinding(&errors.ExtractedError{
		Message:  "workflow error",
		File:     "/app/test.go",
		Line:     42,
		Severity: "error",
		Category: errors.CategoryTest,
		WorkflowContext: &errors.WorkflowContext{
			Job:  "test-job",
			Step: "test-step",
		},
	})
	if err != nil {
		t.Fatalf("RecordFinding() failed: %v", err)
	}

	// Verify it was recorded in memory
	if len(recorder.errors) != 1 {
		t.Fatalf("Expected 1 error in memory, got %d", len(recorder.errors))
	}

	if recorder.errors[0].WorkflowContext == nil {
		t.Error("WorkflowContext should not be nil")
	} else {
		if recorder.errors[0].WorkflowContext.Job != "test-job" {
			t.Errorf("Job = %v, want 'test-job'", recorder.errors[0].WorkflowContext.Job)
		}
		if recorder.errors[0].WorkflowContext.Step != "test-step" {
			t.Errorf("Step = %v, want 'test-step'", recorder.errors[0].WorkflowContext.Step)
		}
	}
}

// TestRecorder_StartTime tests that start time is set
func TestRecorder_StartTime(t *testing.T) {
	tmpDir := t.TempDir()
	before := time.Now()

	recorder, err := NewRecorder(tmpDir, "test", "abc123", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() { _ = recorder.Finalize(0) }()

	after := time.Now()

	// Verify start time is within reasonable bounds
	if recorder.startTime.Before(before) || recorder.startTime.After(after) {
		t.Errorf("startTime = %v, should be between %v and %v", recorder.startTime, before, after)
	}
}

// TestRecorder_ErrorCategoryTracking tests that different error categories are tracked
func TestRecorder_ErrorCategoryTracking(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "test", "abc123", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() { _ = recorder.Finalize(0) }()

	categories := []errors.ErrorCategory{
		errors.CategoryLint,
		errors.CategoryTypeCheck,
		errors.CategoryCompile,
		errors.CategoryTest,
		errors.CategoryRuntime,
	}

	for i, cat := range categories {
		err := recorder.RecordFinding(&errors.ExtractedError{
			Message:  "test error for category",
			File:     "/app/test.go",
			Line:     i + 1,
			Severity: "error",
			Category: cat,
		})
		if err != nil {
			t.Fatalf("Failed to record error for category %v: %v", cat, err)
		}
	}

	// Verify all categories were recorded
	if len(recorder.errors) != len(categories) {
		t.Errorf("Expected %d errors, got %d", len(categories), len(recorder.errors))
	}

	// Verify each category is present
	categoryMap := make(map[errors.ErrorCategory]bool)
	for _, e := range recorder.errors {
		categoryMap[e.Category] = true
	}

	for _, cat := range categories {
		if !categoryMap[cat] {
			t.Errorf("Category %v not found in recorded errors", cat)
		}
	}
}

// TestRecorder_RecordFinding_WithoutFile tests recording errors without file information
func TestRecorder_RecordFinding_WithoutFile(t *testing.T) {
	tmpDir := t.TempDir()
	recorder, err := NewRecorder(tmpDir, "test", "abc123", "github", false, nil)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() { _ = recorder.Finalize(0) }()

	err = recorder.RecordFinding(&errors.ExtractedError{
		Message:  "generic error",
		Severity: "error",
		Category: errors.CategoryRuntime,
	})
	if err != nil {
		t.Fatalf("RecordFinding() error = %v", err)
	}

	if len(recorder.errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(recorder.errors))
	}

	// Verify no file counts were incremented
	if len(recorder.errorCounts) != 0 {
		t.Errorf("errorCounts should be empty, got %d entries", len(recorder.errorCounts))
	}
}
