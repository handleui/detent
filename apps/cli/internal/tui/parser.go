package tui

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Pre-compiled regex for ParseActProgress
	jobStepPattern = regexp.MustCompile(`^\[([^\]]+)\]\s+(.+)`)
)

// ParseActProgress extracts progress information from act output
func ParseActProgress(line string) *ProgressMsg {
	// Pattern: [Job Name/Step Name] or similar
	// act outputs lines like: "[job-name] 🚀  Start image..."
	// "[job-name]   ✅  Success - Step Name"
	// We'll parse these to extract current step info

	// Match job/step patterns using pre-compiled regex
	matches := jobStepPattern.FindStringSubmatch(line)

	if len(matches) >= 3 {
		jobName := matches[1]
		stepInfo := strings.TrimSpace(matches[2])

		// Clean up step info
		stepInfo = strings.TrimPrefix(stepInfo, "🚀  ")
		stepInfo = strings.TrimPrefix(stepInfo, "✅  ")
		stepInfo = strings.TrimPrefix(stepInfo, "❌  ")

		status := fmt.Sprintf("%s: %s", jobName, stepInfo)

		return &ProgressMsg{
			Status:      status,
			CurrentStep: 0, // TODO: Parse step numbers from act output when format is stable
			TotalSteps:  0, // TODO: Count total steps from workflow definition
		}
	}

	return nil
}
