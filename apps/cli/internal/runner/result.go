package runner

import (
	"time"

	"github.com/detent/cli/internal/act"
	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
)

// RunResult contains the complete result of a workflow execution run.
// This aggregates results from act execution, error extraction, worktree info,
// and execution metadata into a single comprehensive result structure.
type RunResult struct {
	// ActResult contains the raw output and exit code from act execution
	ActResult *act.RunResult

	// Extracted contains all extracted errors from act output (flat list)
	Extracted []*errors.ExtractedError

	// Grouped contains errors organized by file for efficient reporting (used for JSON output)
	Grouped *errors.GroupedErrors

	// GroupedComprehensive contains errors organized by category and file for text output
	GroupedComprehensive *errors.ComprehensiveErrorGroup

	// WorktreeInfo contains metadata about the git worktree used for execution
	WorktreeInfo *git.WorktreeInfo

	// RunID is the unique identifier for this run (UUID)
	RunID string

	// StartTime is when the workflow execution began
	StartTime time.Time

	// Duration is how long the workflow execution took
	Duration time.Duration

	// Cancelled indicates whether the execution was cancelled by the user
	Cancelled bool

	// ExitCode is the exit code from act execution
	ExitCode int
}

// HasErrors returns true if the run result contains any errors (not warnings).
// Uses O(1) check from ComprehensiveErrorGroup stats.
func (r *RunResult) HasErrors() bool {
	if r.GroupedComprehensive == nil {
		return false
	}
	return r.GroupedComprehensive.Stats.ErrorCount > 0
}

// Success returns true if the workflow execution succeeded without errors.
// A successful run has exit code 0 and no errors (warnings are allowed).
func (r *RunResult) Success() bool {
	return r.ExitCode == 0 && !r.HasErrors()
}
