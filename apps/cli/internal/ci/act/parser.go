package act

import (
	"encoding/json"
	"regexp"
	"strconv"
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
	// manifest stores the parsed v2 manifest
	manifest *ci.ManifestInfo
	// expectedJobs contains job IDs from the manifest for validation (v1 compat)
	expectedJobs map[string]bool
}

// New creates a new act output parser.
func New() *Parser {
	return &Parser{
		expectedJobs: make(map[string]bool),
	}
}

// ParseLine parses a single line of act output for job, step, or manifest events.
// It prioritizes detent markers and falls back to emoji signals for reusable workflows.
// Returns the event (JobEvent, StepEvent, or ManifestEvent) and whether an event was found.
func (p *Parser) ParseLine(line string) (any, bool) {
	// Primary: check for detent markers (reliable, version-agnostic)
	if idx := strings.Index(line, detentMarkerPrefix); idx >= 0 {
		return p.parseDetentMarker(line[idx:])
	}

	// Fallback: emoji parsing for reusable workflows or older act versions
	return p.parseEmojiSignals(line)
}

// ParseLineJobEvent is a convenience method that only returns JobEvent.
// Used for backward compatibility with code that only cares about job events.
func (p *Parser) ParseLineJobEvent(line string) (*ci.JobEvent, bool) {
	event, ok := p.ParseLine(line)
	if !ok {
		return nil, false
	}
	if jobEvent, isJob := event.(*ci.JobEvent); isJob {
		return jobEvent, true
	}
	return nil, false
}

// parseDetentMarker parses detent markers injected by InjectJobMarkers.
// Formats:
//   - ::detent::manifest::v2::{json}        (v2 manifest with full job/step info)
//   - ::detent::manifest::job1,job2,job3    (v1 manifest, job IDs only)
//   - ::detent::job-start::job-id
//   - ::detent::job-end::job-id::success|failure|cancelled
//   - ::detent::step-start::job-id::step-index::step-name
func (p *Parser) parseDetentMarker(marker string) (any, bool) {
	// Strip prefix and split by ::
	content := strings.TrimPrefix(marker, detentMarkerPrefix)
	parts := strings.SplitN(content, "::", 3)

	if len(parts) < 1 {
		return nil, false
	}

	switch parts[0] {
	case "manifest":
		return p.parseManifest(parts[1:])

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
			JobID:  jobID,
			Action: "start",
		}, true

	case "job-end":
		if len(parts) < 2 {
			return nil, false
		}
		// parts[1] may contain "job-id::status", need to split further
		endParts := strings.SplitN(parts[1], "::", 2)
		jobID := strings.TrimSpace(endParts[0])
		// Validate job ID
		if jobID == "" || !isValidJobID(jobID) {
			return nil, false
		}

		// Handle missing or invalid status gracefully - default to failure
		// Valid statuses: success, failure, cancelled
		status := ""
		if len(endParts) >= 2 {
			status = strings.TrimSpace(endParts[1])
		} else if len(parts) >= 3 {
			status = strings.TrimSpace(parts[2])
		}

		return &ci.JobEvent{
			JobID:   jobID,
			Action:  "finish",
			Success: status == "success",
		}, true

	case "step-start":
		return p.parseStepStart(parts[1:])
	}

	return nil, false
}

// parseManifest parses v1 or v2 manifest format.
func (p *Parser) parseManifest(parts []string) (any, bool) {
	// Only first manifest matters
	if p.manifestSeen {
		return nil, false
	}

	if len(parts) < 1 {
		return nil, false
	}

	// Check for v2 format: "v2::{json}"
	if parts[0] == "v2" && len(parts) >= 2 {
		return p.parseManifestV2(parts[1])
	}

	// v1 format: "job1,job2,job3"
	return p.parseManifestV1(parts[0])
}

// parseManifestV1 parses the legacy v1 manifest (comma-separated job IDs).
func (p *Parser) parseManifestV1(content string) (any, bool) {
	p.manifestSeen = true
	jobIDs := strings.Split(content, ",")
	var validIDs []string
	for _, id := range jobIDs {
		id = strings.TrimSpace(id)
		// Only store valid job IDs to prevent injection
		if id != "" && isValidJobID(id) {
			p.expectedJobs[id] = true
			validIDs = append(validIDs, id)
		}
	}

	// Create a basic manifest for v1 compatibility
	jobs := make([]ci.ManifestJob, 0, len(validIDs))
	for _, id := range validIDs {
		jobs = append(jobs, ci.ManifestJob{
			ID:   id,
			Name: id,
		})
	}

	p.manifest = &ci.ManifestInfo{
		Version: 1,
		Jobs:    jobs,
	}

	return &ci.ManifestEvent{Manifest: p.manifest}, true
}

// parseManifestV2 parses the v2 JSON manifest.
func (p *Parser) parseManifestV2(jsonContent string) (any, bool) {
	var manifest ci.ManifestInfo
	if err := json.Unmarshal([]byte(jsonContent), &manifest); err != nil {
		return nil, false
	}

	p.manifestSeen = true
	p.manifest = &manifest

	// Populate expectedJobs for validation
	for _, job := range manifest.Jobs {
		if isValidJobID(job.ID) {
			p.expectedJobs[job.ID] = true
		}
	}

	return &ci.ManifestEvent{Manifest: &manifest}, true
}

// parseStepStart parses step-start marker content.
// Format: "job-id::step-index::step-name" or "job-id::step-index" in parts
func (p *Parser) parseStepStart(parts []string) (any, bool) {
	if len(parts) < 1 {
		return nil, false
	}

	// parts[0] may contain "job-id::index::name"
	stepParts := strings.SplitN(parts[0], "::", 3)

	// Combine with additional parts if needed
	if len(stepParts) == 1 && len(parts) >= 2 {
		// Format: parts = ["job-id", "index::name"] or ["job-id", "index", "name"]
		stepParts = append(stepParts, strings.SplitN(parts[1], "::", 2)...)
	}
	if len(stepParts) == 2 && len(parts) >= 3 {
		stepParts = append(stepParts, parts[2])
	}

	if len(stepParts) < 2 {
		return nil, false
	}

	jobID := strings.TrimSpace(stepParts[0])
	if jobID == "" || !isValidJobID(jobID) {
		return nil, false
	}

	stepIdx, err := strconv.Atoi(strings.TrimSpace(stepParts[1]))
	if err != nil {
		return nil, false
	}

	stepName := ""
	if len(stepParts) >= 3 {
		stepName = stepParts[2]
	}

	return &ci.StepEvent{
		JobID:    jobID,
		StepIdx:  stepIdx,
		StepName: stepName,
	}, true
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
func (p *Parser) parseEmojiSignals(line string) (any, bool) {
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
	// Note: For emoji-based parsing, we use the job name (not ID) since we don't have the ID
	jobName := bracketContent
	if idx := strings.Index(bracketContent, "/"); idx > 0 {
		jobName = strings.TrimSpace(bracketContent[idx+1:])
	}

	event := &ci.JobEvent{
		JobID: jobName, // For emoji fallback, use name as ID (best effort)
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

// Manifest returns the parsed manifest, if one was seen.
func (p *Parser) Manifest() *ci.ManifestInfo {
	return p.manifest
}
