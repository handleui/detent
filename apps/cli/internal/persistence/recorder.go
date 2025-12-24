package persistence

import (
	"fmt"
	"time"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/util"
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

// NewRecorder creates a new persistence recorder
func NewRecorder(repoRoot, workflowName, commitSHA, execMode string, isDirty bool, dirtyFiles []string) (*Recorder, error) {
	runID, err := util.GenerateUUID()
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

// RecordFindings records multiple findings in a single transaction for better performance
func (r *Recorder) RecordFindings(findings []*errors.ExtractedError) error {
	if len(findings) == 0 {
		return nil
	}

	// Build finding records for all findings
	findingRecords := make([]*FindingRecord, 0, len(findings))
	for _, err := range findings {
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

		findingRecords = append(findingRecords, finding)
	}

	// Write all findings in a single transaction
	if err := r.sqlite.RecordFindings(findingRecords); err != nil {
		return fmt.Errorf("failed to record findings: %w", err)
	}

	// Update in-memory tracking after successful batch write
	for _, err := range findings {
		r.errors = append(r.errors, err)

		// Track file-level counts
		if err.File != "" {
			if err.Severity == "error" {
				r.errorCounts[err.File]++
			} else {
				r.warningCounts[err.File]++
			}
		}
	}

	return nil
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
