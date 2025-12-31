package errors

import (
	"path/filepath"
	"strings"
)

// ErrorCategory represents the type of error for categorization and AI prompt generation
type ErrorCategory string

// Error categories for workflow execution errors
const (
	CategoryLint      ErrorCategory = "lint"
	CategoryTypeCheck ErrorCategory = "type-check"
	CategoryTest      ErrorCategory = "test"
	CategoryCompile   ErrorCategory = "compile"
	CategoryRuntime   ErrorCategory = "runtime"
	CategoryMetadata  ErrorCategory = "metadata"
	CategoryUnknown   ErrorCategory = "unknown"
)

// Error sources for attribution and filtering
const (
	SourceESLint     = "eslint"
	SourceTypeScript = "typescript"
	SourceGo         = "go"
	SourceGoTest     = "go-test"
	SourcePython     = "python"
	SourceRust       = "rust"
	SourceDocker     = "docker"
	SourceNodeJS     = "nodejs"
	SourceMetadata   = "metadata"
)

// WorkflowContext captures GitHub Actions workflow execution context
type WorkflowContext struct {
	Job    string `json:"job,omitempty"`    // From [workflow/job] prefix in act output
	Step   string `json:"step,omitempty"`   // Future: parse from step names
	Action string `json:"action,omitempty"` // Future: parse from action names
}

// Clone creates a deep copy of WorkflowContext to prevent stale pointer sharing
func (w *WorkflowContext) Clone() *WorkflowContext {
	if w == nil {
		return nil
	}
	return &WorkflowContext{
		Job:    w.Job,
		Step:   w.Step,
		Action: w.Action,
	}
}

// ExtractedError represents a single error extracted from act output
type ExtractedError struct {
	Message         string           `json:"message"`
	File            string           `json:"file,omitempty"`
	Line            int              `json:"line,omitempty"`
	Column          int              `json:"column,omitempty"`
	Severity        string           `json:"severity,omitempty"`         // "error" or "warning"
	Raw             string           `json:"raw,omitempty"`
	StackTrace      string           `json:"stack_trace,omitempty"`      // Multi-line stack trace for detailed error context
	RuleID          string           `json:"rule_id,omitempty"`          // e.g., "no-var", "TS2749"
	Category        ErrorCategory    `json:"category,omitempty"`         // lint, type-check, test, etc.
	WorkflowContext *WorkflowContext `json:"workflow_context,omitempty"` // Job/step info
	Source          string           `json:"source,omitempty"`           // "eslint", "typescript", "go", etc.
}

// GroupedErrors groups errors by file path for organized output
type GroupedErrors struct {
	ByFile    map[string][]*ExtractedError `json:"by_file"`
	NoFile    []*ExtractedError            `json:"no_file"`
	Total     int                          `json:"total"`
	hasErrors bool                         // Track if any errors (not warnings) exist
}

// GroupByFile organizes extracted errors by their file paths
func GroupByFile(errs []*ExtractedError) *GroupedErrors {
	return GroupByFileWithBase(errs, "")
}

// GroupByFileWithBase organizes extracted errors by their file paths,
// making paths relative to basePath if provided
func GroupByFileWithBase(errs []*ExtractedError, basePath string) *GroupedErrors {
	grouped := &GroupedErrors{
		ByFile: make(map[string][]*ExtractedError),
		Total:  len(errs),
	}

	for _, err := range errs {
		// Track if we encounter any actual errors (not warnings)
		if err.Severity == "error" {
			grouped.hasErrors = true
		}

		if err.File != "" {
			file := err.File
			if basePath != "" {
				file = makeRelative(file, basePath)
			}
			grouped.ByFile[file] = append(grouped.ByFile[file], err)
		} else {
			grouped.NoFile = append(grouped.NoFile, err)
		}
	}

	return grouped
}

// HasErrors returns true if the grouped errors contain any errors (not warnings).
// This is tracked during grouping in O(n) time to avoid expensive nested loops.
func (g *GroupedErrors) HasErrors() bool {
	return g.hasErrors
}

// Flatten reconstructs a linear list of errors from the grouped structure.
// This is useful for persistence where you need all errors in a single slice.
// The method combines errors from all file groups with ungrouped errors.
func (g *GroupedErrors) Flatten() []*ExtractedError {
	result := make([]*ExtractedError, 0, g.Total)
	for _, errs := range g.ByFile {
		result = append(result, errs...)
	}
	result = append(result, g.NoFile...)
	return result
}

// makeRelative converts an absolute path to relative if it's under basePath.
// Uses filepath.Rel for correct path boundary handling (avoids false positives
// like "/home/user-data" matching "/home/user" prefix).
func makeRelative(path, basePath string) string {
	if basePath == "" || !filepath.IsAbs(path) {
		return path
	}

	rel, err := filepath.Rel(basePath, path)
	if err != nil {
		return path
	}

	// If the relative path escapes basePath (starts with ".."), use original
	if strings.HasPrefix(rel, "..") {
		return path
	}

	return rel
}

// ErrorStats provides statistics for AI prompt generation
type ErrorStats struct {
	ErrorCount   int                   `json:"error_count"`
	WarningCount int                   `json:"warning_count"`
	ByCategory   map[ErrorCategory]int `json:"by_category"`
	BySource     map[string]int        `json:"by_source"`
	UniqueFiles  int                   `json:"unique_files"`
	UniqueRules  int                   `json:"unique_rules"`
}

// ComprehensiveErrorGroup supports multiple grouping strategies for AI consumption
type ComprehensiveErrorGroup struct {
	ByFile     map[string][]*ExtractedError        `json:"by_file"`
	ByCategory map[ErrorCategory][]*ExtractedError `json:"by_category"`
	ByWorkflow map[string][]*ExtractedError        `json:"by_workflow"`
	NoFile     []*ExtractedError                   `json:"no_file"`
	Total      int                                 `json:"total"`
	Stats      ErrorStats                          `json:"stats"`
}

// GroupComprehensive creates comprehensive grouping with all strategies and statistics.
// It provides multi-dimensional error organization (by file, category, workflow) with
// detailed statistics, making it ideal for detailed reporting and analysis.
func GroupComprehensive(errs []*ExtractedError, basePath string) *ComprehensiveErrorGroup {
	// PASS 1: Count occurrences for pre-allocation
	fileCounts := make(map[string]int)
	categoryCounts := make(map[ErrorCategory]int)
	workflowCounts := make(map[string]int)
	sourceCounts := make(map[string]int)
	uniqueFiles := make(map[string]struct{})
	uniqueRules := make(map[string]struct{})

	for _, err := range errs {
		// Count files
		if err.File != "" {
			file := err.File
			if basePath != "" {
				file = makeRelative(file, basePath)
			}
			fileCounts[file]++
			uniqueFiles[file] = struct{}{}
		}

		// Count categories
		category := err.Category
		if category == "" {
			category = CategoryUnknown
		}
		categoryCounts[category]++

		// Count workflows
		workflowKey := "no-workflow"
		if err.WorkflowContext != nil && err.WorkflowContext.Job != "" {
			workflowKey = err.WorkflowContext.Job
		}
		workflowCounts[workflowKey]++

		// Count sources
		if err.Source != "" {
			sourceCounts[err.Source]++
		}

		// Count unique rules
		if err.RuleID != "" {
			uniqueRules[err.RuleID] = struct{}{}
		}
	}

	// PASS 2: Pre-allocate and populate
	grouped := &ComprehensiveErrorGroup{
		ByFile:     make(map[string][]*ExtractedError, len(fileCounts)),
		ByCategory: make(map[ErrorCategory][]*ExtractedError, len(categoryCounts)),
		ByWorkflow: make(map[string][]*ExtractedError, len(workflowCounts)),
		Total:      len(errs),
		Stats: ErrorStats{
			ByCategory: make(map[ErrorCategory]int, len(categoryCounts)),
			BySource:   make(map[string]int, len(sourceCounts)),
		},
	}

	// Pre-allocate slice capacities
	for file, count := range fileCounts {
		grouped.ByFile[file] = make([]*ExtractedError, 0, count)
	}
	for category, count := range categoryCounts {
		grouped.ByCategory[category] = make([]*ExtractedError, 0, count)
	}
	for workflow, count := range workflowCounts {
		grouped.ByWorkflow[workflow] = make([]*ExtractedError, 0, count)
	}

	// Now populate with pre-allocated slices
	for _, err := range errs {
		// Group by file
		if err.File != "" {
			file := err.File
			if basePath != "" {
				file = makeRelative(file, basePath)
			}
			grouped.ByFile[file] = append(grouped.ByFile[file], err)
		} else {
			grouped.NoFile = append(grouped.NoFile, err)
		}

		// Group by category
		category := err.Category
		if category == "" {
			category = CategoryUnknown
		}
		grouped.ByCategory[category] = append(grouped.ByCategory[category], err)

		// Group by workflow
		workflowKey := "no-workflow"
		if err.WorkflowContext != nil && err.WorkflowContext.Job != "" {
			workflowKey = err.WorkflowContext.Job
		}
		grouped.ByWorkflow[workflowKey] = append(grouped.ByWorkflow[workflowKey], err)

		// Collect statistics
		// Note: All errors should have severity set by the extractor via InferSeverity
		switch err.Severity {
		case "error":
			grouped.Stats.ErrorCount++
		case "warning":
			grouped.Stats.WarningCount++
		}

		grouped.Stats.ByCategory[category]++

		if err.Source != "" {
			grouped.Stats.BySource[err.Source]++
		}
	}

	grouped.Stats.UniqueFiles = len(uniqueFiles)
	grouped.Stats.UniqueRules = len(uniqueRules)

	return grouped
}
