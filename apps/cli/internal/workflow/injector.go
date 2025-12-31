package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/detent/cli/internal/ci"
	"github.com/goccy/go-yaml"
	"golang.org/x/sync/errgroup"
)

// validJobIDPattern matches GitHub Actions job ID requirements: [a-zA-Z_][a-zA-Z0-9_-]*
// This prevents shell injection via malicious job IDs in marker echo commands.
var validJobIDPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)

// InjectContinueOnError modifies a workflow to add continue-on-error: true to all jobs and steps.
// This ensures that Docker failures, job-level failures, and step-level failures don't stop execution,
// allowing Detent to capture ALL errors instead of just the first failure.
func InjectContinueOnError(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}
	for _, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Inject at JOB level ONLY (critical for Docker failures and continuing past step failures)
		// Job.ContinueOnError is `any` type to support bool or expressions
		// NOTE: We intentionally do NOT inject at step level because it suppresses step output in act,
		// preventing error extraction. Job-level continue-on-error is sufficient to prevent workflow truncation.
		if job.ContinueOnError == nil || job.ContinueOnError == false {
			job.ContinueOnError = true
		}
	}
}

const (
	// Timeout values in minutes to prevent hanging Docker operations
	defaultJobTimeoutMinutes  = 30 // Default timeout for jobs
	defaultStepTimeoutMinutes = 15 // Default timeout for steps
)

// InjectTimeouts adds reasonable timeout values to prevent hanging Docker operations.
// Jobs default to 30 minutes, steps to 15 minutes. Only applied if not already set.
func InjectTimeouts(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}

	for _, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Set job timeout if not already specified
		if job.TimeoutMinutes == nil {
			job.TimeoutMinutes = defaultJobTimeoutMinutes
		}

		// Set step timeouts if not already specified
		if job.Steps != nil {
			for _, step := range job.Steps {
				if step == nil {
					continue
				}
				if step.TimeoutMinutes == nil {
					step.TimeoutMinutes = defaultStepTimeoutMinutes
				}
			}
		}
	}
}

// BuildManifest creates a v2 manifest from a workflow containing full job and step information.
// The manifest includes job IDs, display names, step names, dependencies, and reusable workflow references.
// Jobs are returned in topological order (respecting dependencies).
func BuildManifest(wf *Workflow) *ci.ManifestInfo {
	if wf == nil || wf.Jobs == nil {
		return &ci.ManifestInfo{Version: 2, Jobs: []ci.ManifestJob{}}
	}

	// Build job info map for topological sorting
	jobInfoMap := make(map[string]*ci.ManifestJob)
	for jobID, job := range wf.Jobs {
		if job == nil || !isValidJobID(jobID) {
			continue
		}

		mj := &ci.ManifestJob{
			ID:   jobID,
			Name: job.Name,
		}
		if mj.Name == "" {
			mj.Name = jobID
		}

		// Handle reusable workflows
		if job.Uses != "" {
			mj.Uses = job.Uses
		} else {
			// Extract step names
			for _, step := range job.Steps {
				stepName := getStepDisplayName(step)
				mj.Steps = append(mj.Steps, stepName)
			}
		}

		// Parse dependencies
		mj.Needs = parseJobNeeds(job.Needs)

		jobInfoMap[jobID] = mj
	}

	// Topological sort for consistent ordering
	sortedJobs := topologicalSortManifest(jobInfoMap)

	return &ci.ManifestInfo{
		Version: 2,
		Jobs:    sortedJobs,
	}
}

// BuildCombinedManifest builds a single manifest from multiple workflows.
// This ensures all jobs from all workflow files are included in one manifest,
// which is injected once for consistent TUI display.
func BuildCombinedManifest(workflows map[string]*Workflow) *ci.ManifestInfo {
	if len(workflows) == 0 {
		return &ci.ManifestInfo{Version: 2, Jobs: []ci.ManifestJob{}}
	}

	// Sort workflow paths for deterministic ordering
	paths := make([]string, 0, len(workflows))
	for p := range workflows {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Collect all jobs from all workflows
	allJobsMap := make(map[string]*ci.ManifestJob)
	for _, path := range paths {
		wf := workflows[path]
		wfManifest := BuildManifest(wf)
		for i := range wfManifest.Jobs {
			job := &wfManifest.Jobs[i]
			allJobsMap[job.ID] = job
		}
	}

	// Topological sort for consistent ordering
	sortedJobs := topologicalSortManifest(allJobsMap)

	return &ci.ManifestInfo{
		Version: 2,
		Jobs:    sortedJobs,
	}
}

// findFirstJobAcrossWorkflows finds the first valid job ID and its workflow path
// across all workflows. Prefers jobs WITHOUT dependencies (needs:) since those
// run first. Among valid candidates, picks alphabetically first.
func findFirstJobAcrossWorkflows(workflows map[string]*Workflow) (workflowPath, jobID string) {
	// Sort workflow paths for deterministic ordering
	paths := make([]string, 0, len(workflows))
	for p := range workflows {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// First pass: find jobs without dependencies (they run first)
	var bestPath, bestJobID string
	var fallbackPath, fallbackJobID string // Fallback if all jobs have dependencies

	for _, path := range paths {
		wf := workflows[path]
		if wf == nil || wf.Jobs == nil {
			continue
		}

		for jID, job := range wf.Jobs {
			if job == nil || job.Uses != "" || !isValidJobID(jID) {
				continue
			}

			// Track fallback (any valid job, alphabetically first)
			if fallbackJobID == "" || path < fallbackPath || (path == fallbackPath && jID < fallbackJobID) {
				fallbackPath = path
				fallbackJobID = jID
			}

			// Prefer jobs without dependencies
			if !jobHasNeeds(job) {
				if bestJobID == "" || path < bestPath || (path == bestPath && jID < bestJobID) {
					bestPath = path
					bestJobID = jID
				}
			}
		}
	}

	// Return job without dependencies if found, otherwise fallback
	if bestJobID != "" {
		return bestPath, bestJobID
	}
	return fallbackPath, fallbackJobID
}

// jobHasNeeds returns true if the job has dependencies (needs field).
func jobHasNeeds(job *Job) bool {
	if job == nil || job.Needs == nil {
		return false
	}
	switch v := job.Needs.(type) {
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	}
	return false
}

// getStepDisplayName returns a human-readable name for a step.
// Tries: step.Name, step.ID, action name from step.Uses, truncated run command.
func getStepDisplayName(step *Step) string {
	if step == nil {
		return "Unknown step"
	}
	if step.Name != "" {
		return step.Name
	}
	if step.ID != "" {
		return step.ID
	}
	if step.Uses != "" {
		// Extract action name from "owner/repo@ref" or "owner/repo/path@ref"
		parts := strings.Split(step.Uses, "@")
		if len(parts) > 0 {
			actionPath := parts[0]
			segments := strings.Split(actionPath, "/")
			if len(segments) >= 2 {
				return segments[len(segments)-1] // Last segment is action name
			}
			return actionPath
		}
		return step.Uses
	}
	if step.Run != "" {
		// Truncate long run commands
		run := strings.TrimSpace(step.Run)
		run = strings.Split(run, "\n")[0] // First line only
		if len(run) > 40 {
			return run[:37] + "..."
		}
		return run
	}
	return "Step"
}

// parseJobNeeds extracts job dependencies from the needs field.
// Handles both string and []string formats.
func parseJobNeeds(needs any) []string {
	if needs == nil {
		return nil
	}

	switch v := needs.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	}
	return nil
}

// topologicalSortManifest sorts jobs by dependencies (jobs with fewer deps first).
func topologicalSortManifest(jobInfoMap map[string]*ci.ManifestJob) []ci.ManifestJob {
	// Build in-degree map
	inDegree := make(map[string]int)
	for id := range jobInfoMap {
		inDegree[id] = 0
	}
	for _, job := range jobInfoMap {
		for _, need := range job.Needs {
			if _, exists := jobInfoMap[need]; exists {
				inDegree[job.ID]++
			}
		}
	}

	// Kahn's algorithm with stable sorting
	var result []ci.ManifestJob
	var queue []string

	// Start with jobs that have no dependencies
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue) // Stable order

	for len(queue) > 0 {
		// Pop first item
		current := queue[0]
		queue = queue[1:]

		if job, exists := jobInfoMap[current]; exists {
			result = append(result, *job)
		}

		// Find jobs that depend on current
		var nextBatch []string
		for id, job := range jobInfoMap {
			for _, need := range job.Needs {
				if need == current {
					inDegree[id]--
					if inDegree[id] == 0 {
						nextBatch = append(nextBatch, id)
					}
					break
				}
			}
		}
		sort.Strings(nextBatch)
		queue = append(queue, nextBatch...)
	}

	// Add any remaining jobs (cycles or missing deps)
	if len(result) < len(jobInfoMap) {
		var remaining []string
		added := make(map[string]bool)
		for _, job := range result {
			added[job.ID] = true
		}
		for id := range jobInfoMap {
			if !added[id] {
				remaining = append(remaining, id)
			}
		}
		sort.Strings(remaining)
		for _, id := range remaining {
			if job, exists := jobInfoMap[id]; exists {
				result = append(result, *job)
			}
		}
	}

	return result
}

// InjectJobMarkers injects lifecycle marker steps into each job for reliable job tracking.
// This uses the v2 manifest format with full step information.
// Each job gets:
// - A manifest step (first job only, contains all job/step info as JSON)
// - Step-start markers before each user step
// - A job-end marker with always() condition
// Jobs using reusable workflows (uses:) are skipped as they have no steps to inject.
// Jobs with invalid IDs (not matching GitHub Actions spec) are skipped for security.
//
// Note: For multi-workflow scenarios, use InjectJobMarkersWithManifest to share a combined manifest.
func InjectJobMarkers(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}

	// Build the v2 manifest for this single workflow
	manifest := BuildManifest(wf)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		manifestJSON = []byte(`{"v":2,"jobs":[]}`)
	}

	// Find the first job alphabetically to inject manifest
	var firstJobID string
	for jobID, job := range wf.Jobs {
		if job == nil || job.Uses != "" || !isValidJobID(jobID) {
			continue
		}
		if firstJobID == "" || jobID < firstJobID {
			firstJobID = jobID
		}
	}

	injectJobMarkersInternal(wf, manifestJSON, firstJobID)
}

// InjectJobMarkersWithManifest injects lifecycle markers using an externally-provided manifest.
// Use this when processing multiple workflows to ensure all jobs appear in a single manifest.
// Parameters:
//   - wf: The workflow to inject markers into
//   - manifestJSON: Pre-built manifest JSON (nil to skip manifest injection for this workflow)
//   - manifestJobID: The job ID that should receive the manifest step (empty string to skip)
func InjectJobMarkersWithManifest(wf *Workflow, manifestJSON []byte, manifestJobID string) {
	if wf == nil || wf.Jobs == nil {
		return
	}
	injectJobMarkersInternal(wf, manifestJSON, manifestJobID)
}

// injectJobMarkersInternal is the shared implementation for marker injection.
func injectJobMarkersInternal(wf *Workflow, manifestJSON []byte, manifestJobID string) {
	for jobID, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Skip reusable workflows (they have no steps to inject)
		if job.Uses != "" {
			continue
		}

		// Skip jobs with invalid IDs to prevent shell injection
		if !isValidJobID(jobID) {
			continue
		}

		var newSteps []*Step

		// Add manifest step only to the designated job
		if manifestJSON != nil && jobID == manifestJobID {
			manifestStep := &Step{
				Name: "detent: manifest",
				Run:  fmt.Sprintf("echo '::detent::manifest::v2::%s'", escapeForShell(string(manifestJSON))),
			}
			newSteps = append(newSteps, manifestStep)
		}

		// Add job-start marker
		jobStartStep := &Step{
			Name: "detent: job start",
			Run:  fmt.Sprintf("echo '::detent::job-start::%s'", jobID),
		}
		newSteps = append(newSteps, jobStartStep)

		// Add step markers before each original step
		for i, step := range job.Steps {
			stepName := getStepDisplayName(step)
			markerStep := &Step{
				Name: fmt.Sprintf("detent: step %d", i),
				Run:  fmt.Sprintf("echo '::detent::step-start::%s::%d::%s'", jobID, i, escapeForShell(stepName)),
			}
			newSteps = append(newSteps, markerStep, step)
		}

		// Add job end marker with always() to capture success/failure/cancelled
		endStep := &Step{
			Name: "detent: job end",
			If:   "always()",
			Run:  fmt.Sprintf("echo '::detent::job-end::%s::${{ job.status }}'", jobID),
		}
		newSteps = append(newSteps, endStep)

		job.Steps = newSteps
	}
}

// escapeForShell escapes a string for safe use in single-quoted shell strings.
// Single quotes are replaced with '\'' (end quote, escaped quote, start quote).
func escapeForShell(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// isValidJobID checks if a job ID matches GitHub Actions requirements.
// Valid IDs must start with a letter or underscore and contain only alphanumeric, underscore, or hyphen.
// This validation prevents shell injection in marker echo commands.
func isValidJobID(jobID string) bool {
	return validJobIDPattern.MatchString(jobID)
}

// PrepareWorkflows processes workflows and returns temp directory path.
// If specificWorkflow is provided, only that workflow is processed.
// Otherwise, all workflows in srcDir are discovered and processed.
func PrepareWorkflows(srcDir, specificWorkflow string) (tmpDir string, cleanup func(), err error) {
	var workflows []string

	if specificWorkflow != "" {
		// Validate path BEFORE cleaning to catch patterns like ./file
		if filepath.IsAbs(specificWorkflow) || specificWorkflow != "" && specificWorkflow[0] == '.' {
			return "", nil, fmt.Errorf("workflow path must be relative and cannot reference parent directories")
		}

		// Clean the path after validation
		cleanWorkflow := filepath.Clean(specificWorkflow)

		// Get absolute paths for validation
		absSrcDir, absErr := filepath.Abs(srcDir)
		if absErr != nil {
			return "", nil, fmt.Errorf("resolving source directory: %w", absErr)
		}

		// Process specific workflow file
		workflowPath := filepath.Join(absSrcDir, cleanWorkflow)
		absPath, absPathErr := filepath.Abs(workflowPath)
		if absPathErr != nil {
			return "", nil, fmt.Errorf("resolving workflow path: %w", absPathErr)
		}

		// Validate the resolved path is within the source directory using filepath.Rel
		relPath, relErr := filepath.Rel(absSrcDir, absPath)
		if relErr != nil || strings.HasPrefix(relPath, "..") {
			return "", nil, fmt.Errorf("workflow path must be within the workflows directory")
		}

		// Validate file exists and is a workflow file
		fileInfo, statErr := os.Lstat(absPath)
		if statErr != nil {
			return "", nil, fmt.Errorf("workflow file not found: %w", statErr)
		}

		// Reject symlinks to prevent path traversal
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("workflow file cannot be a symlink")
		}

		ext := filepath.Ext(cleanWorkflow)
		if ext != ".yml" && ext != ".yaml" {
			return "", nil, fmt.Errorf("workflow file must have .yml or .yaml extension")
		}

		workflows = []string{absPath}
	} else {
		// Discover all workflows
		workflows, err = DiscoverWorkflows(srcDir)
		if err != nil {
			return "", nil, err
		}

		if len(workflows) == 0 {
			return "", nil, fmt.Errorf("no workflow files found in %s", srcDir)
		}
	}

	// First pass: parse and validate all workflows before creating temp directory
	parsedWorkflows := make(map[string]*Workflow, len(workflows))
	for _, wfPath := range workflows {
		wf, parseErr := ParseWorkflowFile(wfPath)
		if parseErr != nil {
			return "", nil, fmt.Errorf("parsing %s: %w", wfPath, parseErr)
		}
		parsedWorkflows[wfPath] = wf
	}

	// Validate all workflows for unsupported features
	var allWorkflows []*Workflow
	for _, wf := range parsedWorkflows {
		allWorkflows = append(allWorkflows, wf)
	}
	if validationErr := ValidateWorkflows(allWorkflows); validationErr != nil {
		// Only block on actual errors, not warnings
		if validationErrors, ok := validationErr.(ValidationErrors); ok {
			if validationErrors.HasErrors() {
				return "", nil, validationErrors.Errors()
			}
			// Warnings only - continue execution (warnings are logged elsewhere if needed)
		} else {
			return "", nil, validationErr
		}
	}

	tmpDir, err = os.MkdirTemp("", "detent-workflows-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp directory: %w", err)
	}

	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	// Build combined manifest from ALL workflows before processing
	// This ensures the TUI sees all jobs from all workflow files in a single manifest
	combinedManifest := BuildCombinedManifest(parsedWorkflows)
	combinedManifestJSON, err := json.Marshal(combinedManifest)
	if err != nil {
		combinedManifestJSON = []byte(`{"v":2,"jobs":[]}`)
	}

	// Find which workflow and job should receive the manifest
	manifestWfPath, manifestJobID := findFirstJobAcrossWorkflows(parsedWorkflows)

	// Process workflows in parallel using errgroup
	// Each workflow is independent, so parallel processing is safe
	var g errgroup.Group
	var mu sync.Mutex // Protects file writes to tmpDir

	// Set a reasonable concurrency limit to avoid resource exhaustion
	// This limits the number of concurrent workflow processing goroutines
	g.SetLimit(10)

	for wfPath, wf := range parsedWorkflows {
		wfPath := wfPath // Capture loop variable for goroutine
		wf := wf         // Capture loop variable for goroutine
		g.Go(func() error {
			// Apply modifications
			// Order matters: continue-on-error first, then markers, then timeouts
			InjectContinueOnError(wf)

			// Inject markers with combined manifest (only first workflow gets manifest step)
			if wfPath == manifestWfPath {
				InjectJobMarkersWithManifest(wf, combinedManifestJSON, manifestJobID)
			} else {
				// Other workflows get markers but no manifest
				InjectJobMarkersWithManifest(wf, nil, "")
			}

			InjectTimeouts(wf)

			// Marshal to YAML
			data, marshalErr := yaml.Marshal(wf)
			if marshalErr != nil {
				return fmt.Errorf("marshaling %s: %w", wfPath, marshalErr)
			}

			// Write to temp directory (mutex-protected to ensure thread-safe file writes)
			filename := filepath.Base(wfPath)
			mu.Lock()
			writeErr := os.WriteFile(filepath.Join(tmpDir, filename), data, 0o600)
			mu.Unlock()

			if writeErr != nil {
				return fmt.Errorf("writing %s: %w", filename, writeErr)
			}

			return nil
		})
	}

	// Wait for all goroutines to complete and check for errors
	if err := g.Wait(); err != nil {
		cleanup()
		return "", nil, err
	}

	return tmpDir, cleanup, nil
}
