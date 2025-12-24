package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// ParseWorkflowFile reads and parses a single workflow YAML file.
// The path must be validated by the caller to be within an expected directory.
func ParseWorkflowFile(path string) (*Workflow, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path validated by caller via DiscoverWorkflows
	if err != nil {
		return nil, fmt.Errorf("reading workflow file: %w", err)
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
