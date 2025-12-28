package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
)

// Tool defines the interface all heal tools must implement.
type Tool interface {
	// Name returns the tool name as Claude will call it.
	Name() string

	// Description returns the tool description for the prompt.
	Description() string

	// InputSchema returns the JSON schema for the tool input.
	InputSchema() map[string]any

	// Execute runs the tool with the given input in the worktree context.
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

// Result encapsulates the result of a tool execution.
type Result struct {
	// Content is the result content to send back to Claude.
	Content string

	// IsError indicates whether this is an error result.
	// Error results are returned to Claude (not as Go errors) so it can learn and retry.
	IsError bool

	// Metadata contains optional data for logging/debugging.
	Metadata map[string]any
}

// ErrorResult creates an error result with the given message.
func ErrorResult(msg string) Result {
	return Result{Content: msg, IsError: true}
}

// SuccessResult creates a success result with the given content.
func SuccessResult(content string) Result {
	return Result{Content: content, IsError: false}
}

// TargetApprovalResult contains the user's decision about a make target.
type TargetApprovalResult struct {
	Allowed bool // Target is allowed
	Always  bool // Persist to config (vs session-only)
}

// Context provides the execution context for tools.
type Context struct {
	// WorktreePath is the absolute path to the worktree where fixes are applied.
	WorktreePath string

	// RepoRoot is the original repo root (for reference only, tools should not modify).
	RepoRoot string

	// RunID is the current run identifier.
	RunID string

	// FirstCommitSHA is the repo identifier for per-repo config lookup.
	FirstCommitSHA string

	// ApprovedTargets tracks make targets approved by the user for this session.
	// Session-scoped: does not persist to disk.
	ApprovedTargets map[string]bool

	// DeniedTargets tracks make targets denied by the user for this session.
	// Prevents infinite retry loops when AI keeps trying the same target.
	DeniedTargets map[string]bool

	// TargetApprover is a callback to prompt the user for approval of unknown make targets.
	// Returns approval result (allowed + persist preference). If nil, unknown targets are rejected.
	TargetApprover func(target string) (TargetApprovalResult, error)

	// TargetPersister is a callback to persist a target approval to global config.
	// Called when user selects "always for this repo". If nil, persistence is skipped.
	TargetPersister func(target string) error

	// RepoTargetChecker checks if a target is approved for this repo in global config.
	// Returns true if the target is persisted in the repo's approved_targets list.
	RepoTargetChecker func(target string) bool
}

// IsTargetApproved checks if a make target has been approved for this session.
// Case-insensitive to match isAllowedMakeTarget behavior.
func (c *Context) IsTargetApproved(target string) bool {
	if c.ApprovedTargets == nil {
		return false
	}
	return c.ApprovedTargets[strings.ToLower(target)]
}

// ApproveTarget marks a make target as approved for this session.
// Stores lowercase to ensure case-insensitive matching.
func (c *Context) ApproveTarget(target string) {
	if c.ApprovedTargets == nil {
		c.ApprovedTargets = make(map[string]bool)
	}
	c.ApprovedTargets[strings.ToLower(target)] = true
}

// IsTargetDenied checks if a make target was denied by user this session.
func (c *Context) IsTargetDenied(target string) bool {
	if c.DeniedTargets == nil {
		return false
	}
	return c.DeniedTargets[strings.ToLower(target)]
}

// DenyTarget marks a make target as denied for this session.
// Prevents prompting for the same target again.
func (c *Context) DenyTarget(target string) {
	if c.DeniedTargets == nil {
		c.DeniedTargets = make(map[string]bool)
	}
	c.DeniedTargets[strings.ToLower(target)] = true
}

// ValidatePath checks if a path is within the worktree and returns the absolute path.
// Returns an error result if the path escapes the worktree.
func (c *Context) ValidatePath(relPath string) (string, *Result) {
	// Clean the path to resolve . and ..
	cleanPath := filepath.Clean(relPath)

	// Reject absolute paths to prevent directory traversal
	if filepath.IsAbs(cleanPath) {
		r := ErrorResult("absolute paths not allowed: " + relPath)
		return "", &r
	}

	absPath := filepath.Join(c.WorktreePath, cleanPath)

	// Use filepath.Rel for robust containment check.
	// If the relative path starts with "..", it escapes the worktree.
	rel, err := filepath.Rel(c.WorktreePath, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		r := ErrorResult("path escapes worktree: " + relPath)
		return "", &r
	}

	return absPath, nil
}
