package persistence

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/util"
)

// Recorder persists scan results to SQLite
type Recorder struct {
	runID     string
	repoRoot  string
	startTime time.Time
	sqlite    *SQLiteWriter

	errors        []*errors.ExtractedError
	fileMetadata  map[string]*ScannedFile
	errorCounts   map[string]int
	warningCounts map[string]int
	fileHashCache map[string]string

	workflowName string
	commitSHA    string
	execMode     string
}

// NewRecorder creates a recorder for the given repository
func NewRecorder(repoRoot, workflowName, commitSHA, execMode string) (*Recorder, error) {
	runID, err := util.GenerateUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate run ID: %w", err)
	}

	sqlite, err := NewSQLiteWriter(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite writer: %w", err)
	}

	if err := sqlite.RecordRun(runID, workflowName, commitSHA, execMode); err != nil {
		_ = sqlite.Close()
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
		fileHashCache: make(map[string]string),
		workflowName:  workflowName,
		commitSHA:     commitSHA,
		execMode:      execMode,
	}, nil
}

// RecordFindings records multiple findings in a single transaction
func (r *Recorder) RecordFindings(findings []*errors.ExtractedError) error {
	if len(findings) == 0 {
		return nil
	}

	findingRecords := make([]*FindingRecord, 0, len(findings))
	for _, err := range findings {
		findingRecords = append(findingRecords, r.buildFindingRecord(err))
	}

	if err := r.sqlite.RecordFindings(findingRecords); err != nil {
		return fmt.Errorf("failed to record findings: %w", err)
	}

	for _, err := range findings {
		r.trackFinding(err)
	}

	return nil
}

// RecordFinding records a single finding to the database
func (r *Recorder) RecordFinding(err *errors.ExtractedError) error {
	finding := r.buildFindingRecord(err)

	if dbErr := r.sqlite.RecordError(finding); dbErr != nil {
		return fmt.Errorf("failed to record finding: %w", dbErr)
	}

	r.trackFinding(err)
	return nil
}

func (r *Recorder) buildFindingRecord(err *errors.ExtractedError) *FindingRecord {
	finding := &FindingRecord{
		Timestamp:  time.Now(),
		RunID:      r.runID,
		FilePath:   err.File,
		FileHash:   r.computeFileHash(err.File),
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

	if err.WorkflowContext != nil {
		finding.WorkflowJob = err.WorkflowContext.Job
		finding.WorkflowStep = err.WorkflowContext.Step
	}

	return finding
}

func (r *Recorder) trackFinding(err *errors.ExtractedError) {
	r.errors = append(r.errors, err)

	if err.File != "" {
		if err.Severity == "error" {
			r.errorCounts[err.File]++
		} else {
			r.warningCounts[err.File]++
		}
	}
}

// Finalize completes the run and closes the database connection
func (r *Recorder) Finalize(exitCode int) error {
	totalErrors := r.sqlite.GetErrorCount()

	if err := r.sqlite.FinalizeRun(r.runID, totalErrors); err != nil {
		return fmt.Errorf("failed to finalize run: %w", err)
	}

	if err := r.sqlite.Close(); err != nil {
		return fmt.Errorf("failed to close SQLite writer: %w", err)
	}

	return nil
}

// GetOutputPath returns the database file path
func (r *Recorder) GetOutputPath() string {
	return r.sqlite.Path()
}

// computeFileHash returns cached or computed hash. Empty string if file missing or outside repo.
func (r *Recorder) computeFileHash(filePath string) string {
	if filePath == "" {
		return ""
	}

	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(r.repoRoot, filePath)
	}
	absPath = filepath.Clean(absPath)

	// Prevent path traversal
	repoRootClean := filepath.Clean(r.repoRoot)
	if !strings.HasPrefix(absPath, repoRootClean+string(filepath.Separator)) && absPath != repoRootClean {
		return ""
	}

	if hash, ok := r.fileHashCache[absPath]; ok {
		return hash
	}

	hash, err := ComputeFileHash(absPath)
	if err != nil {
		return ""
	}

	r.fileHashCache[absPath] = hash
	return hash
}
