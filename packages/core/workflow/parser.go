package workflow

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

const (
	// maxWorkflowSizeBytes is the maximum allowed size for a workflow file (1MB)
	// This prevents resource exhaustion from maliciously large files
	maxWorkflowSizeBytes = 1 * 1024 * 1024
)

// validateWorkflowContent checks for potentially malicious or malformed content.
// This provides defense-in-depth against crafted workflow files.
func validateWorkflowContent(data []byte) error {
	// Size limit to prevent resource exhaustion
	if len(data) > maxWorkflowSizeBytes {
		return fmt.Errorf("workflow file exceeds maximum size of %d bytes", maxWorkflowSizeBytes)
	}

	// Null bytes indicate binary content disguised as YAML
	if bytes.Contains(data, []byte{0x00}) {
		return fmt.Errorf("workflow file contains null bytes (binary content not allowed)")
	}

	// Check for excessive control characters (excluding newline, carriage return, tab)
	// This catches malformed files that might exploit YAML parser edge cases
	controlCount := 0
	for _, b := range data {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			controlCount++
		}
	}
	if controlCount > 10 {
		return fmt.Errorf("workflow file contains excessive control characters (%d found)", controlCount)
	}

	return nil
}

// ParseWorkflowFile reads and parses a single workflow YAML file.
// The path must be validated by the caller to be within an expected directory.
func ParseWorkflowFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path validated by caller via DiscoverWorkflows
	if err != nil {
		return nil, fmt.Errorf("reading workflow file: %w", err)
	}

	// Validate content before parsing (defense-in-depth)
	if err := validateWorkflowContent(data); err != nil {
		return nil, err
	}

	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}

	return &wf, nil
}

// DiscoverWorkflows finds all workflow files in a directory.
// Only regular files with .yml or .yaml extensions are returned.
// Symlinks and files outside the directory are skipped for security.
func DiscoverWorkflows(dir string) ([]string, error) {
	if dir == "" {
		return nil, fmt.Errorf("workflows directory cannot be empty")
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving workflows directory: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading workflows directory: %w", err)
	}

	var workflows []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip symlinks to prevent path traversal
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yml" && ext != ".yaml" {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			continue
		}

		// Ensure resolved path is within the directory using filepath.Rel
		relPath, err := filepath.Rel(absDir, absPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			continue
		}

		workflows = append(workflows, fullPath)
	}

	return workflows, nil
}

// ExtractJobInfo extracts job information from a workflow for TUI display.
// Returns a slice of JobInfo with ID and display name for each job.
// Jobs are sorted topologically by needs dependencies, with alphabetical ordering
// as a tiebreaker for jobs at the same dependency level.
func ExtractJobInfo(wf *Workflow) []JobInfo {
	if wf == nil || wf.Jobs == nil {
		return nil
	}

	// First pass: build job info map and parse needs
	jobInfoMap := make(map[string]JobInfo)
	for id, job := range wf.Jobs {
		if job == nil {
			continue
		}

		name := job.Name
		if name == "" {
			name = id
		}

		// Parse needs (can be string or []string)
		var needs []string
		switch n := job.Needs.(type) {
		case string:
			if n != "" {
				needs = []string{n}
			}
		case []any:
			for _, v := range n {
				if s, ok := v.(string); ok {
					needs = append(needs, s)
				}
			}
		}

		jobInfoMap[id] = JobInfo{
			ID:    id,
			Name:  name,
			Needs: needs,
		}
	}

	// Topological sort using Kahn's algorithm
	return topologicalSort(jobInfoMap)
}

// topologicalSort performs a topological sort of jobs based on their needs dependencies.
// Jobs with no dependencies come first, followed by jobs that depend on them.
// Within each level, jobs are sorted alphabetically for deterministic ordering.
func topologicalSort(jobInfoMap map[string]JobInfo) []JobInfo {
	if len(jobInfoMap) == 0 {
		return nil
	}

	// Calculate in-degree (number of dependencies) for each job
	inDegree := make(map[string]int)
	for id := range jobInfoMap {
		inDegree[id] = 0
	}

	// Build adjacency list and count in-degrees
	// dependents[a] contains all jobs that depend on a
	dependents := make(map[string][]string)
	for id, job := range jobInfoMap {
		for _, dep := range job.Needs {
			// Only count dependencies that exist in the workflow
			if _, exists := jobInfoMap[dep]; exists {
				inDegree[id]++
				dependents[dep] = append(dependents[dep], id)
			}
		}
	}

	// Start with jobs that have no dependencies
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	// Sort for deterministic ordering
	sort.Strings(queue)

	var result []JobInfo
	for len(queue) > 0 {
		// Process all jobs at current level
		current := queue[0]
		queue = queue[1:]

		result = append(result, jobInfoMap[current])

		// Find all dependents and reduce their in-degree
		var nextLevel []string
		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				nextLevel = append(nextLevel, dependent)
			}
		}

		// Sort next level for deterministic ordering and add to queue
		sort.Strings(nextLevel)
		queue = append(queue, nextLevel...)
	}

	// Handle cycles or missing dependencies - add remaining jobs alphabetically
	if len(result) < len(jobInfoMap) {
		var remaining []string
		addedSet := make(map[string]bool)
		for _, job := range result {
			addedSet[job.ID] = true
		}
		for id := range jobInfoMap {
			if !addedSet[id] {
				remaining = append(remaining, id)
			}
		}
		sort.Strings(remaining)
		for _, id := range remaining {
			result = append(result, jobInfoMap[id])
		}
	}

	return result
}

// ExtractJobInfoFromDir discovers and parses all workflows in a directory,
// returning job info for all jobs across all workflows.
func ExtractJobInfoFromDir(dir string) ([]JobInfo, error) {
	workflows, err := DiscoverWorkflows(dir)
	if err != nil {
		return nil, err
	}

	var allJobs []JobInfo
	for _, wfPath := range workflows {
		wf, err := ParseWorkflowFile(wfPath)
		if err != nil {
			continue // Skip workflows that fail to parse
		}
		jobs := ExtractJobInfo(wf)
		allJobs = append(allJobs, jobs...)
	}

	return allJobs, nil
}
