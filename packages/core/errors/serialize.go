package errors

import "strings"

// OrchestratorError is a lightweight error view for the orchestrator.
// It excludes CodeSnippet, StackTrace, and other detail fields to minimize tokens.
type OrchestratorError struct {
	File        string        `json:"file,omitempty"`
	Line        int           `json:"line,omitempty"`
	Message     string        `json:"message"`
	Severity    string        `json:"severity"`
	Category    ErrorCategory `json:"category"`
	Source      string        `json:"source"`
	RuleID      string        `json:"rule_id,omitempty"`
	WorkflowJob string        `json:"workflow_job,omitempty"`
}

// OrchestratorView is the lightweight structure for the Haiku orchestrator.
type OrchestratorView struct {
	Errors []OrchestratorError `json:"errors"`
	Stats  ErrorStats          `json:"stats"`
}

// ForOrchestrator returns a lightweight view suitable for the Haiku orchestrator.
// It strips CodeSnippet, StackTrace, Suggestions, and other detail fields.
func (g *ComprehensiveErrorGroup) ForOrchestrator() *OrchestratorView {
	allErrors := g.flatten()
	orchestratorErrors := make([]OrchestratorError, 0, len(allErrors))

	for _, err := range allErrors {
		orchErr := OrchestratorError{
			File:     err.File,
			Line:     err.Line,
			Message:  err.Message,
			Severity: err.Severity,
			Category: err.Category,
			Source:   err.Source,
			RuleID:   err.RuleID,
		}

		if err.WorkflowContext != nil && err.WorkflowContext.Job != "" {
			orchErr.WorkflowJob = err.WorkflowContext.Job
		}

		orchestratorErrors = append(orchestratorErrors, orchErr)
	}

	return &OrchestratorView{
		Errors: orchestratorErrors,
		Stats:  g.Stats,
	}
}

// ForAgent filters errors and returns full detail for a specific agent.
// The filter function determines which errors to include.
func (g *ComprehensiveErrorGroup) ForAgent(filter func(*ExtractedError) bool) []*ExtractedError {
	allErrors := g.flatten()
	result := make([]*ExtractedError, 0)

	for _, err := range allErrors {
		if filter(err) {
			result = append(result, err)
		}
	}

	return result
}

// flatten reconstructs a linear list of errors from the grouped structure.
func (g *ComprehensiveErrorGroup) flatten() []*ExtractedError {
	result := make([]*ExtractedError, 0, g.Total)
	for _, errs := range g.ByFile {
		result = append(result, errs...)
	}
	result = append(result, g.NoFile...)
	return result
}

// FilterByCategory returns a filter for errors matching the given category.
func FilterByCategory(cat ErrorCategory) func(*ExtractedError) bool {
	return func(err *ExtractedError) bool {
		return err.Category == cat
	}
}

// FilterByFile returns a filter for errors in files matching the prefix.
func FilterByFile(prefix string) func(*ExtractedError) bool {
	return func(err *ExtractedError) bool {
		return strings.HasPrefix(err.File, prefix)
	}
}

// FilterBySeverity returns a filter for errors matching the given severity.
func FilterBySeverity(sev string) func(*ExtractedError) bool {
	return func(err *ExtractedError) bool {
		return err.Severity == sev
	}
}
