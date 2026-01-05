package output

import (
	"encoding/json"
	"io"

	"github.com/detentsh/core/errors"
)

// FormatJSON formats error groups as JSON output using the simple GroupedErrors structure.
// Use this for basic error grouping by file path.
// Returns error if JSON marshaling or writing fails.
func FormatJSON(w io.Writer, grouped *errors.GroupedErrors) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(grouped)
}

// FormatJSONDetailed formats error groups as JSON output using the comprehensive ComprehensiveErrorGroup structure.
// This includes multi-dimensional grouping (by file, category, workflow) and detailed statistics.
// Use this for AI consumption or advanced error analysis.
// Returns error if JSON marshaling or writing fails.
func FormatJSONDetailed(w io.Writer, grouped *errors.ComprehensiveErrorGroup) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(grouped)
}
