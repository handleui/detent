package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/detent/cli/internal/errors"
)

// ANSI color codes - using ANSI 256 palette to match TUI styles (internal/tui/styles.go)
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[38;5;203m"   // ColorError from styles.go
	colorYellow = "\033[38;5;214m"   // ColorWarning from styles.go
	colorGreen  = "\033[38;5;42m"    // ColorSuccess from styles.go
	colorCyan   = "\033[38;5;45m"    // ColorAccent from styles.go
	colorGray   = "\033[38;5;240m"   // ColorMuted from styles.go
	colorBold   = "\033[1m"
)

// dividerWidth is the standard width for section dividers
const dividerWidth = 60

// categoryOrder defines the display order for error categories
var categoryOrder = []errors.ErrorCategory{
	errors.CategoryLint,
	errors.CategoryTypeCheck,
	errors.CategoryTest,
	errors.CategoryCompile,
	errors.CategoryRuntime,
	errors.CategoryMetadata,
	errors.CategoryUnknown,
}

// categoryNames maps error categories to human-readable section headers
var categoryNames = map[errors.ErrorCategory]string{
	errors.CategoryLint:      "Lint Issues",
	errors.CategoryTypeCheck: "Type Errors",
	errors.CategoryTest:      "Test Failures",
	errors.CategoryCompile:   "Build Errors",
	errors.CategoryRuntime:   "Runtime Errors",
	errors.CategoryMetadata:  "Metadata Issues",
	errors.CategoryUnknown:   "Other Issues",
}

// divider returns a horizontal line of the specified width
func divider(width int) string {
	return strings.Repeat("─", width)
}

// FormatText formats error groups as human-readable text output.
// It displays errors grouped by category (Lint, TypeCheck, Test, etc.)
// with file grouping within each category, colored severity indicators,
// and summary statistics at the end.
func FormatText(w io.Writer, grouped *errors.ComprehensiveErrorGroup) {
	errorCount := grouped.Stats.ErrorCount
	warningCount := grouped.Stats.WarningCount

	_, _ = fmt.Fprintln(w)

	if errorCount > 0 || warningCount > 0 {
		totalProblems := errorCount + warningCount
		problemText := fmt.Sprintf("Found %d problem%s", totalProblems, plural(totalProblems))
		detailText := fmt.Sprintf("(%s%d error%s, %d warning%s)%s",
			colorRed, errorCount, plural(errorCount), warningCount, plural(warningCount), colorReset)

		// Show aggregate stats: file count
		fileCount := len(grouped.ByFile)
		if fileCount > 0 {
			_, _ = fmt.Fprintf(w, "%s> %s✖ %s %s across %d file%s%s\n",
				colorBold, colorRed, problemText, detailText, fileCount, plural(fileCount), colorReset)
		} else {
			_, _ = fmt.Fprintf(w, "%s> %s✖ %s %s%s\n", colorBold, colorRed, problemText, detailText, colorReset)
		}
		_, _ = fmt.Fprintf(w, "%s  Run 'detent heal' to auto-fix or fix manually and re-run%s\n\n", colorGray, colorReset)
		_, _ = fmt.Fprintf(w, "%s%s%s\n\n", colorGray, divider(dividerWidth), colorReset)
	}

	// Display errors grouped by category
	for _, category := range categoryOrder {
		categoryErrors := grouped.ByCategory[category]
		if len(categoryErrors) == 0 {
			continue
		}

		// Print category header
		categoryName := categoryNames[category]
		categoryColor := getCategoryColor(category)
		_, _ = fmt.Fprintf(w, "%s%s%s:%s\n\n", colorBold, categoryColor, categoryName, colorReset)

		// Group errors within this category by file
		byFile := make(map[string][]*errors.ExtractedError)
		var noFile []*errors.ExtractedError

		for _, err := range categoryErrors {
			if err.File != "" {
				byFile[err.File] = append(byFile[err.File], err)
			} else {
				noFile = append(noFile, err)
			}
		}

		// Display errors with file location
		if len(byFile) > 0 {
			files := make([]string, 0, len(byFile))
			for f := range byFile {
				files = append(files, f)
			}
			sort.Strings(files)

			for _, file := range files {
				errs := byFile[file]
				fileErrors := countBySeverity(errs, "error")
				fileWarnings := countBySeverity(errs, "warning")

				_, _ = fmt.Fprintf(w, "%s%s%s%s ", colorBold, colorCyan, file, colorReset)
				_, _ = fmt.Fprintf(w, "%s(%d error%s, %d warning%s)%s\n",
					colorGray, fileErrors, plural(fileErrors), fileWarnings, plural(fileWarnings), colorReset)

				for _, err := range errs {
					formatError(w, err)
				}
				_, _ = fmt.Fprintln(w)
			}
		}

		// Display errors without file location
		for _, err := range noFile {
			severity := getSeverityColor(err.Severity)
			message := err.Message
			if err.RuleID != "" {
				message = fmt.Sprintf("%s [%s]", err.Message, err.RuleID)
			}
			_, _ = fmt.Fprintf(w, "  %s●%s %s\n", severity, colorReset, message)
		}
		if len(noFile) > 0 {
			_, _ = fmt.Fprintln(w)
		}
	}

	if grouped.Total == 0 {
		_, _ = fmt.Fprintf(w, "%s> %s✓ No problems found%s\n", colorBold, colorGreen, colorReset)
	}
}

// getCategoryColor returns the appropriate color for a category
func getCategoryColor(category errors.ErrorCategory) string {
	switch category {
	case errors.CategoryLint:
		return colorYellow
	case errors.CategoryTypeCheck:
		return colorRed
	case errors.CategoryTest:
		return colorRed
	case errors.CategoryCompile:
		return colorRed
	case errors.CategoryRuntime:
		return colorRed
	default:
		return colorYellow
	}
}

// formatError writes a single error with location to w.
func formatError(w io.Writer, err *errors.ExtractedError) {
	severity := getSeverityColor(err.Severity)
	location := formatLocation(err)

	message := err.Message
	if err.RuleID != "" {
		message = fmt.Sprintf("%s [%s]", err.Message, err.RuleID)
	}

	_, _ = fmt.Fprintf(w, "  %s%s:%s%s %s%s%s %s\n",
		colorGray, location, colorReset,
		severity, getSeveritySymbol(err.Severity), colorReset,
		colorGray, message)
}

// formatLocation formats the line:column part
func formatLocation(err *errors.ExtractedError) string {
	switch {
	case err.Line > 0 && err.Column > 0:
		return fmt.Sprintf("%d:%d", err.Line, err.Column)
	case err.Line > 0:
		return fmt.Sprintf("%d", err.Line)
	default:
		return ""
	}
}

// getSeverityColor returns the color for a severity level
func getSeverityColor(severity string) string {
	switch severity {
	case "error":
		return colorRed
	case "warning":
		return colorYellow
	default:
		return colorGray
	}
}

// getSeveritySymbol returns a symbol for a severity level
func getSeveritySymbol(severity string) string {
	switch severity {
	case "error":
		return "✖"
	case "warning":
		return "⚠"
	default:
		return "●"
	}
}

// plural returns "s" if count != 1
func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// countBySeverity counts errors by severity level in a list
func countBySeverity(errs []*errors.ExtractedError, severity string) int {
	count := 0
	for _, err := range errs {
		if err.Severity == severity {
			count++
		}
	}
	return count
}
