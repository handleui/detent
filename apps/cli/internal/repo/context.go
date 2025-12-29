package repo

import (
	"fmt"
	"path/filepath"

	"github.com/detent/cli/internal/git"
)

// Context contains resolved repository information.
// Use Resolve() or ResolveWith() to create an instance.
type Context struct {
	Path           string // Absolute path to the repository root
	FirstCommitSHA string // SHA of the first (root) commit for repo identification
	RunID          string // Deterministic run ID from current codebase state
	TreeHash       string // Current tree hash
	CommitSHA      string // Current commit SHA
}

// Option configures Context resolution behavior.
type Option func(*resolveConfig)

type resolveConfig struct {
	includeFirstCommit bool
	includeRunID       bool
}

// WithFirstCommit includes the first commit SHA in the resolved context.
// Useful for per-repo configuration lookups.
func WithFirstCommit() Option {
	return func(c *resolveConfig) {
		c.includeFirstCommit = true
	}
}

// WithRunID includes the run ID, tree hash, and commit SHA in the resolved context.
// Useful for deterministic cache keys and worktree management.
func WithRunID() Option {
	return func(c *resolveConfig) {
		c.includeRunID = true
	}
}

// WithAll includes all optional fields (first commit and run ID).
// Equivalent to calling WithFirstCommit() and WithRunID().
func WithAll() Option {
	return func(c *resolveConfig) {
		c.includeFirstCommit = true
		c.includeRunID = true
	}
}

// Resolve resolves the repository context from the current directory.
// By default, only resolves the repository path. Use options to include
// additional fields:
//
//	ctx, err := repo.Resolve()                     // Path only
//	ctx, err := repo.Resolve(repo.WithFirstCommit()) // Path + FirstCommitSHA
//	ctx, err := repo.Resolve(repo.WithRunID())     // Path + RunID/TreeHash/CommitSHA
//	ctx, err := repo.Resolve(repo.WithAll())       // All fields
func Resolve(opts ...Option) (*Context, error) {
	cfg := &resolveConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	repoPath, err := filepath.Abs(".")
	if err != nil {
		return nil, fmt.Errorf("resolving current directory: %w", err)
	}

	ctx := &Context{
		Path: repoPath,
	}

	if cfg.includeFirstCommit {
		firstCommitSHA, err := git.GetFirstCommitSHA(repoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to identify repository: %w", err)
		}
		ctx.FirstCommitSHA = firstCommitSHA
	}

	if cfg.includeRunID {
		runID, treeHash, commitSHA, err := git.ComputeCurrentRunID(repoPath)
		if err != nil {
			return nil, err
		}
		ctx.RunID = runID
		ctx.TreeHash = treeHash
		ctx.CommitSHA = commitSHA
	}

	return ctx, nil
}
