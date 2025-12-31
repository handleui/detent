package parser

import (
	"regexp"
)

// ansiEscapePattern matches ANSI escape sequences for colored terminal output.
// Pattern: ESC[ followed by numeric parameters separated by semicolons, ending with 'm'.
// Examples: \x1b[0m (reset), \x1b[31m (red), \x1b[1;31;40m (bold red on black)
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes ANSI escape sequences from a string.
// This is used to clean up colored CLI output before parsing error patterns.
// CI tools like golangci-lint, cargo, tsc, and eslint may output colored text
// when running with --color or similar flags.
func StripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}
