package output

import (
	"encoding/json"
	"io"

	"github.com/detent/cli/internal/errors"
)

// FormatJSON writes JSON error output
func FormatJSON(w io.Writer, grouped *errors.GroupedErrors) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(grouped)
}
