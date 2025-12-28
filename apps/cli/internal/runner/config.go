package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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

	// DryRun skips actual workflow execution and shows simulated UI
	DryRun bool
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

	if c.RunID == "" {
		return fmt.Errorf("RunID is required")
	}
	if !validRunIDPattern.MatchString(c.RunID) {
		return fmt.Errorf("invalid RunID format: must be a 16-character hex string")
	}

	// Validate WorkflowPath doesn't escape RepoRoot
	absRepo, err := filepath.Abs(c.RepoRoot)
	if err != nil {
		return fmt.Errorf("resolving RepoRoot: %w", err)
	}

	absWorkflow, err := filepath.Abs(c.WorkflowPath)
	if err != nil {
		return fmt.Errorf("resolving WorkflowPath: %w", err)
	}

	relPath, err := filepath.Rel(absRepo, absWorkflow)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("WorkflowPath must be within RepoRoot")
	}

	// Check if path exists and is not a symlink
	fileInfo, err := os.Lstat(absWorkflow)
	if err != nil {
		// Path doesn't exist yet - that's ok, PrepareWorkflows will handle it
		// But we still validated it's within RepoRoot
	} else if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("WorkflowPath cannot be a symlink")
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

// ActTimeout returns the default timeout for act execution.
// This is currently a package-level constant but could be made configurable in the future.
const ActTimeout = 35 * time.Minute

// WorkflowsDir is the default directory containing workflow files
const WorkflowsDir = ".github/workflows"
