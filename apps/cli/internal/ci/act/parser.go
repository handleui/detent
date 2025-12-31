package act

import (
	"strings"

	"github.com/detent/cli/internal/ci"
)

// Parser implements ci.Parser for nektos/act output.
type Parser struct{}

// New creates a new act output parser.
func New() *Parser {
	return &Parser{}
}

// ParseLine parses a single line of act output for job events.
// Only matches job-level signals:
//   - 🚀 = job start
//   - 🏁 = job finish (success or failure)
//   - ⏭️ = job skipped (due to unmet needs or condition)
//
// Step-level signals (✅❌⭐) are ignored as they fire multiple times per job.
func (p *Parser) ParseLine(line string) (*ci.JobEvent, bool) {
	// Quick reject: must start with '[' and contain job-level emoji
	if line == "" || line[0] != '[' {
		return nil, false
	}

	// Only match job-level emojis
	hasStart := strings.Contains(line, "🚀")
	hasFinish := strings.Contains(line, "🏁")
	hasSkip := strings.Contains(line, "⏭️")

	if !hasStart && !hasFinish && !hasSkip {
		return nil, false
	}

	// Find the closing bracket before the emoji
	// Format: [Workflow/JobName] emoji ...
	closeBracket := -1
	for i := 1; i < len(line); i++ {
		if line[i] == ']' {
			rest := strings.TrimSpace(line[i+1:])
			if rest != "" && (strings.HasPrefix(rest, "🚀") || strings.HasPrefix(rest, "🏁") || strings.HasPrefix(rest, "⏭️")) {
				closeBracket = i
				break
			}
		}
	}

	if closeBracket < 2 {
		return nil, false
	}

	bracketContent := strings.TrimSpace(line[1:closeBracket])
	rest := strings.TrimSpace(line[closeBracket+1:])

	// Extract job name (after the /)
	// Format: "Workflow/JobName" or "CI/[CLI] Lint"
	jobName := bracketContent
	if idx := strings.Index(bracketContent, "/"); idx > 0 {
		jobName = strings.TrimSpace(bracketContent[idx+1:])
	}

	event := &ci.JobEvent{
		JobName: jobName,
	}

	switch {
	case strings.HasPrefix(rest, "🚀"):
		event.Action = "start"
	case strings.HasPrefix(rest, "🏁"):
		event.Action = "finish"
		// Detect success/failure from the message
		// "🏁  Job succeeded" or "🏁  Job failed"
		event.Success = strings.Contains(rest, "succeeded")
	case strings.HasPrefix(rest, "⏭️"):
		event.Action = "skip"
	default:
		return nil, false
	}

	return event, true
}
