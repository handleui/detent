package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validEventPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var validRunIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// RunConfig configures a workflow execution run.
// This is the high-level configuration that orchestrates the entire check workflow,
// including preflight checks, worktree management, act execution, and result processing.
type RunConfig struct {
	// RepoRoot is the absolute path to the repository root directory
	RepoRoot string

	// WorkflowPath is the path to the workflow directory
	// (e.g., ".github/workflows")
	WorkflowPath string

	// WorkflowFile is an optional specific workflow file name
	// (e.g., "ci.yml"). If empty, all workflows in WorkflowPath are processed.
	WorkflowFile string

	// Event is the GitHub event type to trigger (e.g., "push", "pull_request")
	Event string

	// UseTUI determines whether to use the terminal UI (Bubble Tea) for execution
	UseTUI bool

	// StreamOutput determines whether to stream act output to stderr in real-time
	// Only applicable when UseTUI is false
	StreamOutput bool

	// RunID is a unique identifier for this run (UUID)
	RunID string

	// IsAgentMode is true when running in an AI agent environment (Claude Code, Cursor, etc.)
	// This enables verbose output and skips interactive prompts
	IsAgentMode bool

	// AgentName is the detected AI agent name (e.g., "Claude Code")
	AgentName string
}

// Validate checks that all required fields are set and sets defaults for optional fields.
// Returns an error if validation fails.
func (c *RunConfig) Validate() error {
	if c.RepoRoot == "" {
		return fmt.Errorf("repoRoot is required")
	}

	if c.WorkflowPath == "" {
		return fmt.Errorf("workflowPath is required")
	}

	if c.RunID == "" {
		return fmt.Errorf("runID is required")
	}
	if !validRunIDPattern.MatchString(c.RunID) {
		return fmt.Errorf("invalid runID format: must be a 16-character hex string")
	}

	// Validate WorkflowPath doesn't escape RepoRoot
	absRepo, err := filepath.Abs(c.RepoRoot)
	if err != nil {
		return fmt.Errorf("resolving repoRoot: %w", err)
	}

	absWorkflow, err := filepath.Abs(c.WorkflowPath)
	if err != nil {
		return fmt.Errorf("resolving workflowPath: %w", err)
	}

	relPath, err := filepath.Rel(absRepo, absWorkflow)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("workflowPath must be within repoRoot")
	}

	// Check if path exists and is not a symlink
	fileInfo, err := os.Lstat(absWorkflow)
	if err != nil {
		// Path doesn't exist yet - that's ok, PrepareWorkflows will handle it
		// But we still validated it's within RepoRoot
	} else if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workflowPath cannot be a symlink")
	}

	if c.Event == "" {
		c.Event = "push" // Default to push event
	}

	// Validate event name format
	if !validEventPattern.MatchString(c.Event) {
		return fmt.Errorf("invalid event name %q: must contain only alphanumeric, underscore, or hyphen", c.Event)
	}

	return nil
}

// ResolveWorkflowPath returns the workflow path.
// This is a helper method that returns the WorkflowPath field.
// In the future, this could be enhanced to resolve relative paths or apply transformations.
func (c *RunConfig) ResolveWorkflowPath() string {
	return c.WorkflowPath
}

// ActTimeout returns the timeout for act execution.
// The value can be configured via the DETENT_ACT_TIMEOUT environment variable.
// Default is 35 minutes if not configured.
var ActTimeout = GetActTimeout()

// WorkflowsDir is the default directory containing workflow files
const WorkflowsDir = ".github/workflows"
