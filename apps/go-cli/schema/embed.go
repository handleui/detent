package schema

import _ "embed"

// JSON contains the embedded JSON Schema for detent configuration files.
//
//go:embed detent.schema.json
var JSON string
