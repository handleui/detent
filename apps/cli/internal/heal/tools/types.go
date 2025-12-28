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

// Context provides the execution context for tools.
type Context struct {
	// WorktreePath is the absolute path to the worktree where fixes are applied.
	WorktreePath string

	// RepoRoot is the original repo root (for reference only, tools should not modify).
	RepoRoot string

	// RunID is the current run identifier.
	RunID string
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
