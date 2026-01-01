package act

import (
	"regexp"
	"strings"

	"github.com/handleui/detent/packages/core/ci"
)

// Regex patterns for act context parsing.
var (
	// actContextPattern matches the [workflow/job] prefix in act output.
	// Example: "[CI/build]" -> captures "CI/build"
	actContextPattern = regexp.MustCompile(`^\[([^\]]+)\]`)

	// actDebugStructPattern matches act's verbose debug output that contains
	// Go struct dumps with <nil> values (e.g., "Job.Strategy: <nil>").
	actDebugStructPattern = regexp.MustCompile(`^\s*(?:Job\.|level=debug|time=).*<nil>`)
)

// isActDebugNoise returns true if the line is act debug noise that should be skipped.
// This filters out Go struct dumps and debug messages that contain <nil> values,
// which aren't actual errors but internal act state dumps.
func isActDebugNoise(line string) bool {
	// Fast path: exact match without trimming
	if line == "<nil>" {
		return true
	}
	// Only trim if potentially matching (contains <nil>)
	if strings.Contains(line, "<nil>") {
		if strings.TrimSpace(line) == "<nil>" {
			return true
		}
	}
	if actDebugStructPattern.MatchString(line) {
		return true
	}
	return false
}

// ContextParser parses context information from act output lines.
// It extracts the [workflow/job] prefix and filters debug noise.
type ContextParser struct{}

// NewContextParser creates a new act context parser.
func NewContextParser() *ContextParser {
	return &ContextParser{}
}

// ParseLine parses a line of act output to extract context information.
// It handles the act output format: [Workflow/JobName] | tool output here
//
// Returns:
//   - context: Contains the job name if a [workflow/job] prefix was found
//   - cleanedLine: The line with the prefix and pipe separator removed
//   - skip: True if the line is debug noise and should be skipped
func (p *ContextParser) ParseLine(line string) (context *ci.LineContext, cleanedLine string, skip bool) {
	// Check for debug noise first
	if isActDebugNoise(line) {
		return &ci.LineContext{IsNoise: true}, "", true
	}

	// Try to extract [workflow/job] context
	if match := actContextPattern.FindStringSubmatchIndex(line); match != nil {
		context = &ci.LineContext{
			Job: line[match[2]:match[3]], // Extract the captured group
		}

		// Extract the rest of the line after the bracket
		if match[1] < len(line) {
			rest := line[match[1]:]
			// Skip whitespace before pipe: "[CI/build]   | message" -> "message"
			rest = strings.TrimLeft(rest, " \t")
			if rest != "" && rest[0] == '|' {
				rest = rest[1:]
			}
			cleanedLine = strings.TrimSpace(rest)
			return context, cleanedLine, false
		}
	}

	return nil, line, false
}

// Ensure ContextParser implements ci.ContextParser
var _ ci.ContextParser = (*ContextParser)(nil)
