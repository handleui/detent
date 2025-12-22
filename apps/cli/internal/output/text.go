package output

import (
	"fmt"
	"io"
	"sort"

	"github.com/detent/cli/internal/errors"
)

// FormatText writes human-readable error output to w.
func FormatText(w io.Writer, grouped *errors.GroupedErrors) {
	_, _ = fmt.Fprintf(w, "\n=== Detent Error Report ===\n\n")
	_, _ = fmt.Fprintf(w, "Total errors: %d\n\n", grouped.Total)

	if len(grouped.ByFile) > 0 {
		_, _ = fmt.Fprintf(w, "--- Errors by File ---\n\n")

		files := make([]string, 0, len(grouped.ByFile))
		for f := range grouped.ByFile {
			files = append(files, f)
		}
		sort.Strings(files)

		for _, file := range files {
			errs := grouped.ByFile[file]
			_, _ = fmt.Fprintf(w, "%s (%d errors)\n", file, len(errs))

			for _, err := range errs {
				formatError(w, file, err)
			}
			_, _ = fmt.Fprintln(w)
		}
	}

	if len(grouped.NoFile) > 0 {
		_, _ = fmt.Fprintf(w, "--- Other Errors ---\n\n")
		for _, err := range grouped.NoFile {
			_, _ = fmt.Fprintf(w, "  %s\n", err.Message)
		}
		_, _ = fmt.Fprintln(w)
	}

	if grouped.Total == 0 {
		_, _ = fmt.Fprintf(w, "No errors found.\n")
	}
}

// formatError writes a single error with location to w.
// Avoids string concatenation by using conditional format strings.
func formatError(w io.Writer, file string, err *errors.ExtractedError) {
	switch {
	case err.Line > 0 && err.Column > 0:
		_, _ = fmt.Fprintf(w, "  %s:%d:%d: %s\n", file, err.Line, err.Column, err.Message)
	case err.Line > 0:
		_, _ = fmt.Fprintf(w, "  %s:%d: %s\n", file, err.Line, err.Message)
	default:
		_, _ = fmt.Fprintf(w, "  %s: %s\n", file, err.Message)
	}
}
