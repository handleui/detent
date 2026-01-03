// Package errors provides error extraction, categorization, and serialization
// for workflow execution results.
package errors

import "strings"

// OrchestratorError is a lightweight error view for the orchestrator.
// It excludes CodeSnippet, StackTrace, and other detail fields to minimize tokens.
type OrchestratorError struct {
	File        string        `json:"file,omitempty"`         // Source file path
	Line        int           `json:"line,omitempty"`         // Line number (1-indexed)
	Message     string        `json:"message"`                // Error message
	Severity    string        `json:"severity"`               // "error" or "warning"
	Category    ErrorCategory `json:"category"`               // Error category (lint, type-check, etc.)
	Source      string        `json:"source"`                 // Tool that produced the error (eslint, typescript, etc.)
	RuleID      string        `json:"rule_id,omitempty"`      // Rule identifier (e.g., "no-var", "TS2749")
	WorkflowJob string        `json:"workflow_job,omitempty"` // GitHub Actions job name
}

// OrchestratorView is the lightweight structure for the Haiku orchestrator.
// It contains a minimal representation of errors suitable for high-level routing decisions.
type OrchestratorView struct {
	Errors []OrchestratorError `json:"errors"` // Lightweight error list
	Stats  ErrorStats          `json:"stats"`  // Aggregated statistics
}

// ForOrchestrator returns a lightweight view suitable for the Haiku orchestrator.
// It strips CodeSnippet, StackTrace, Suggestions, and other detail fields.
// Returns nil if the receiver is nil.
func (g *ComprehensiveErrorGroup) ForOrchestrator() *OrchestratorView {
	if g == nil {
		return nil
	}
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
// Returns nil if the receiver is nil, or empty slice if filter is nil.
func (g *ComprehensiveErrorGroup) ForAgent(filter func(*ExtractedError) bool) []*ExtractedError {
	if g == nil {
		return nil
	}
	if filter == nil {
		return []*ExtractedError{}
	}
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
