package persistence

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
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

	// JSONL writer for streaming results
	jsonl *JSONLWriter

	// In-memory tracking for final summary
	errors        []*errors.ExtractedError
	fileMetadata  map[string]*ScannedFile // keyed by file path
	errorCounts   map[string]int          // error count per file
	warningCounts map[string]int          // warning count per file

	// Run metadata
	workflowsPath string
	event         string
}

// generateUUID creates a simple UUID v4 without external dependencies
func generateUUID() string {
	b := make([]byte, uuidBytesTotal)
	_, _ = rand.Read(b)
	// Set version (4) and variant bits
	b[uuidVersionByteIndex] = (b[uuidVersionByteIndex] & uuidVersionMask) | uuidVersion4
	b[uuidVariantByteIndex] = (b[uuidVariantByteIndex] & uuidVariantMask) | uuidVariantRFC
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:uuidSlice1End],
		b[uuidSlice2Start:uuidSlice2End],
		b[uuidSlice3Start:uuidSlice3End],
		b[uuidSlice4Start:uuidSlice4End],
		b[uuidSlice5Start:])
}

// NewRecorder creates a new persistence recorder
func NewRecorder(repoRoot, workflowsPath, event string) (*Recorder, error) {
	runID := generateUUID()

	jsonl, err := NewJSONLWriter(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to create JSONL writer: %w", err)
	}

	return &Recorder{
		runID:         runID,
		repoRoot:      repoRoot,
		startTime:     time.Now(),
		jsonl:         jsonl,
		errors:        make([]*errors.ExtractedError, 0),
		fileMetadata:  make(map[string]*ScannedFile),
		errorCounts:   make(map[string]int),
		warningCounts: make(map[string]int),
		workflowsPath: workflowsPath,
		event:         event,
	}, nil
}

// RecordFinding logs a single finding to JSONL and tracks it in memory
func (r *Recorder) RecordFinding(err *errors.ExtractedError) error {
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

	// Build finding record for JSONL
	finding := &FindingRecord{
		Timestamp: time.Now(),
		RunID:     r.runID,
		FilePath:  err.File,
		Message:   err.Message,
		Line:      err.Line,
		Column:    err.Column,
		Severity:  err.Severity,
		RuleID:    err.RuleID,
		Category:  string(err.Category),
		Source:    err.Source,
		Raw:       err.Raw,
	}

	// Add workflow context if available
	if err.WorkflowContext != nil {
		finding.WorkflowJob = err.WorkflowContext.Job
		finding.WorkflowStep = err.WorkflowContext.Step
	}

	// Write to JSONL file
	if err := r.jsonl.WriteFinding(finding); err != nil {
		return fmt.Errorf("failed to record finding: %w", err)
	}

	return nil
}

// Finalize computes file hashes, builds the final run summary, and closes the JSONL writer
func (r *Recorder) Finalize(exitCode int) error {
	duration := time.Since(r.startTime)

	// Compute file hashes and metadata for all scanned files
	if err := r.computeFileMetadata(); err != nil {
		return fmt.Errorf("failed to compute file metadata: %w", err)
	}

	// Build scanned files list
	scannedFiles := make([]ScannedFile, 0, len(r.fileMetadata))
	for _, file := range r.fileMetadata {
		scannedFiles = append(scannedFiles, *file)
	}

	// Calculate statistics
	totalErrors := 0
	totalWarnings := 0
	for _, err := range r.errors {
		if err.Severity == "error" {
			totalErrors++
		} else {
			totalWarnings++
		}
	}

	// Count unique files and rules
	uniqueFiles := len(r.fileMetadata)
	uniqueRules := countUniqueRules(r.errors)

	// Build run summary
	summary := &RunRecord{
		RunID:         r.runID,
		Timestamp:     r.startTime,
		RepoPath:      r.repoRoot,
		WorkflowsPath: r.workflowsPath,
		Event:         r.event,
		Duration:      duration,
		ExitCode:      exitCode,
		TotalErrors:   totalErrors,
		TotalWarnings: totalWarnings,
		UniqueFiles:   uniqueFiles,
		UniqueRules:   uniqueRules,
		ScannedFiles:  scannedFiles,
		Errors:        r.errors,
	}

	// Write summary to JSONL
	if err := r.jsonl.WriteRunSummary(summary); err != nil {
		return fmt.Errorf("failed to write run summary: %w", err)
	}

	// Close JSONL writer
	if err := r.jsonl.Close(); err != nil {
		return fmt.Errorf("failed to close JSONL writer: %w", err)
	}

	return nil
}

// computeFileMetadata computes hashes and metadata for all files with errors
func (r *Recorder) computeFileMetadata() error {
	for filePath := range r.errorCounts {
		if filePath == "" {
			continue // Skip errors without file paths
		}

		// Make path absolute relative to repo root
		absPath := filePath
		if !filepath.IsAbs(filePath) {
			absPath = filepath.Join(r.repoRoot, filePath)
		}

		errorCount := r.errorCounts[filePath]
		warningCount := r.warningCounts[filePath]

		scannedFile, err := BuildScannedFile(absPath, errorCount, warningCount)
		if err != nil {
			// Log warning but don't fail - file might have been deleted
			fmt.Printf("Warning: failed to compute metadata for %s: %v\n", filePath, err)
			continue
		}

		r.fileMetadata[filePath] = scannedFile
	}

	return nil
}

// countUniqueRules counts the number of unique rule IDs in errors
func countUniqueRules(extractedErrors []*errors.ExtractedError) int {
	rules := make(map[string]struct{})
	for _, err := range extractedErrors {
		if err.RuleID != "" {
			rules[err.RuleID] = struct{}{}
		}
	}
	return len(rules)
}

// GetOutputPath returns the path to the JSONL output file
func (r *Recorder) GetOutputPath() string {
	return r.jsonl.Path()
}
