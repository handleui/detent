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

// HealStatus represents the lifecycle state of a heal
type HealStatus string

// HealStatus constants
const (
	HealStatusPending  HealStatus = "pending"
	HealStatusApplied  HealStatus = "applied"
	HealStatusRejected HealStatus = "rejected"
	HealStatusFailed   HealStatus = "failed"
)

// VerificationResult represents the outcome of running verification
type VerificationResult string

// VerificationResult constants
const (
	VerificationPassed VerificationResult = "passed"
	VerificationFailed VerificationResult = "failed"
)

// HealRecord represents an AI-recommended fix for an error
type HealRecord struct {
	HealID             string             `json:"heal_id"`
	ErrorID            string             `json:"error_id"`
	RunID              string             `json:"run_id,omitempty"`
	DiffContent        string             `json:"diff_content"`
	DiffContentHash    string             `json:"diff_content_hash,omitempty"`
	FilePath           string             `json:"file_path,omitempty"`
	FileHash           string             `json:"file_hash,omitempty"`
	ModelID            string             `json:"model_id,omitempty"`
	PromptHash         string             `json:"prompt_hash,omitempty"`
	InputTokens        int                `json:"input_tokens"`
	OutputTokens       int                `json:"output_tokens"`
	CacheReadTokens    int                `json:"cache_read_tokens"`
	CacheWriteTokens   int                `json:"cache_write_tokens"`
	CostUSD            float64            `json:"cost_usd"`
	Status             HealStatus         `json:"status"`
	CreatedAt          time.Time          `json:"created_at"`
	AppliedAt          *time.Time         `json:"applied_at,omitempty"`
	VerifiedAt         *time.Time         `json:"verified_at,omitempty"`
	VerificationResult VerificationResult `json:"verification_result,omitempty"`
	AttemptNumber      int                `json:"attempt_number"`
	ParentHealID       *string            `json:"parent_heal_id,omitempty"`
	FailureReason      *string            `json:"failure_reason,omitempty"`
}

// ErrorLocation tracks where an error appears across runs
type ErrorLocation struct {
	LocationID   string    `json:"location_id"`
	ErrorID      string    `json:"error_id"`
	RunID        string    `json:"run_id"`
	FilePath     string    `json:"file_path"`
	LineNumber   int       `json:"line_number"`
	ColumnNumber int       `json:"column_number"`
	FileHash     string    `json:"file_hash,omitempty"`
	FirstSeenAt  time.Time `json:"first_seen_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	SeenCount    int       `json:"seen_count"`
}
