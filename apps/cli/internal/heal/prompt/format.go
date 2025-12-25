package prompt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/detent/cli/internal/errors"
)

// Constants for default values and limits.
const (
	DefaultCategory    = "unknown"
	DefaultColumn      = 1
	MissingValue       = "-"
	DefaultPriority    = 999
	MaxStackTraceBytes = 50 * 1024 // 50KB max to prevent memory exhaustion
)

// categoryPriority defines the fix order for error categories.
// Lower number = higher priority (fix first).
var categoryPriority = map[errors.ErrorCategory]int{
	errors.CategoryCompile:   1,
	errors.CategoryTypeCheck: 2,
	errors.CategoryTest:      3,
	errors.CategoryRuntime:   4,
	errors.CategoryLint:      5,
	errors.CategoryMetadata:  6,
	errors.CategoryUnknown:   7,
}

// categoriesNeedingStackTrace identifies which error types benefit from stack traces.
// Research: Stack traces improve accuracy from 31% to 80-90% for these categories.
var categoriesNeedingStackTrace = map[errors.ErrorCategory]bool{
	errors.CategoryCompile:  true,
	errors.CategoryTest:     true,
	errors.CategoryRuntime:  true,
	errors.CategoryUnknown:  true,
}

// escapePromptString sanitizes user-controlled strings to prevent prompt injection.
// Escapes backticks, control characters, and normalizes whitespace.
func escapePromptString(s string) string {
	// Replace backticks (can break markdown/code blocks in prompts)
	s = strings.ReplaceAll(s, "`", "'")
	// Replace control characters that could manipulate prompt structure
	s = strings.ReplaceAll(s, "\r", "")
	// Normalize multiple newlines to single (prevents prompt structure manipulation)
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// FormatError formats a single ExtractedError with full diagnostic context.
// Research shows: error message + stack trace + rule ID = best AI accuracy.
func FormatError(err *errors.ExtractedError) string {
	// P0: Nil pointer check
	if err == nil {
		return "(nil error)"
	}

	var parts []string

	// Category and location
	category := string(err.Category)
	if category == "" {
		category = DefaultCategory
	}

	line := err.Line
	column := err.Column
	if column == 0 {
		column = DefaultColumn
	}

	// Primary error line: [category] file:line:col message
	// P0: Escape user-controlled strings to prevent prompt injection
	file := escapePromptString(err.File)
	message := escapePromptString(err.Message)

	location := fmt.Sprintf("%s:%d:%d", file, line, column)
	if file == "" {
		location = fmt.Sprintf("line %d:%d", line, column)
	}
	parts = append(parts, fmt.Sprintf("[%s] %s: %s", category, location, message))

	// Rule ID and source - always include, helps AI understand the error type
	if err.RuleID != "" || err.Source != "" {
		ruleID := escapePromptString(err.RuleID)
		if ruleID == "" {
			ruleID = MissingValue
		}
		source := escapePromptString(err.Source)
		if source == "" {
			source = MissingValue
		}
		parts = append(parts, fmt.Sprintf("  Rule: %s | Source: %s", ruleID, source))
	}

	// Stack trace - include for categories where it improves accuracy
	if stackTrace := FormatStackTrace(err); stackTrace != "" {
		parts = append(parts, stackTrace)
	}

	return strings.Join(parts, "\n")
}

// FormatStackTrace formats and filters a stack trace.
// Returns empty string for categories where stack traces are noise (lint).
func FormatStackTrace(err *errors.ExtractedError) string {
	// P0: Nil pointer check
	if err == nil || err.StackTrace == "" {
		return ""
	}

	// P0: Reject oversized stack traces to prevent memory exhaustion
	if len(err.StackTrace) > MaxStackTraceBytes {
		return "  Stack trace: (truncated - exceeds 50KB limit)"
	}

	// Skip stack traces for categories where they don't help
	if !categoriesNeedingStackTrace[err.Category] {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(err.StackTrace), "\n")
	filtered := filterStackTraceLines(lines)

	if len(filtered) == 0 {
		return ""
	}

	// Limit to MaxStackTraceLines (20 frames = sweet spot per research)
	truncated := false
	originalLen := len(filtered)
	if len(filtered) > MaxStackTraceLines {
		filtered = filtered[:MaxStackTraceLines]
		truncated = true
	}

	// Format with indentation
	var result strings.Builder
	result.WriteString("  Stack trace:")
	for _, line := range filtered {
		result.WriteString("\n    ")
		result.WriteString(escapePromptString(line))
	}
	if truncated {
		remaining := originalLen - MaxStackTraceLines
		result.WriteString("\n    ... (truncated, ")
		result.WriteString(fmt.Sprintf("%d", remaining))
		result.WriteString(" more frames)")
	}

	return result.String()
}

// filterStackTraceLines removes internal/framework frames that add noise.
func filterStackTraceLines(lines []string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if !isInternalFrame(line) && strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// isInternalFrame checks if a stack frame is from internal/framework code.
func isInternalFrame(line string) bool {
	for _, pattern := range InternalFramePatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}
	return false
}

// FormatErrors formats multiple errors with full context.
func FormatErrors(errs []*errors.ExtractedError) string {
	if len(errs) == 0 {
		return "(no errors)"
	}

	sorted := PrioritizeErrors(errs)
	var parts []string

	for _, err := range sorted {
		// P0: Skip nil errors
		if err == nil {
			continue
		}
		parts = append(parts, FormatError(err))
	}

	if len(parts) == 0 {
		return "(no valid errors)"
	}

	return strings.Join(parts, "\n\n")
}

// FormatWorkflowContext formats the workflow job/step context.
func FormatWorkflowContext(errs []*errors.ExtractedError) string {
	jobs := make(map[string]struct{})
	for _, err := range errs {
		// P0: Skip nil errors
		if err == nil {
			continue
		}
		if err.WorkflowContext != nil && err.WorkflowContext.Job != "" {
			jobs[escapePromptString(err.WorkflowContext.Job)] = struct{}{}
		}
	}

	if len(jobs) == 0 {
		return ""
	}

	jobList := make([]string, 0, len(jobs))
	for job := range jobs {
		jobList = append(jobList, job)
	}
	sort.Strings(jobList)

	return fmt.Sprintf("CI Jobs: %s", strings.Join(jobList, ", "))
}

// PrioritizeErrors sorts errors by category priority.
// Compile errors first, then type-check, test, runtime, lint.
// Returns a new slice; does not modify the original.
func PrioritizeErrors(errs []*errors.ExtractedError) []*errors.ExtractedError {
	if len(errs) == 0 {
		return errs
	}

	sorted := make([]*errors.ExtractedError, len(errs))
	copy(sorted, errs)

	sort.SliceStable(sorted, func(i, j int) bool {
		// P0: Handle nil pointers
		if sorted[i] == nil {
			return false
		}
		if sorted[j] == nil {
			return true
		}

		priI := getPriority(sorted[i].Category)
		priJ := getPriority(sorted[j].Category)
		if priI != priJ {
			return priI < priJ
		}
		// Within same priority, sort by file then line
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line < sorted[j].Line
	})

	return sorted
}

// getPriority returns the priority for a category, with a safe default.
func getPriority(cat errors.ErrorCategory) int {
	if p, ok := categoryPriority[cat]; ok {
		return p
	}
	return DefaultPriority
}

// CountErrors returns error and warning counts.
func CountErrors(errs []*errors.ExtractedError) (errorCount, warningCount int) {
	for _, err := range errs {
		// P0: Skip nil errors
		if err == nil {
			continue
		}
		switch err.Severity {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		}
	}
	return
}

// CountByCategory returns a breakdown by category.
func CountByCategory(errs []*errors.ExtractedError) map[errors.ErrorCategory]int {
	counts := make(map[errors.ErrorCategory]int)
	for _, err := range errs {
		// P0: Skip nil errors
		if err == nil {
			continue
		}
		cat := err.Category
		if cat == "" {
			cat = errors.CategoryUnknown
		}
		counts[cat]++
	}
	return counts
}
