package tui

import (
	"regexp"
	"strings"
)

var (
	// Pre-compiled regex for ParseActProgress
	// ONLY matches act step progress with emoji indicators, NOT debug logs
	// Matches: [job-name] 🚀  Start image...
	// Matches: [job/step] ⭐  Run Main Install dependencies
	// Does NOT match: [DEBUG] Found revision
	// Does NOT match: [Release/Release] : [DEBUG] ...
	jobStepPattern = regexp.MustCompile(`^\[([^\]]+)\]\s+([🚀✅❌⭐].*)`)
)

// ParseActProgress extracts progress information from act output
// Only matches lines with emoji indicators (🚀✅❌⭐), filtering out debug logs
func ParseActProgress(line string) *ProgressMsg {
	matches := jobStepPattern.FindStringSubmatch(line)

	if len(matches) >= 3 {
		bracketContent := matches[1]
		stepInfo := strings.TrimSpace(matches[2])

		// Extract job ID from bracket content
		// Act outputs [job/step] format - we want just the job part
		jobID := bracketContent
		if idx := strings.Index(bracketContent, "/"); idx > 0 {
			jobID = bracketContent[:idx]
		}

		// Clean up step info (remove emoji prefixes)
		stepInfo = strings.TrimPrefix(stepInfo, "🚀  ")
		stepInfo = strings.TrimPrefix(stepInfo, "✅  ")
		stepInfo = strings.TrimPrefix(stepInfo, "❌  ")
		stepInfo = strings.TrimPrefix(stepInfo, "⭐  ")

		return &ProgressMsg{
			Status: stepInfo,
			JobID:  jobID,
		}
	}

	return nil
}
