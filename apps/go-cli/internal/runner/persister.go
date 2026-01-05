package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/detent/go-cli/internal/persistence"
	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/git"
)

// ResultPersister handles database persistence of check run results.
// It records findings and run metadata to the .detent directory.
type ResultPersister struct {
	repoRoot     string
	workflowPath string
	worktreeInfo *git.WorktreeInfo
	runID        string
}

// NewResultPersister creates a new ResultPersister with the given configuration.
// runID should be the deterministic RunID from RunConfig (computed from codebase state).
func NewResultPersister(repoRoot, workflowPath, runID string, worktreeInfo *git.WorktreeInfo) *ResultPersister {
	return &ResultPersister{
		repoRoot:     repoRoot,
		workflowPath: workflowPath,
		worktreeInfo: worktreeInfo,
		runID:        runID,
	}
}

// Persist saves the check run results to the database.
// This writes the complete run result including:
// - Run metadata (ID, timing, exit code)
// - Worktree information (commit SHA)
// - Extracted errors with full context
//
// Returns error if persistence fails.
func (p *ResultPersister) Persist(extracted []*errors.ExtractedError, exitCode int) error {
	if p.worktreeInfo == nil {
		return fmt.Errorf("no worktree info available (Prepare/PrepareWithTUI must be called first)")
	}

	// Extract workflow name from config.WorkflowPath
	// If WorkflowPath is a file, use its base name; otherwise use "all"
	workflowName := "all"
	fileInfo, err := os.Stat(p.workflowPath)
	if err == nil && !fileInfo.IsDir() {
		workflowName = filepath.Base(p.workflowPath)
	}

	// Detect execution mode
	execMode := git.DetectExecutionMode()

	// Initialize persistence recorder with deterministic runID
	recorder, err := persistence.NewRecorder(
		p.repoRoot,
		workflowName,
		p.worktreeInfo.CommitSHA,
		execMode,
		p.runID,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence storage at %s/.detent: %w", p.repoRoot, err)
	}

	// Record all findings in a single batch operation
	if err := recorder.RecordFindings(extracted); err != nil {
		return fmt.Errorf("failed to record findings: %w", err)
	}

	// Finalize the run with exit code (this also closes the database connection)
	if err := recorder.Finalize(exitCode); err != nil {
		return fmt.Errorf("failed to finalize persistence storage (run data may be incomplete): %w", err)
	}

	return nil
}
