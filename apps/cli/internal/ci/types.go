package ci

// JobStatus represents the status of a workflow job.
type JobStatus string

// JobStatus values representing the possible states of a tracked job.
const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobSuccess JobStatus = "success"
	JobFailed  JobStatus = "failed"
)

// JobEvent represents a job lifecycle event parsed from CI output.
type JobEvent struct {
	JobName string // Display name of the job
	Action  string // "start" or "finish"
	Success bool   // Only relevant when Action="finish"
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
