package errors

import (
	"path/filepath"
	"strings"
)

// ErrorCategory represents the type of error for categorization and AI prompt generation
type ErrorCategory string

const (
	CategoryLint      ErrorCategory = "lint"
	CategoryTypeCheck ErrorCategory = "type-check"
	CategoryTest      ErrorCategory = "test"
	CategoryCompile   ErrorCategory = "compile"
	CategoryRuntime   ErrorCategory = "runtime"
	CategoryUnknown   ErrorCategory = "unknown"
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
	RuleID          string           `json:"rule_id,omitempty"`          // e.g., "no-var", "TS2749"
	Category        ErrorCategory    `json:"category,omitempty"`         // lint, type-check, test, etc.
	WorkflowContext *WorkflowContext `json:"workflow_context,omitempty"` // Job/step info
	Source          string           `json:"source,omitempty"`           // "eslint", "typescript", "go", etc.
}

// GroupedErrors groups errors by file path for organized output
type GroupedErrors struct {
	ByFile map[string][]*ExtractedError `json:"by_file"`
	NoFile []*ExtractedError            `json:"no_file"`
	Total  int                          `json:"total"`
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

// makeRelative converts an absolute path to relative if it starts with basePath
func makeRelative(path, basePath string) string {
	if basePath == "" || !filepath.IsAbs(path) {
		return path
	}

	// Clean both paths
	path = filepath.Clean(path)
	basePath = filepath.Clean(basePath)

	// If path starts with basePath, make it relative
	if strings.HasPrefix(path, basePath) {
		rel := strings.TrimPrefix(path, basePath)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		if rel != "" {
			return rel
		}
	}

	return path
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

// GroupedErrorsV2 supports multiple grouping strategies for AI consumption
type GroupedErrorsV2 struct {
	ByFile     map[string][]*ExtractedError        `json:"by_file"`
	ByCategory map[ErrorCategory][]*ExtractedError `json:"by_category"`
	ByWorkflow map[string][]*ExtractedError        `json:"by_workflow"`
	NoFile     []*ExtractedError                   `json:"no_file"`
	Total      int                                 `json:"total"`
	Stats      ErrorStats                          `json:"stats"`
}

// GroupByCategory organizes errors by their category
func GroupByCategory(errs []*ExtractedError) map[ErrorCategory][]*ExtractedError {
	grouped := make(map[ErrorCategory][]*ExtractedError)
	for _, err := range errs {
		category := err.Category
		if category == "" {
			category = CategoryUnknown
		}
		grouped[category] = append(grouped[category], err)
	}
	return grouped
}

// GroupByWorkflow organizes errors by workflow context
func GroupByWorkflow(errs []*ExtractedError) map[string][]*ExtractedError {
	grouped := make(map[string][]*ExtractedError)
	for _, err := range errs {
		key := "no-workflow"
		if err.WorkflowContext != nil && err.WorkflowContext.Job != "" {
			key = err.WorkflowContext.Job
		}
		grouped[key] = append(grouped[key], err)
	}
	return grouped
}

// GroupForAI creates comprehensive grouping with all strategies and statistics
func GroupForAI(errs []*ExtractedError, basePath string) *GroupedErrorsV2 {
	grouped := &GroupedErrorsV2{
		ByFile:     make(map[string][]*ExtractedError),
		ByCategory: GroupByCategory(errs),
		ByWorkflow: GroupByWorkflow(errs),
		Total:      len(errs),
		Stats: ErrorStats{
			ByCategory: make(map[ErrorCategory]int),
			BySource:   make(map[string]int),
		},
	}

	uniqueFiles := make(map[string]struct{})
	uniqueRules := make(map[string]struct{})

	for _, err := range errs {
		// Group by file
		if err.File != "" {
			file := err.File
			if basePath != "" {
				file = makeRelative(file, basePath)
			}
			grouped.ByFile[file] = append(grouped.ByFile[file], err)
			uniqueFiles[file] = struct{}{}
		} else {
			grouped.NoFile = append(grouped.NoFile, err)
		}

		// Collect statistics
		if err.Severity == "warning" {
			grouped.Stats.WarningCount++
		} else {
			grouped.Stats.ErrorCount++
		}

		category := err.Category
		if category == "" {
			category = CategoryUnknown
		}
		grouped.Stats.ByCategory[category]++

		if err.Source != "" {
			grouped.Stats.BySource[err.Source]++
		}

		if err.RuleID != "" {
			uniqueRules[err.RuleID] = struct{}{}
		}
	}

	grouped.Stats.UniqueFiles = len(uniqueFiles)
	grouped.Stats.UniqueRules = len(uniqueRules)

	return grouped
}
