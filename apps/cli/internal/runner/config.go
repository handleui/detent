package runner

import (
	"fmt"
	"regexp"
	"time"
)

var validEventPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

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
}

// Validate checks that all required fields are set and sets defaults for optional fields.
// Returns an error if validation fails.
func (c *RunConfig) Validate() error {
	if c.RepoRoot == "" {
		return fmt.Errorf("RepoRoot is required")
	}

	if c.WorkflowPath == "" {
		return fmt.Errorf("WorkflowPath is required")
	}

	if c.Event == "" {
		c.Event = "push" // Default to push event
	}

	// Validate event name format
	if !validEventPattern.MatchString(c.Event) {
		return fmt.Errorf("invalid event name %q: must contain only alphanumeric, underscore, or hyphen", c.Event)
	}

	if c.RunID == "" {
		return fmt.Errorf("RunID is required")
	}

	return nil
}

// ResolveWorkflowPath returns the workflow path.
// This is a helper method that returns the WorkflowPath field.
// In the future, this could be enhanced to resolve relative paths or apply transformations.
func (c *RunConfig) ResolveWorkflowPath() string {
	return c.WorkflowPath
}

// ActTimeout returns the default timeout for act execution.
// This is currently a package-level constant but could be made configurable in the future.
const ActTimeout = 35 * time.Minute

// WorkflowsDir is the default directory containing workflow files
const WorkflowsDir = ".github/workflows"
