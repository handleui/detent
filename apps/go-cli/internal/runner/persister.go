package runner

import (
	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/git"
)

// ResultPersister handles database persistence of check run results.
// NOTE: Persistence has been migrated to TypeScript. This is a stub.
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
// NOTE: This is a stub - persistence has been migrated to TypeScript.
func (p *ResultPersister) Persist(_ []*errors.ExtractedError, _ int) error {
	// Persistence migrated to TypeScript CLI
	return nil
}
