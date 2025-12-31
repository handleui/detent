package ci

// JobStatus represents the status of a workflow job.
type JobStatus string

// JobStatus values representing the possible states of a tracked job.
const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobSuccess JobStatus = "success"
	JobFailed  JobStatus = "failed"
	JobSkipped JobStatus = "skipped"
)

// JobEvent represents a job lifecycle event parsed from CI output.
type JobEvent struct {
	JobID   string // Job ID (key in workflow jobs map)
	Action  string // "start", "finish", or "skip"
	Success bool   // Only relevant when Action="finish"
}

// StepStatus represents the status of a workflow step.
type StepStatus string

// StepStatus values representing the possible states of a tracked step.
const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepSuccess   StepStatus = "success"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
	StepCancelled StepStatus = "cancelled"
)

// StepEvent represents a step lifecycle event parsed from CI output.
type StepEvent struct {
	JobID    string // Job ID this step belongs to
	StepIdx  int    // Step index (0-based)
	StepName string // Step display name
}

// ManifestJob contains information about a single job in the manifest.
type ManifestJob struct {
	ID    string   `json:"id"`              // Job ID (key in jobs map)
	Name  string   `json:"name"`            // Display name
	Steps []string `json:"steps,omitempty"` // Step names in order (empty for uses: jobs)
	Needs []string `json:"needs,omitempty"` // Job IDs this job depends on
	Uses  string   `json:"uses,omitempty"`  // Reusable workflow reference (if present, no steps)
}

// ManifestInfo contains the full manifest for a workflow run.
// This is the v2 manifest format that includes step information.
type ManifestInfo struct {
	Version int           `json:"v"`    // Manifest version (2 for this format)
	Jobs    []ManifestJob `json:"jobs"` // All jobs in topological order
}

// ManifestEvent is emitted when a manifest is parsed from CI output.
// This initializes the TUI with all job and step information.
type ManifestEvent struct {
	Manifest *ManifestInfo
}

// Parser defines the interface for parsing CI output into job events.
// Different CI systems (act, GitHub Actions) implement this interface.
type Parser interface {
	// ParseLine parses a single line of CI output.
	// Returns a JobEvent and true if the line contains a job event, nil and false otherwise.
	ParseLine(line string) (*JobEvent, bool)
}

// LineContext contains CI platform-specific context extracted from a log line.
type LineContext struct {
	Job     string // Job name from CI output
	Step    string // Step name (if parseable)
	IsNoise bool   // True if line should be skipped (debug output)
}

// ContextParser extracts CI platform-specific context from log lines.
// Different CI systems (act, GitHub Actions, GitLab) implement this interface
// to parse their specific output formats and extract job/step context.
type ContextParser interface {
	// ParseLine extracts context from a CI log line.
	// Returns the context, the cleaned line (with CI prefixes removed), and whether to skip.
	// If skip is true, the line should be ignored (debug noise, metadata).
	ParseLine(line string) (ctx *LineContext, cleanLine string, skip bool)
}
