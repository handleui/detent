package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
)

// Tool defines the interface all heal tools must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

// Result encapsulates the result of a tool execution.
type Result struct {
	Content  string
	IsError  bool
	Metadata map[string]any
}

// ErrorResult creates an error result.
func ErrorResult(msg string) Result {
	return Result{Content: msg, IsError: true}
}

// SuccessResult creates a success result.
func SuccessResult(content string) Result {
	return Result{Content: content, IsError: false}
}

// CommandApproval contains the user's decision about a command.
type CommandApproval struct {
	Allowed bool
	Always  bool
}

// Context provides the execution context for tools.
type Context struct {
	WorktreePath   string
	RepoRoot       string
	RunID          string
	FirstCommitSHA string

	// Command approval - unified for all commands (including make)
	ApprovedCommands map[string]bool
	DeniedCommands   map[string]bool

	// mu protects ApprovedCommands and DeniedCommands maps
	mu sync.RWMutex

	// CommandChecker checks if command is in local config
	CommandChecker func(cmd string) bool

	// CommandApprover prompts user for unknown commands
	CommandApprover func(cmd string) (CommandApproval, error)

	// CommandPersister saves approved command to config
	CommandPersister func(cmd string) error
}

// IsCommandApproved checks if command was approved this session.
func (c *Context) IsCommandApproved(cmd string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ApprovedCommands == nil {
		return false
	}
	return c.ApprovedCommands[cmd]
}

// ApproveCommand marks command as approved for this session.
func (c *Context) ApproveCommand(cmd string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ApprovedCommands == nil {
		c.ApprovedCommands = make(map[string]bool)
	}
	c.ApprovedCommands[cmd] = true
}

// IsCommandDenied checks if command was denied this session.
func (c *Context) IsCommandDenied(cmd string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.DeniedCommands == nil {
		return false
	}
	return c.DeniedCommands[cmd]
}

// DenyCommand marks command as denied for this session.
func (c *Context) DenyCommand(cmd string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.DeniedCommands == nil {
		c.DeniedCommands = make(map[string]bool)
	}
	c.DeniedCommands[cmd] = true
}

// ValidatePath checks if a path is within the worktree.
func (c *Context) ValidatePath(relPath string) (string, *Result) {
	cleanPath := filepath.Clean(relPath)

	if filepath.IsAbs(cleanPath) {
		r := ErrorResult("absolute paths not allowed: " + relPath)
		return "", &r
	}

	absPath := filepath.Join(c.WorktreePath, cleanPath)

	rel, err := filepath.Rel(c.WorktreePath, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		r := ErrorResult("path escapes worktree: " + relPath)
		return "", &r
	}

	return absPath, nil
}
