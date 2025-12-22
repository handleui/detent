package output

import (
	"fmt"
	"io"
	"sort"

	"github.com/detent/cli/internal/errors"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

// FormatText writes human-readable error output to w.
func FormatText(w io.Writer, grouped *errors.GroupedErrors) {
	// Count errors and warnings
	errorCount, warningCount := countSeverities(grouped)

	_, _ = fmt.Fprintf(w, "\n%s%s=== Detent Error Report ===%s\n\n", colorBold, colorCyan, colorReset)

	if errorCount > 0 || warningCount > 0 {
		_, _ = fmt.Fprintf(w, "  %s%d error%s%s", colorRed, errorCount, plural(errorCount), colorReset)
		_, _ = fmt.Fprintf(w, "  %s%d warning%s%s\n\n", colorYellow, warningCount, plural(warningCount), colorReset)
	}

	if len(grouped.ByFile) > 0 {
		files := make([]string, 0, len(grouped.ByFile))
		for f := range grouped.ByFile {
			files = append(files, f)
		}
		sort.Strings(files)

		for _, file := range files {
			errs := grouped.ByFile[file]
			fileErrors := countErrorsInList(errs)
			fileWarnings := countWarningsInList(errs)

			_, _ = fmt.Fprintf(w, "%s%s%s%s ", colorBold, colorCyan, file, colorReset)
			_, _ = fmt.Fprintf(w, "%s(%d error%s, %d warning%s)%s\n",
				colorGray, fileErrors, plural(fileErrors), fileWarnings, plural(fileWarnings), colorReset)

			for _, err := range errs {
				formatError(w, file, err)
			}
			_, _ = fmt.Fprintln(w)
		}
	}

	if len(grouped.NoFile) > 0 {
		_, _ = fmt.Fprintf(w, "%s%sOther Issues:%s\n\n", colorBold, colorYellow, colorReset)
		for _, err := range grouped.NoFile {
			severity := getSeverityColor(err.Severity)
			_, _ = fmt.Fprintf(w, "  %s●%s %s\n", severity, colorReset, err.Message)
		}
		_, _ = fmt.Fprintln(w)
	}

	if grouped.Total == 0 {
		_, _ = fmt.Fprintf(w, "%s✓ No errors found.%s\n", colorGreen, colorReset)
	}
}

// formatError writes a single error with location to w.
func formatError(w io.Writer, file string, err *errors.ExtractedError) {
	severity := getSeverityColor(err.Severity)
	location := formatLocation(err)

	_, _ = fmt.Fprintf(w, "  %s%s:%s%s %s%s%s %s\n",
		colorGray, location, colorReset,
		severity, getSeveritySymbol(err.Severity), colorReset,
		colorGray, err.Message)
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

// countSeverities counts errors and warnings in grouped errors
func countSeverities(grouped *errors.GroupedErrors) (errorCount, warningCount int) {
	for _, errs := range grouped.ByFile {
		errorCount += countErrorsInList(errs)
		warningCount += countWarningsInList(errs)
	}
	errorCount += countErrorsInList(grouped.NoFile)
	warningCount += countWarningsInList(grouped.NoFile)
	return errorCount, warningCount
}

// countErrorsInList counts errors in a list
func countErrorsInList(errs []*errors.ExtractedError) int {
	count := 0
	for _, err := range errs {
		if err.Severity == "error" || err.Severity == "" {
			count++
		}
	}
	return count
}

// countWarningsInList counts warnings in a list
func countWarningsInList(errs []*errors.ExtractedError) int {
	count := 0
	for _, err := range errs {
		if err.Severity == "warning" {
			count++
		}
	}
	return count
}
