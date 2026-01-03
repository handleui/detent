package persistence

import (
	"time"
)

// FindingRecord represents a single finding for persistence
// This is written as each error is extracted during scanning
type FindingRecord struct {
	Timestamp time.Time `json:"timestamp"`
	RunID     string    `json:"run_id"`

	// File information
	FilePath string `json:"file_path"`
	FileHash string `json:"file_hash,omitempty"`

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

	// Raw output for compliance/debugging
	Raw string `json:"raw,omitempty"`

	// Pre-computed content hash (optional, computed lazily if empty)
	ContentHash string `json:"content_hash,omitempty"`
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

// ============================================================================
// Assignment Types (Phase 1 Fix Protocol)
// ============================================================================

// AssignmentStatus represents the lifecycle state of an assignment
type AssignmentStatus string

// AssignmentStatus constants
const (
	AssignmentStatusAssigned   AssignmentStatus = "assigned"
	AssignmentStatusInProgress AssignmentStatus = "in_progress"
	AssignmentStatusCompleted  AssignmentStatus = "completed"
	AssignmentStatusFailed     AssignmentStatus = "failed"
	AssignmentStatusExpired    AssignmentStatus = "expired"
)

// Assignment represents a batch of errors assigned to a single agent for resolution.
// Each agent gets its own isolated environment (worktree, sandbox) to prevent conflicts.
type Assignment struct {
	// Identity
	AssignmentID string `json:"assignment_id"`
	RunID        string `json:"run_id"`

	// Agent context
	AgentID      string `json:"agent_id"`
	WorktreePath string `json:"worktree_path,omitempty"`

	// Scope (which errors to fix)
	ErrorCount int      `json:"error_count"`
	ErrorIDs   []string `json:"error_ids"`

	// Lifecycle
	Status      AssignmentStatus `json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
	StartedAt   *time.Time       `json:"started_at,omitempty"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	ExpiresAt   time.Time        `json:"expires_at"`

	// Result
	FixID         string `json:"fix_id,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// ============================================================================
// SuggestedFix Types (Phase 1 Fix Protocol)
// ============================================================================

// FixStatus represents the lifecycle state of a suggested fix
type FixStatus string

// FixStatus constants
const (
	FixStatusPending    FixStatus = "pending"
	FixStatusApplied    FixStatus = "applied"
	FixStatusRejected   FixStatus = "rejected"
	FixStatusSuperseded FixStatus = "superseded"
)

// SuggestedFix represents an AI-proposed fix for one or more errors.
// The fix is content-addressed: same code changes = same FixID.
type SuggestedFix struct {
	// Identity (content-addressed hash of file changes)
	FixID        string `json:"fix_id"`
	AssignmentID string `json:"assignment_id"`

	// Agent context (environment-agnostic)
	AgentID      string `json:"agent_id,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`

	// The actual fix (map of file path -> changes)
	FileChanges map[string]FileChange `json:"file_changes"`
	Explanation string                `json:"explanation,omitempty"`
	Confidence  int                   `json:"confidence"` // 1-100

	// Verification evidence
	Verification VerificationRecord `json:"verification"`

	// Cost tracking
	ModelID      string  `json:"model_id,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`

	// Lifecycle
	Status           FixStatus  `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	AppliedAt        *time.Time `json:"applied_at,omitempty"`
	AppliedBy        string     `json:"applied_by,omitempty"`
	AppliedCommitSHA string     `json:"applied_commit_sha,omitempty"`
	RejectedAt       *time.Time `json:"rejected_at,omitempty"`
	RejectedBy       string     `json:"rejected_by,omitempty"`
	RejectionReason  string     `json:"rejection_reason,omitempty"`

	// Related errors (populated from fix_errors junction table)
	ErrorIDs []string `json:"error_ids,omitempty"`
}

// FileChange captures a single file modification.
// Stores both full content (for apply/re-verify) and diff (for display).
type FileChange struct {
	// Full content for re-verification and apply
	BeforeContent string `json:"before_content,omitempty"`
	AfterContent  string `json:"after_content,omitempty"`

	// Compact display format
	UnifiedDiff string `json:"unified_diff,omitempty"`

	// Metadata
	LinesAdded   int  `json:"lines_added"`
	LinesRemoved int  `json:"lines_removed"`
	IsNew        bool `json:"is_new,omitempty"`
	IsDeleted    bool `json:"is_deleted,omitempty"`
}

// VerificationRecord captures evidence that the fix works.
type VerificationRecord struct {
	Command      string `json:"command"`
	ExitCode     int    `json:"exit_code"`
	Output       string `json:"output,omitempty"`
	DurationMs   int    `json:"duration_ms"`
	ErrorsBefore int    `json:"errors_before"`
	ErrorsAfter  int    `json:"errors_after"`
}

// ============================================================================
// Store Interfaces (Environment Abstraction)
// ============================================================================

// FixStore abstracts fix/assignment persistence for different environments.
// CLI uses SQLiteWriter (local), Cloud uses remote API.
type FixStore interface {
	// Assignment operations
	CreateAssignment(a *Assignment) error
	GetAssignment(assignmentID string) (*Assignment, error)
	UpdateAssignmentStatus(assignmentID string, status AssignmentStatus, fixID, failureReason string) error
	ListAssignmentsByRun(runID string) ([]*Assignment, error)

	// SuggestedFix operations
	StoreSuggestedFix(fix *SuggestedFix) error
	GetSuggestedFix(fixID string) (*SuggestedFix, error)
	ListPendingFixes(runID string) ([]*SuggestedFix, error)
	UpdateFixStatus(fixID string, status FixStatus, appliedBy, commitSHA, rejectedBy, rejectionReason string) error
}
