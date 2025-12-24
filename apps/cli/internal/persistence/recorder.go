package persistence

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/detent/cli/internal/errors"
)

const (
	// UUID v4 bit manipulation constants
	uuidVersionMask = 0x0f
	uuidVersion4    = 0x40
	uuidVariantMask = 0x3f
	uuidVariantRFC  = 0x80

	// UUID byte positions for version and variant
	uuidVersionByteIndex = 6
	uuidVariantByteIndex = 8

	// UUID byte slice sizes for formatting
	uuidBytesTotal  = 16
	uuidSlice1End   = 4
	uuidSlice2Start = 4
	uuidSlice2End   = 6
	uuidSlice3Start = 6
	uuidSlice3End   = 8
	uuidSlice4Start = 8
	uuidSlice4End   = 10
	uuidSlice5Start = 10
)

// Recorder coordinates persistence of scan results
type Recorder struct {
	runID     string
	repoRoot  string
	startTime time.Time

	// SQLite writer for persistent storage
	sqlite *SQLiteWriter

	// In-memory tracking for final summary
	errors        []*errors.ExtractedError
	fileMetadata  map[string]*ScannedFile // keyed by file path
	errorCounts   map[string]int          // error count per file
	warningCounts map[string]int          // warning count per file

	// Run metadata
	workflowName string
	commitSHA    string
	execMode     string
}

// generateUUID creates a simple UUID v4 without external dependencies
func generateUUID() (string, error) {
	b := make([]byte, uuidBytesTotal)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes for UUID: %w", err)
	}
	// Set version (4) and variant bits
	b[uuidVersionByteIndex] = (b[uuidVersionByteIndex] & uuidVersionMask) | uuidVersion4
	b[uuidVariantByteIndex] = (b[uuidVariantByteIndex] & uuidVariantMask) | uuidVariantRFC
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:uuidSlice1End],
		b[uuidSlice2Start:uuidSlice2End],
		b[uuidSlice3Start:uuidSlice3End],
		b[uuidSlice4Start:uuidSlice4End],
		b[uuidSlice5Start:]), nil
}

// NewRecorder creates a new persistence recorder
func NewRecorder(repoRoot, workflowName, commitSHA, execMode string, isDirty bool, dirtyFiles []string) (*Recorder, error) {
	runID, err := generateUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate run ID: %w", err)
	}

	sqlite, err := NewSQLiteWriter(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite writer: %w", err)
	}

	// Record the run in the database
	if err := sqlite.RecordRun(runID, workflowName, commitSHA, execMode, isDirty, dirtyFiles); err != nil {
		if closeErr := sqlite.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to record run: %w (additionally, failed to close database: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to record run: %w", err)
	}

	return &Recorder{
		runID:         runID,
		repoRoot:      repoRoot,
		startTime:     time.Now(),
		sqlite:        sqlite,
		errors:        make([]*errors.ExtractedError, 0),
		fileMetadata:  make(map[string]*ScannedFile),
		errorCounts:   make(map[string]int),
		warningCounts: make(map[string]int),
		workflowName:  workflowName,
		commitSHA:     commitSHA,
		execMode:      execMode,
	}, nil
}

// RecordFinding logs a single finding to JSONL and tracks it in memory
func (r *Recorder) RecordFinding(err *errors.ExtractedError) error {
	// Build finding record for SQLite database
	finding := &FindingRecord{
		Timestamp:  time.Now(),
		RunID:      r.runID,
		FilePath:   err.File,
		Message:    err.Message,
		Line:       err.Line,
		Column:     err.Column,
		Severity:   err.Severity,
		StackTrace: err.StackTrace,
		RuleID:     err.RuleID,
		Category:   string(err.Category),
		Source:     err.Source,
		Raw:        err.Raw,
	}

	// Add workflow context if available
	if err.WorkflowContext != nil {
		finding.WorkflowJob = err.WorkflowContext.Job
		finding.WorkflowStep = err.WorkflowContext.Step
	}

	// Write to SQLite database first (fail fast if DB write fails)
	if dbErr := r.sqlite.RecordError(finding); dbErr != nil {
		return fmt.Errorf("failed to record finding: %w", dbErr)
	}

	// Only update in-memory tracking after successful DB write
	// Track in memory for final summary
	r.errors = append(r.errors, err)

	// Track file-level counts
	if err.File != "" {
		if err.Severity == "error" {
			r.errorCounts[err.File]++
		} else {
			r.warningCounts[err.File]++
		}
	}

	return nil
}

// Finalize updates the run completion status and closes the SQLite connection
func (r *Recorder) Finalize(exitCode int) error {
	// Get total error count from SQLite writer
	totalErrors := r.sqlite.GetErrorCount()

	// Finalize the run in the database
	if err := r.sqlite.FinalizeRun(r.runID, totalErrors); err != nil {
		return fmt.Errorf("failed to finalize run: %w", err)
	}

	// Close SQLite writer
	if err := r.sqlite.Close(); err != nil {
		return fmt.Errorf("failed to close SQLite writer: %w", err)
	}

	return nil
}

// GetOutputPath returns the path to the SQLite database file
func (r *Recorder) GetOutputPath() string {
	return r.sqlite.Path()
}
