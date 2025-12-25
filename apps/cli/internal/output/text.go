package output

import (
	"fmt"
	"io"
	"sort"
	"strings"

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

// dividerWidth is the standard width for section dividers
const dividerWidth = 60

// divider returns a horizontal line of the specified width
func divider(width int) string {
	return strings.Repeat("─", width)
}

// FormatText formats error groups as human-readable text output.
// It displays errors grouped by file with colored severity indicators
// and summary statistics at the end.
func FormatText(w io.Writer, grouped *errors.GroupedErrors) {
	// Count errors and warnings
	errorCount, warningCount := countSeverities(grouped)

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

	if len(grouped.ByFile) > 0 {
		files := make([]string, 0, len(grouped.ByFile))
		for f := range grouped.ByFile {
			files = append(files, f)
		}
		sort.Strings(files)

		for _, file := range files {
			errs := grouped.ByFile[file]
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

	if len(grouped.NoFile) > 0 {
		_, _ = fmt.Fprintf(w, "%s%sOther Issues:%s\n\n", colorBold, colorYellow, colorReset)
		for _, err := range grouped.NoFile {
			severity := getSeverityColor(err.Severity)
			_, _ = fmt.Fprintf(w, "  %s●%s %s\n", severity, colorReset, err.Message)
		}
		_, _ = fmt.Fprintln(w)
	}

	if grouped.Total == 0 {
		_, _ = fmt.Fprintf(w, "%s> %s✓ No problems found%s\n", colorBold, colorGreen, colorReset)
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

// countSeverities counts errors and warnings in grouped errors
func countSeverities(grouped *errors.GroupedErrors) (errorCount, warningCount int) {
	for _, errs := range grouped.ByFile {
		errorCount += countBySeverity(errs, "error")
		warningCount += countBySeverity(errs, "warning")
	}
	errorCount += countBySeverity(grouped.NoFile, "error")
	warningCount += countBySeverity(grouped.NoFile, "warning")
	return errorCount, warningCount
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
