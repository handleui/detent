package git

import "fmt"

// ErrWorktreeNotInitialized is returned when operations are attempted before worktree creation
var ErrWorktreeNotInitialized = fmt.Errorf("worktree not initialized - Prepare() must be called before Run()")

// ValidateWorktreeInitialized checks if worktree info is present
// Returns ErrWorktreeNotInitialized if nil
func ValidateWorktreeInitialized(info *WorktreeInfo) error {
	if info == nil {
		return ErrWorktreeNotInitialized
	}
	return nil
}

// RequireWorktreeInitialized panics if worktree is not initialized
// Use this for critical paths where nil should never occur (defense in depth)
func RequireWorktreeInitialized(info *WorktreeInfo) {
	if info == nil {
		panic("CRITICAL: worktree not initialized - this is a bug in the runner state machine")
	}
}
