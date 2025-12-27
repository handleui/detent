package prompt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/detent/cli/internal/errors"
)

// Default values for error formatting.
const (
	DefaultCategory    = "unknown"
	DefaultColumn      = 1
	MissingValue       = "-"
	DefaultPriority    = 999
	MaxStackTraceBytes = 50 * 1024
)

var categoryPriority = map[errors.ErrorCategory]int{
	errors.CategoryCompile:   1,
	errors.CategoryTypeCheck: 2,
	errors.CategoryTest:      3,
	errors.CategoryRuntime:   4,
	errors.CategoryLint:      5,
	errors.CategoryMetadata:  6,
	errors.CategoryUnknown:   7,
}

// Stack traces improve accuracy from 31% to 80-90% for these categories.
var categoriesNeedingStackTrace = map[errors.ErrorCategory]bool{
	errors.CategoryCompile: true,
	errors.CategoryTest:    true,
	errors.CategoryRuntime: true,
	errors.CategoryUnknown: true,
}

func escapePromptString(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "\r", "")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// FormatError formats a single ExtractedError with full diagnostic context.
func FormatError(err *errors.ExtractedError) string {
	if err == nil {
		return "(nil error)"
	}

	var parts []string

	category := string(err.Category)
	if category == "" {
		category = DefaultCategory
	}

	line := err.Line
	column := err.Column
	if column == 0 {
		column = DefaultColumn
	}

	file := escapePromptString(err.File)
	message := escapePromptString(err.Message)

	location := fmt.Sprintf("%s:%d:%d", file, line, column)
	if file == "" {
		location = fmt.Sprintf("line %d:%d", line, column)
	}
	parts = append(parts, fmt.Sprintf("[%s] %s: %s", category, location, message))

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

	if stackTrace := FormatStackTrace(err); stackTrace != "" {
		parts = append(parts, stackTrace)
	}

	return strings.Join(parts, "\n")
}

// FormatStackTrace formats and filters a stack trace.
func FormatStackTrace(err *errors.ExtractedError) string {
	if err == nil || err.StackTrace == "" {
		return ""
	}

	if len(err.StackTrace) > MaxStackTraceBytes {
		return "  Stack trace: (truncated - exceeds 50KB limit)"
	}

	if !categoriesNeedingStackTrace[err.Category] {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(err.StackTrace), "\n")
	filtered := filterStackTraceLines(lines)

	if len(filtered) == 0 {
		return ""
	}

	truncated := false
	originalLen := len(filtered)
	if len(filtered) > MaxStackTraceLines {
		filtered = filtered[:MaxStackTraceLines]
		truncated = true
	}

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

func filterStackTraceLines(lines []string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if !isInternalFrame(line) && strings.TrimSpace(line) != "" {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

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

// PrioritizeErrors sorts errors by category priority (compile first, then type-check, test, runtime, lint).
func PrioritizeErrors(errs []*errors.ExtractedError) []*errors.ExtractedError {
	if len(errs) == 0 {
		return errs
	}

	sorted := make([]*errors.ExtractedError, len(errs))
	copy(sorted, errs)

	sort.SliceStable(sorted, func(i, j int) bool {
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
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		return sorted[i].Line < sorted[j].Line
	})

	return sorted
}

func getPriority(cat errors.ErrorCategory) int {
	if p, ok := categoryPriority[cat]; ok {
		return p
	}
	return DefaultPriority
}

// CountErrors returns error and warning counts.
func CountErrors(errs []*errors.ExtractedError) (errorCount, warningCount int) {
	for _, err := range errs {
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
