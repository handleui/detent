package workflow

// Workflow represents a GitHub Actions workflow file
type Workflow struct {
	Name        string            `yaml:"name,omitempty"`
	On          any               `yaml:"on,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	Jobs        map[string]*Job   `yaml:"jobs"`
	Defaults    any               `yaml:"defaults,omitempty"`
	Concurrency any               `yaml:"concurrency,omitempty"`
	Permissions any               `yaml:"permissions,omitempty"`
}

// Job represents a job in a workflow
type Job struct {
	Name            string            `yaml:"name,omitempty"`
	RunsOn          any               `yaml:"runs-on"`
	Steps           []*Step           `yaml:"steps"`
	Env             map[string]string `yaml:"env,omitempty"`
	If              string            `yaml:"if,omitempty"`
	Needs           any               `yaml:"needs,omitempty"`
	Strategy        any               `yaml:"strategy,omitempty"`
	Container       any               `yaml:"container,omitempty"`
	Services        any               `yaml:"services,omitempty"`
	Outputs         any               `yaml:"outputs,omitempty"`
	Permissions     any               `yaml:"permissions,omitempty"`
	ContinueOnError any               `yaml:"continue-on-error,omitempty"`
	TimeoutMinutes  any               `yaml:"timeout-minutes,omitempty"`
	Defaults        any               `yaml:"defaults,omitempty"`
	Concurrency     any               `yaml:"concurrency,omitempty"`
	Environment     any               `yaml:"environment,omitempty"` // Deployment environment (string or object with name/url)
	Uses            string            `yaml:"uses,omitempty"`        // Reusable workflow reference
	With            map[string]any    `yaml:"with,omitempty"`        // Inputs for reusable workflow
	Secrets         any               `yaml:"secrets,omitempty"`     // Secrets for reusable workflow
}

// JobInfo contains extracted job information for TUI display
type JobInfo struct {
	ID    string   // Job ID (key in jobs map, e.g., "cli-lint")
	Name  string   // Display name (job.name or fallback to ID)
	Needs []string // Job IDs this job depends on
}

// Step represents a step in a job
type Step struct {
	ID              string            `yaml:"id,omitempty"`
	Name            string            `yaml:"name,omitempty"`
	Uses            string            `yaml:"uses,omitempty"`
	Run             string            `yaml:"run,omitempty"`
	With            map[string]any    `yaml:"with,omitempty"`
	Env             map[string]string `yaml:"env,omitempty"`
	If              string            `yaml:"if,omitempty"`
	ContinueOnError bool              `yaml:"continue-on-error,omitempty"`
	TimeoutMinutes  any               `yaml:"timeout-minutes,omitempty"`
	WorkingDirectory string           `yaml:"working-directory,omitempty"`
	Shell           string            `yaml:"shell,omitempty"`
}
