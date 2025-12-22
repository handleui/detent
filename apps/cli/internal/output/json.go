package output

import (
	"encoding/json"
	"io"

	"github.com/detent/cli/internal/errors"
)

// FormatJSON formats error groups as JSON output.
// Returns error if JSON marshaling or writing fails.
func FormatJSON(w io.Writer, grouped *errors.GroupedErrors) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(grouped)
}
