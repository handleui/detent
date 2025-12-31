package act

import (
	"regexp"
	"strings"

	"github.com/detent/cli/internal/ci"
)

const detentMarkerPrefix = "::detent::"

// validJobIDPattern matches GitHub Actions job ID requirements.
// Used to validate job IDs from markers to prevent injection attacks.
var validJobIDPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)

// Parser implements ci.Parser for nektos/act output.
// It parses both detent markers (reliable, injected) and emoji signals (fallback).
type Parser struct {
	// manifestSeen tracks whether we've seen a manifest marker
	manifestSeen bool
	// expectedJobs contains job IDs from the manifest for validation
	expectedJobs map[string]bool
}

// New creates a new act output parser.
func New() *Parser {
	return &Parser{
		expectedJobs: make(map[string]bool),
	}
}

// ParseLine parses a single line of act output for job events.
// It prioritizes detent markers (::detent::job-start::, ::detent::job-end::)
// and falls back to emoji signals for reusable workflows.
func (p *Parser) ParseLine(line string) (*ci.JobEvent, bool) {
	// Primary: check for detent markers (reliable, version-agnostic)
	if idx := strings.Index(line, detentMarkerPrefix); idx >= 0 {
		return p.parseDetentMarker(line[idx:])
	}

	// Fallback: emoji parsing for reusable workflows or older act versions
	return p.parseEmojiSignals(line)
}

// parseDetentMarker parses detent markers injected by InjectJobMarkers.
// Formats:
//   - ::detent::manifest::job1,job2,job3
//   - ::detent::job-start::job-id
//   - ::detent::job-end::job-id::success|failure|cancelled
func (p *Parser) parseDetentMarker(marker string) (*ci.JobEvent, bool) {
	// Strip prefix and split by ::
	content := strings.TrimPrefix(marker, detentMarkerPrefix)
	parts := strings.Split(content, "::")

	if len(parts) < 2 {
		return nil, false
	}

	switch parts[0] {
	case "manifest":
		// Parse manifest and store expected jobs (only first manifest matters)
		if !p.manifestSeen {
			p.manifestSeen = true
			jobIDs := strings.Split(parts[1], ",")
			for _, id := range jobIDs {
				id = strings.TrimSpace(id)
				// Only store valid job IDs to prevent injection
				if id != "" && isValidJobID(id) {
					p.expectedJobs[id] = true
				}
			}
		}
		// Manifest is informational only, not a job event
		return nil, false

	case "job-start":
		if len(parts) < 2 {
			return nil, false
		}
		jobID := strings.TrimSpace(parts[1])
		// Validate job ID to prevent malformed markers from causing issues
		if jobID == "" || !isValidJobID(jobID) {
			return nil, false
		}
		return &ci.JobEvent{
			JobName: jobID,
			Action:  "start",
		}, true

	case "job-end":
		if len(parts) < 2 {
			return nil, false
		}
		jobID := strings.TrimSpace(parts[1])
		// Validate job ID
		if jobID == "" || !isValidJobID(jobID) {
			return nil, false
		}

		// Handle missing or invalid status gracefully - default to failure
		// Valid statuses: success, failure, cancelled
		status := ""
		if len(parts) >= 3 {
			status = strings.TrimSpace(parts[2])
		}

		return &ci.JobEvent{
			JobName: jobID,
			Action:  "finish",
			Success: status == "success",
		}, true
	}

	return nil, false
}

// isValidJobID checks if a job ID matches GitHub Actions requirements.
func isValidJobID(jobID string) bool {
	return validJobIDPattern.MatchString(jobID)
}

// parseEmojiSignals parses legacy emoji-based job signals from act output.
// This is the fallback for reusable workflows that don't have injected markers.
// Signals:
//   - 🚀 = job start
//   - 🏁 = job finish (success or failure)
//   - ⏭️ = job skipped
func (p *Parser) parseEmojiSignals(line string) (*ci.JobEvent, bool) {
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

// ExpectedJobs returns the job IDs from the manifest, if one was parsed.
// Returns nil if no manifest has been seen yet.
func (p *Parser) ExpectedJobs() []string {
	if !p.manifestSeen || len(p.expectedJobs) == 0 {
		return nil
	}
	jobs := make([]string, 0, len(p.expectedJobs))
	for job := range p.expectedJobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// HasManifest returns true if a manifest marker has been parsed.
func (p *Parser) HasManifest() bool {
	return p.manifestSeen
}
