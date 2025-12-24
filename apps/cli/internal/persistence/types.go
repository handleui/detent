package persistence

import (
	"time"
)

// ScannedFile tracks metadata about a file that was scanned
type ScannedFile struct {
	Path         string    `json:"path"`
	Hash         string    `json:"hash"`         // SHA256 hash
	Size         int64     `json:"size"`         // File size in bytes
	ModTime      time.Time `json:"mod_time"`     // Last modification time
	ErrorCount   int       `json:"error_count"`  // Number of errors found in this file
	WarningCount int       `json:"warning_count"` // Number of warnings found in this file
}

// FindingRecord represents a single finding for JSONL streaming
// This is written as each error is extracted during scanning
type FindingRecord struct {
	Timestamp time.Time `json:"timestamp"`
	RunID     string    `json:"run_id"`

	// File information
	FilePath string    `json:"file_path"`
	FileHash string    `json:"file_hash,omitempty"`
	FileSize int64     `json:"file_size,omitempty"`
	ModTime  time.Time `json:"mod_time,omitempty"`

	// Error details (from ExtractedError)
	Message    string `json:"message"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
	Severity   string `json:"severity"` // "error" or "warning"
	StackTrace string `json:"stack_trace,omitempty"`
	RuleID     string `json:"rule_id,omitempty"`
	Category   string `json:"category"` // lint, type-check, test, compile, runtime
	Source     string `json:"source"`   // eslint, typescript, go, etc.

	// Workflow context
	WorkflowJob  string `json:"workflow_job,omitempty"`
	WorkflowStep string `json:"workflow_step,omitempty"`

	// Raw output for debugging
	Raw string `json:"raw"`
}
