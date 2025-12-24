package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseWorkflowFile tests parsing valid and invalid workflow files
func TestParseWorkflowFile(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantErr  bool
		validate func(*testing.T, *Workflow)
	}{
		{
			name: "valid workflow with single job",
			content: `name: Test Workflow
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - run: echo "Hello World"
`,
			wantErr: false,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Name != "Test Workflow" {
					t.Errorf("Name = %q, want %q", wf.Name, "Test Workflow")
				}
				if len(wf.Jobs) != 1 {
					t.Fatalf("Jobs count = %d, want 1", len(wf.Jobs))
				}
				job := wf.Jobs["test"]
				if job == nil {
					t.Fatal("Job 'test' should not be nil")
				}
				if len(job.Steps) != 2 {
					t.Errorf("Steps count = %d, want 2", len(job.Steps))
				}
			},
		},
		{
			name: "valid workflow with multiple jobs",
			content: `name: Multi-Job Workflow
on:
  push:
    branches: [main]
  pull_request:
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
  test:
    runs-on: ubuntu-latest
    needs: build
    steps:
      - run: go test ./...
`,
			wantErr: false,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if len(wf.Jobs) != 2 {
					t.Fatalf("Jobs count = %d, want 2", len(wf.Jobs))
				}
				if wf.Jobs["build"] == nil {
					t.Error("Job 'build' should exist")
				}
				if wf.Jobs["test"] == nil {
					t.Error("Job 'test' should exist")
				}
			},
		},
		{
			name: "workflow with env and timeout",
			content: `name: Complex Workflow
on: push
env:
  NODE_VERSION: '18'
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    env:
      GO_VERSION: '1.21'
    steps:
      - uses: actions/checkout@v3
      - name: Run tests
        run: go test
        timeout-minutes: 10
`,
			wantErr: false,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Env == nil {
					t.Fatal("Env should not be nil")
				}
				if wf.Env["NODE_VERSION"] != "18" {
					t.Errorf("Env NODE_VERSION = %q, want %q", wf.Env["NODE_VERSION"], "18")
				}
				job := wf.Jobs["test"]
				if job == nil {
					t.Fatal("Job 'test' should not be nil")
				}
				if job.TimeoutMinutes == nil {
					t.Error("Job TimeoutMinutes should not be nil")
				}
			},
		},
		{
			name: "workflow with continue-on-error",
			content: `name: Continue on Error
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    continue-on-error: true
    steps:
      - run: exit 1
        continue-on-error: true
`,
			wantErr: false,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job == nil {
					t.Fatal("Job 'test' should not be nil")
				}
				if job.ContinueOnError == nil {
					t.Error("Job ContinueOnError should not be nil")
				}
				if len(job.Steps) != 1 {
					t.Fatalf("Steps count = %d, want 1", len(job.Steps))
				}
				if !job.Steps[0].ContinueOnError {
					t.Error("Step ContinueOnError should be true")
				}
			},
		},
		{
			name:    "invalid YAML syntax",
			content: `name: Invalid\ninvalid yaml: [unclosed`,
			wantErr: true,
		},
		{
			name:    "empty file",
			content: "",
			wantErr: false,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs != nil && len(wf.Jobs) > 0 {
					t.Error("Empty file should result in empty workflow")
				}
			},
		},
		{
			name: "workflow with matrix strategy",
			content: `name: Matrix Workflow
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ['1.20', '1.21']
        os: [ubuntu-latest, macos-latest]
    steps:
      - uses: actions/checkout@v3
`,
			wantErr: false,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job == nil {
					t.Fatal("Job 'test' should not be nil")
				}
				if job.Strategy == nil {
					t.Error("Job Strategy should not be nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpFile := filepath.Join(t.TempDir(), "workflow.yml")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Parse workflow
			wf, err := ParseWorkflowFile(tmpFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWorkflowFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, wf)
			}
		})
	}
}

// TestParseWorkflowFile_FileErrors tests file I/O error conditions
func TestParseWorkflowFile_FileErrors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*testing.T) string
		wantErr bool
	}{
		{
			name: "nonexistent file",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent.yml")
			},
			wantErr: true,
		},
		{
			name: "directory instead of file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				return dir
			},
			wantErr: true,
		},
		{
			name: "unreadable file",
			setup: func(t *testing.T) string {
				t.Helper()
				tmpFile := filepath.Join(t.TempDir(), "unreadable.yml")
				if err := os.WriteFile(tmpFile, []byte("test"), 0o000); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return tmpFile
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			_, err := ParseWorkflowFile(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWorkflowFile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "workflow") {
				t.Errorf("Error should contain context about workflow, got: %v", err)
			}
		})
	}
}

// TestDiscoverWorkflows tests discovering workflow files in a directory
func TestDiscoverWorkflows(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T) string
		wantCount int
		wantErr   bool
		validate  func(*testing.T, []string)
	}{
		{
			name: "directory with yml and yaml files",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				files := []string{"workflow1.yml", "workflow2.yaml", "workflow3.yml"}
				for _, file := range files {
					path := filepath.Join(dir, file)
					if err := os.WriteFile(path, []byte("name: test"), 0o600); err != nil {
						t.Fatalf("Failed to create test file: %v", err)
					}
				}
				return dir
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "directory with non-workflow files",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				files := map[string]string{
					"workflow.yml":  "name: test",
					"readme.md":     "# README",
					"config.json":   "{}",
					"script.sh":     "#!/bin/bash",
					"workflow.yaml": "name: test2",
				}
				for file, content := range files {
					path := filepath.Join(dir, file)
					if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
						t.Fatalf("Failed to create test file: %v", err)
					}
				}
				return dir
			},
			wantCount: 2,
			wantErr:   false,
			validate: func(t *testing.T, workflows []string) {
				t.Helper()
				for _, wf := range workflows {
					ext := filepath.Ext(wf)
					if ext != ".yml" && ext != ".yaml" {
						t.Errorf("Workflow %s has invalid extension %s", wf, ext)
					}
				}
			},
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "directory with subdirectories",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				// Create workflow in root
				if err := os.WriteFile(filepath.Join(dir, "root.yml"), []byte("name: root"), 0o600); err != nil {
					t.Fatalf("Failed to create root workflow: %v", err)
				}
				// Create subdirectory with workflow (should be ignored)
				subdir := filepath.Join(dir, "subdir")
				if err := os.MkdirAll(subdir, 0o750); err != nil {
					t.Fatalf("Failed to create subdirectory: %v", err)
				}
				if err := os.WriteFile(filepath.Join(subdir, "nested.yml"), []byte("name: nested"), 0o600); err != nil {
					t.Fatalf("Failed to create nested workflow: %v", err)
				}
				return dir
			},
			wantCount: 1,
			wantErr:   false,
			validate: func(t *testing.T, workflows []string) {
				t.Helper()
				if len(workflows) != 1 {
					return
				}
				if filepath.Base(workflows[0]) != "root.yml" {
					t.Errorf("Expected root.yml, got %s", filepath.Base(workflows[0]))
				}
			},
		},
		{
			name: "directory with symlinks",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				// Create regular file
				regularFile := filepath.Join(dir, "regular.yml")
				if err := os.WriteFile(regularFile, []byte("name: regular"), 0o600); err != nil {
					t.Fatalf("Failed to create regular file: %v", err)
				}
				// Create symlink to another file (should be ignored)
				targetFile := filepath.Join(dir, "target.yml")
				if err := os.WriteFile(targetFile, []byte("name: target"), 0o600); err != nil {
					t.Fatalf("Failed to create target file: %v", err)
				}
				symlinkFile := filepath.Join(dir, "symlink.yml")
				if err := os.Symlink(targetFile, symlinkFile); err != nil {
					// Skip test if symlinks are not supported
					t.Skip("Symlinks not supported on this system")
				}
				return dir
			},
			wantCount: 2, // regular.yml and target.yml, but not symlink.yml
			wantErr:   false,
			validate: func(t *testing.T, workflows []string) {
				t.Helper()
				for _, wf := range workflows {
					base := filepath.Base(wf)
					if base == "symlink.yml" {
						t.Error("Symlinks should be skipped")
					}
				}
			},
		},
		{
			name: "nonexistent directory",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "empty string directory",
			setup: func(t *testing.T) string {
				t.Helper()
				return ""
			},
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			workflows, err := DiscoverWorkflows(dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("DiscoverWorkflows() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(workflows) != tt.wantCount {
					t.Errorf("DiscoverWorkflows() returned %d workflows, want %d", len(workflows), tt.wantCount)
				}

				// Verify all paths are absolute
				for _, wf := range workflows {
					if !filepath.IsAbs(wf) {
						t.Errorf("Workflow path should be absolute, got %s", wf)
					}
				}

				if tt.validate != nil {
					tt.validate(t, workflows)
				}
			}
		})
	}
}

// TestDiscoverWorkflows_PathTraversal tests path traversal prevention
func TestDiscoverWorkflows_PathTraversal(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0o750); err != nil {
		t.Fatalf("Failed to create workflows directory: %v", err)
	}

	// Create a workflow file in workflows directory
	if err := os.WriteFile(filepath.Join(workflowsDir, "safe.yml"), []byte("name: safe"), 0o600); err != nil {
		t.Fatalf("Failed to create safe workflow: %v", err)
	}

	// Create a file outside workflows directory
	outsideDir := filepath.Join(tmpDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o750); err != nil {
		t.Fatalf("Failed to create outside directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "outside.yml"), []byte("name: outside"), 0o600); err != nil {
		t.Fatalf("Failed to create outside workflow: %v", err)
	}

	// Try to create a symlink that points outside the directory
	symlinkPath := filepath.Join(workflowsDir, "escape.yml")
	targetPath := filepath.Join(outsideDir, "outside.yml")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Skip("Symlinks not supported on this system")
	}

	// Discover workflows
	workflows, err := DiscoverWorkflows(workflowsDir)
	if err != nil {
		t.Fatalf("DiscoverWorkflows() failed: %v", err)
	}

	// Verify that the symlink is not included
	for _, wf := range workflows {
		if filepath.Base(wf) == "escape.yml" {
			t.Error("Symlink pointing outside directory should be skipped")
		}
		// Verify all workflows are within the directory
		relPath, err := filepath.Rel(workflowsDir, wf)
		if err != nil || strings.HasPrefix(relPath, "..") {
			t.Errorf("Workflow %s is outside the workflows directory", wf)
		}
	}
}

// TestDiscoverWorkflows_ErrorWrapping tests that errors are properly wrapped
func TestDiscoverWorkflows_ErrorWrapping(t *testing.T) {
	tests := []struct {
		name        string
		dir         string
		errorSubstr string
	}{
		{
			name:        "empty directory path",
			dir:         "",
			errorSubstr: "cannot be empty",
		},
		{
			name:        "nonexistent directory",
			dir:         filepath.Join(t.TempDir(), "nonexistent"),
			errorSubstr: "reading workflows directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DiscoverWorkflows(tt.dir)
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.errorSubstr) {
				t.Errorf("Error should contain %q, got: %v", tt.errorSubstr, err)
			}
		})
	}
}

// TestDiscoverWorkflows_CaseInsensitiveExtensions tests extension handling
func TestDiscoverWorkflows_CaseInsensitiveExtensions(t *testing.T) {
	dir := t.TempDir()

	// Create files with various extensions
	files := []struct {
		name      string
		shouldFind bool
	}{
		{"workflow.yml", true},
		{"workflow.yaml", true},
		{"workflow.YML", false},   // uppercase not supported
		{"workflow.YAML", false},  // uppercase not supported
		{"workflow.Yml", false},   // mixed case not supported
		{"workflow.txt", false},
		{"workflow", false},
	}

	for _, file := range files {
		path := filepath.Join(dir, file.name)
		if err := os.WriteFile(path, []byte("name: test"), 0o600); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file.name, err)
		}
	}

	workflows, err := DiscoverWorkflows(dir)
	if err != nil {
		t.Fatalf("DiscoverWorkflows() failed: %v", err)
	}

	foundFiles := make(map[string]bool)
	for _, wf := range workflows {
		foundFiles[filepath.Base(wf)] = true
	}

	for _, file := range files {
		found := foundFiles[file.name]
		if found != file.shouldFind {
			t.Errorf("File %s: found = %v, want %v", file.name, found, file.shouldFind)
		}
	}
}
