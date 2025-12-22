package output

import (
	"encoding/json"
	"io"

	"github.com/detent/cli/internal/errors"
)

// FormatJSON formats error groups as JSON output using the simple GroupedErrors structure.
// Use this for basic error grouping by file path.
// Returns error if JSON marshaling or writing fails.
func FormatJSON(w io.Writer, grouped *errors.GroupedErrors) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(grouped)
}

// FormatJSONV2 formats error groups as JSON output using the comprehensive GroupedErrorsV2 structure.
// This includes multi-dimensional grouping (by file, category, workflow) and detailed statistics.
// Use this for AI consumption or advanced error analysis.
// Returns error if JSON marshaling or writing fails.
func FormatJSONV2(w io.Writer, grouped *errors.GroupedErrorsV2) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(grouped)
}
