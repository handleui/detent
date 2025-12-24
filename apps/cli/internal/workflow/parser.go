package workflow

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

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
		if err != nil || len(relPath) >= 2 && relPath[:2] == ".." {
			continue
		}

		workflows = append(workflows, fullPath)
	}

	return workflows, nil
}
