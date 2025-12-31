package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInjectContinueOnError tests injecting continue-on-error to jobs
func TestInjectContinueOnError(t *testing.T) {
	tests := []struct {
		name     string
		workflow *Workflow
		validate func(*testing.T, *Workflow)
	}{
		{
			name: "inject to job without continue-on-error",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "echo test"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job.ContinueOnError == nil {
					t.Error("ContinueOnError should be set")
					return
				}
				if job.ContinueOnError != true {
					t.Errorf("ContinueOnError = %v, want true", job.ContinueOnError)
				}
			},
		},
		{
			name: "preserve existing true value",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn:          "ubuntu-latest",
						ContinueOnError: true,
						Steps:           []*Step{{Run: "echo test"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job.ContinueOnError != true {
					t.Errorf("ContinueOnError should remain true")
				}
			},
		},
		{
			name: "override false value",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn:          "ubuntu-latest",
						ContinueOnError: false,
						Steps:           []*Step{{Run: "echo test"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job.ContinueOnError != true {
					t.Errorf("ContinueOnError should be overridden to true")
				}
			},
		},
		{
			name: "inject to multiple jobs",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "go build"}},
					},
					"test": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "go test"}},
					},
					"lint": {
						RunsOn:          "ubuntu-latest",
						ContinueOnError: true,
						Steps:           []*Step{{Run: "golangci-lint"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				for name, job := range wf.Jobs {
					if job.ContinueOnError != true {
						t.Errorf("Job %s: ContinueOnError should be true", name)
					}
				}
			},
		},
		{
			name:     "nil workflow",
			workflow: nil,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
			},
		},
		{
			name: "workflow with nil jobs map",
			workflow: &Workflow{
				Name: "Test",
				Jobs: nil,
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
			},
		},
		{
			name: "workflow with nil job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test":  {RunsOn: "ubuntu-latest"},
					"build": nil,
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["test"].ContinueOnError != true {
					t.Error("Valid job should have ContinueOnError set")
				}
				// Nil job should not cause panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InjectContinueOnError(tt.workflow)
			tt.validate(t, tt.workflow)
		})
	}
}

// TestInjectTimeouts tests injecting timeouts to jobs and steps
func TestInjectTimeouts(t *testing.T) {
	tests := []struct {
		name     string
		workflow *Workflow
		validate func(*testing.T, *Workflow)
	}{
		{
			name: "inject job and step timeouts",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps: []*Step{
							{Run: "echo test1"},
							{Run: "echo test2"},
						},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job.TimeoutMinutes == nil {
					t.Error("Job TimeoutMinutes should be set")
					return
				}
				if job.TimeoutMinutes != defaultJobTimeoutMinutes {
					t.Errorf("Job TimeoutMinutes = %v, want %v", job.TimeoutMinutes, defaultJobTimeoutMinutes)
				}
				for i, step := range job.Steps {
					if step.TimeoutMinutes == nil {
						t.Errorf("Step %d TimeoutMinutes should be set", i)
						continue
					}
					if step.TimeoutMinutes != defaultStepTimeoutMinutes {
						t.Errorf("Step %d TimeoutMinutes = %v, want %v", i, step.TimeoutMinutes, defaultStepTimeoutMinutes)
					}
				}
			},
		},
		{
			name: "preserve existing job timeout",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn:         "ubuntu-latest",
						TimeoutMinutes: 60,
						Steps: []*Step{
							{Run: "echo test"},
						},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job.TimeoutMinutes != 60 {
					t.Errorf("Job TimeoutMinutes should remain 60, got %v", job.TimeoutMinutes)
				}
			},
		},
		{
			name: "preserve existing step timeout",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps: []*Step{
							{Run: "echo test", TimeoutMinutes: 20},
						},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				step := wf.Jobs["test"].Steps[0]
				if step.TimeoutMinutes != 20 {
					t.Errorf("Step TimeoutMinutes should remain 20, got %v", step.TimeoutMinutes)
				}
			},
		},
		{
			name: "inject to multiple jobs and steps",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {
						RunsOn: "ubuntu-latest",
						Steps: []*Step{
							{Run: "go build"},
							{Run: "go install"},
						},
					},
					"test": {
						RunsOn:         "ubuntu-latest",
						TimeoutMinutes: 45,
						Steps: []*Step{
							{Run: "go test", TimeoutMinutes: 25},
							{Run: "go test -race"},
						},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Build job should get default timeout
				if wf.Jobs["build"].TimeoutMinutes != defaultJobTimeoutMinutes {
					t.Errorf("Build job timeout = %v, want %v", wf.Jobs["build"].TimeoutMinutes, defaultJobTimeoutMinutes)
				}
				// Test job should keep custom timeout
				if wf.Jobs["test"].TimeoutMinutes != 45 {
					t.Error("Test job custom timeout should be preserved")
				}
				// Build steps should get default timeout
				for i, step := range wf.Jobs["build"].Steps {
					if step.TimeoutMinutes != defaultStepTimeoutMinutes {
						t.Errorf("Build step %d timeout = %v, want %v", i, step.TimeoutMinutes, defaultStepTimeoutMinutes)
					}
				}
				// Test step 0 should keep custom timeout
				if wf.Jobs["test"].Steps[0].TimeoutMinutes != 25 {
					t.Error("Test step 0 custom timeout should be preserved")
				}
				// Test step 1 should get default timeout
				if wf.Jobs["test"].Steps[1].TimeoutMinutes != defaultStepTimeoutMinutes {
					t.Errorf("Test step 1 timeout = %v, want %v", wf.Jobs["test"].Steps[1].TimeoutMinutes, defaultStepTimeoutMinutes)
				}
			},
		},
		{
			name:     "nil workflow",
			workflow: nil,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
			},
		},
		{
			name: "workflow with nil jobs map",
			workflow: &Workflow{
				Name: "Test",
				Jobs: nil,
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
			},
		},
		{
			name: "job with nil steps",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps:  nil,
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["test"]
				if job.TimeoutMinutes != defaultJobTimeoutMinutes {
					t.Error("Job timeout should be set even without steps")
				}
			},
		},
		{
			name: "job with nil step",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {
						RunsOn: "ubuntu-latest",
						Steps: []*Step{
							{Run: "echo test"},
							nil,
						},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
				if wf.Jobs["test"].Steps[0].TimeoutMinutes != defaultStepTimeoutMinutes {
					t.Error("Valid step should have timeout set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InjectTimeouts(tt.workflow)
			tt.validate(t, tt.workflow)
		})
	}
}

// TestInjectJobMarkers tests injecting job lifecycle markers
func TestInjectJobMarkers(t *testing.T) {
	tests := []struct {
		name     string
		workflow *Workflow
		validate func(*testing.T, *Workflow)
	}{
		{
			name: "inject markers to single job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "echo test"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["build"]
				if len(job.Steps) != 3 { // start marker + original step + end marker
					t.Errorf("Expected 3 steps, got %d", len(job.Steps))
					return
				}

				// Check start marker
				startStep := job.Steps[0]
				if startStep.Name != "detent: job start" {
					t.Errorf("First step name = %q, want %q", startStep.Name, "detent: job start")
				}
				if !strings.Contains(startStep.Run, "::detent::manifest::") {
					t.Error("Start step should contain manifest marker")
				}
				if !strings.Contains(startStep.Run, "::detent::job-start::build") {
					t.Error("Start step should contain job-start marker")
				}

				// Check original step is preserved
				if job.Steps[1].Run != "echo test" {
					t.Errorf("Middle step should be original, got %q", job.Steps[1].Run)
				}

				// Check end marker
				endStep := job.Steps[2]
				if endStep.Name != "detent: job end" {
					t.Errorf("Last step name = %q, want %q", endStep.Name, "detent: job end")
				}
				if endStep.If != "always()" {
					t.Errorf("End step should have if: always(), got %q", endStep.If)
				}
				if !strings.Contains(endStep.Run, "::detent::job-end::build::${{ job.status }}") {
					t.Errorf("End step should contain job-end marker, got %q", endStep.Run)
				}
			},
		},
		{
			name: "inject markers to multiple jobs with sorted manifest",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"zebra": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "echo zebra"}},
					},
					"alpha": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "echo alpha"}},
					},
					"beta": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "echo beta"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// All jobs should have the same sorted manifest
				expectedManifest := "::detent::manifest::alpha,beta,zebra"

				for jobID, job := range wf.Jobs {
					if len(job.Steps) != 3 {
						t.Errorf("Job %s: expected 3 steps, got %d", jobID, len(job.Steps))
						continue
					}

					startStep := job.Steps[0]
					if !strings.Contains(startStep.Run, expectedManifest) {
						t.Errorf("Job %s: manifest should be sorted, got %q", jobID, startStep.Run)
					}

					// Each job should have its own job-start marker
					expectedStart := "::detent::job-start::" + jobID
					if !strings.Contains(startStep.Run, expectedStart) {
						t.Errorf("Job %s: start marker should contain job ID, got %q", jobID, startStep.Run)
					}
				}
			},
		},
		{
			name: "skip reusable workflow jobs",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"regular": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "echo test"}},
					},
					"reusable": {
						Uses: "./.github/workflows/other.yml",
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Regular job should have markers
				regularJob := wf.Jobs["regular"]
				if len(regularJob.Steps) != 3 {
					t.Errorf("Regular job: expected 3 steps, got %d", len(regularJob.Steps))
				}

				// Reusable job should be unchanged (no Steps)
				reusableJob := wf.Jobs["reusable"]
				if reusableJob.Steps != nil {
					t.Error("Reusable job should not have steps added")
				}

				// Manifest should include both jobs
				startStep := regularJob.Steps[0]
				if !strings.Contains(startStep.Run, "regular") {
					t.Error("Manifest should include regular job")
				}
				if !strings.Contains(startStep.Run, "reusable") {
					t.Error("Manifest should include reusable job")
				}
			},
		},
		{
			name: "preserve empty steps array",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"empty": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				job := wf.Jobs["empty"]
				// Should add start and end markers even with empty steps
				if len(job.Steps) != 2 {
					t.Errorf("Expected 2 steps (markers only), got %d", len(job.Steps))
				}
			},
		},
		{
			name:     "nil workflow",
			workflow: nil,
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
			},
		},
		{
			name: "workflow with nil jobs map",
			workflow: &Workflow{
				Name: "Test",
				Jobs: nil,
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
			},
		},
		{
			name: "workflow with nil job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"valid": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
					"nil":   nil,
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Should not panic
				validJob := wf.Jobs["valid"]
				if len(validJob.Steps) != 3 {
					t.Errorf("Valid job should have markers, got %d steps", len(validJob.Steps))
				}
			},
		},
		{
			name: "skip invalid job IDs - shell injection attempts",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"valid_job": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo valid"}}},
					"exploit`whoami`": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo bad"}}},
					"$(rm -rf /)": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo bad"}}},
					"test;ls": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo bad"}}},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Valid job should have markers
				validJob := wf.Jobs["valid_job"]
				if len(validJob.Steps) != 3 {
					t.Errorf("valid_job should have markers, got %d steps", len(validJob.Steps))
				}

				// Invalid jobs should NOT have markers (original step count preserved)
				for _, invalidID := range []string{"exploit`whoami`", "$(rm -rf /)", "test;ls"} {
					job := wf.Jobs[invalidID]
					if len(job.Steps) != 1 {
						t.Errorf("Job %q should NOT have markers (security), got %d steps", invalidID, len(job.Steps))
					}
				}

				// Manifest should only include valid job
				startStep := validJob.Steps[0]
				if !strings.Contains(startStep.Run, "valid_job") {
					t.Error("Manifest should include valid_job")
				}
				if strings.Contains(startStep.Run, "exploit") || strings.Contains(startStep.Run, "rm") {
					t.Error("Manifest should NOT include invalid job IDs")
				}
			},
		},
		{
			name: "job ID with spaces rejected",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"valid": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
					"job with spaces": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Job with spaces should not get markers
				spacesJob := wf.Jobs["job with spaces"]
				if len(spacesJob.Steps) != 1 {
					t.Errorf("Job with spaces should NOT have markers, got %d steps", len(spacesJob.Steps))
				}
			},
		},
		{
			name: "job ID starting with number rejected",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"valid_job": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
					"123invalid": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Job starting with number should not get markers
				numJob := wf.Jobs["123invalid"]
				if len(numJob.Steps) != 1 {
					t.Errorf("Job starting with number should NOT have markers, got %d steps", len(numJob.Steps))
				}
			},
		},
		{
			name: "valid job IDs with hyphens and underscores",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build-test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
					"_private": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
					"Test_Job-123": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// All these should be valid and get markers
				for jobID, job := range wf.Jobs {
					if len(job.Steps) != 3 {
						t.Errorf("Job %q should have markers, got %d steps", jobID, len(job.Steps))
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InjectJobMarkers(tt.workflow)
			tt.validate(t, tt.workflow)
		})
	}
}

// TestPrepareWorkflows tests the full workflow preparation pipeline
func TestPrepareWorkflows(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(*testing.T) (srcDir string, specificWorkflow string)
		wantErr          bool
		validateTmpDir   func(*testing.T, string)
		validateCleanup  func(*testing.T, string)
	}{
		{
			name: "prepare all workflows in directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				workflows := map[string]string{
					"ci.yml": `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
`,
					"release.yaml": `name: Release
on: release
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: echo deploy
`,
				}
				for file, content := range workflows {
					if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
						t.Fatalf("Failed to create workflow file: %v", err)
					}
				}
				return dir, ""
			},
			wantErr: false,
			validateTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
				// Verify temp directory exists
				if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
					t.Error("Temp directory should exist")
				}
				// Verify workflow files exist
				files, err := os.ReadDir(tmpDir)
				if err != nil {
					t.Fatalf("Failed to read temp directory: %v", err)
				}
				if len(files) != 2 {
					t.Errorf("Expected 2 files in temp directory, got %d", len(files))
				}
				// Verify continue-on-error, timeouts, and job markers are injected
				for _, file := range files {
					path := filepath.Join(tmpDir, file.Name())
					wf, err := ParseWorkflowFile(path)
					if err != nil {
						t.Errorf("Failed to parse prepared workflow: %v", err)
						continue
					}
					for jobName, job := range wf.Jobs {
						if job.ContinueOnError != true {
							t.Errorf("Job %s should have continue-on-error injected", jobName)
						}
						if job.TimeoutMinutes == nil {
							t.Errorf("Job %s should have timeout injected", jobName)
						}
						// Verify job markers are injected (first and last steps)
						if len(job.Steps) < 2 {
							t.Errorf("Job %s should have at least 2 steps (markers)", jobName)
							continue
						}
						firstStep := job.Steps[0]
						if firstStep.Name != "detent: job start" {
							t.Errorf("Job %s: first step should be job start marker", jobName)
						}
						lastStep := job.Steps[len(job.Steps)-1]
						if lastStep.Name != "detent: job end" {
							t.Errorf("Job %s: last step should be job end marker", jobName)
						}
					}
				}
			},
			validateCleanup: func(t *testing.T, tmpDir string) {
				t.Helper()
				if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
					t.Error("Temp directory should be removed after cleanup")
				}
			},
		},
		{
			name: "prepare specific workflow",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				workflows := map[string]string{
					"ci.yml":      "name: CI\non: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ci",
					"release.yml": "name: Release\non: push\njobs:\n  deploy:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo release",
				}
				for file, content := range workflows {
					if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
						t.Fatalf("Failed to create workflow file: %v", err)
					}
				}
				return dir, "ci.yml"
			},
			wantErr: false,
			validateTmpDir: func(t *testing.T, tmpDir string) {
				t.Helper()
				files, err := os.ReadDir(tmpDir)
				if err != nil {
					t.Fatalf("Failed to read temp directory: %v", err)
				}
				if len(files) != 1 {
					t.Errorf("Expected 1 file in temp directory, got %d", len(files))
					return
				}
				if files[0].Name() != "ci.yml" {
					t.Errorf("Expected ci.yml, got %s", files[0].Name())
				}
			},
		},
		{
			name: "no workflows found",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				// Create non-workflow files
				if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# README"), 0o600); err != nil {
					t.Fatalf("Failed to create readme: %v", err)
				}
				return dir, ""
			},
			wantErr: true,
		},
		{
			name: "invalid workflow YAML",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "invalid.yml"), []byte("invalid: [yaml"), 0o600); err != nil {
					t.Fatalf("Failed to create invalid workflow: %v", err)
				}
				return dir, ""
			},
			wantErr: true,
		},
		{
			name: "specific workflow not found",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				return dir, "nonexistent.yml"
			},
			wantErr: true,
		},
		{
			name: "specific workflow with absolute path",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "test.yml"), []byte("name: test\njobs:\n  test:\n    runs-on: ubuntu-latest"), 0o600); err != nil {
					t.Fatalf("Failed to create workflow: %v", err)
				}
				// Absolute paths should be rejected
				return dir, filepath.Join(dir, "test.yml")
			},
			wantErr: true,
		},
		{
			name: "specific workflow with parent directory reference",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				return dir, "../escape.yml"
			},
			wantErr: true,
		},
		{
			name: "specific workflow that is symlink",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				target := filepath.Join(dir, "target.yml")
				if err := os.WriteFile(target, []byte("name: target\njobs:\n  test:\n    runs-on: ubuntu-latest"), 0o600); err != nil {
					t.Fatalf("Failed to create target workflow: %v", err)
				}
				symlink := filepath.Join(dir, "symlink.yml")
				if err := os.Symlink(target, symlink); err != nil {
					t.Skip("Symlinks not supported on this system")
				}
				return dir, "symlink.yml"
			},
			wantErr: true,
		},
		{
			name: "specific workflow with invalid extension",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o600); err != nil {
					t.Fatalf("Failed to create file: %v", err)
				}
				return dir, "test.txt"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir, specificWorkflow := tt.setup(t)

			tmpDir, cleanup, err := PrepareWorkflows(srcDir, specificWorkflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("PrepareWorkflows() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tmpDir == "" {
					t.Error("tmpDir should not be empty")
				}
				if cleanup == nil {
					t.Error("cleanup function should not be nil")
				}

				if tt.validateTmpDir != nil {
					tt.validateTmpDir(t, tmpDir)
				}

				// Test cleanup
				cleanup()
				if tt.validateCleanup != nil {
					tt.validateCleanup(t, tmpDir)
				}
			}
		})
	}
}

// TestPrepareWorkflows_PathValidation tests path validation and security
func TestPrepareWorkflows_PathValidation(t *testing.T) {
	tests := []struct {
		name             string
		specificWorkflow string
		wantErr          bool
		errorSubstr      string
	}{
		{
			name:             "path with double dots",
			specificWorkflow: "../../../etc/passwd.yml",
			wantErr:          true,
			errorSubstr:      "parent directories",
		},
		{
			name:             "path starting with dot",
			specificWorkflow: "./workflow.yml",
			wantErr:          true,
			errorSubstr:      "parent directories",
		},
		{
			name:             "absolute path",
			specificWorkflow: "/etc/passwd.yml",
			wantErr:          true,
			errorSubstr:      "parent directories",
		},
		{
			name:             "path with embedded parent reference",
			specificWorkflow: "foo/../bar.yml",
			wantErr:          false, // filepath.Clean removes this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory with a valid workflow
			dir := t.TempDir()
			// Create nested structure to test path validation
			subdir := filepath.Join(dir, "foo")
			if err := os.MkdirAll(subdir, 0o750); err != nil {
				t.Fatalf("Failed to create subdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(subdir, "bar.yml"), []byte("name: test\njobs:\n  test:\n    runs-on: ubuntu-latest"), 0o600); err != nil {
				t.Fatalf("Failed to create workflow: %v", err)
			}
			// Also create bar.yml in root for the "foo/../bar.yml" test case
			if err := os.WriteFile(filepath.Join(dir, "bar.yml"), []byte("name: test\njobs:\n  test:\n    runs-on: ubuntu-latest"), 0o600); err != nil {
				t.Fatalf("Failed to create workflow: %v", err)
			}

			_, cleanup, err := PrepareWorkflows(dir, tt.specificWorkflow)
			if cleanup != nil {
				defer cleanup()
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("PrepareWorkflows() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errorSubstr != "" {
				if !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("Error should contain %q, got: %v", tt.errorSubstr, err)
				}
			}
		})
	}
}

// TestPrepareWorkflows_ErrorWrapping tests error message context
func TestPrepareWorkflows_ErrorWrapping(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T) (srcDir string, specificWorkflow string)
		errorSubstr string
	}{
		{
			name: "nonexistent specific workflow",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), "nonexistent.yml"
			},
			errorSubstr: "not found",
		},
		{
			name: "invalid YAML in specific workflow",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "invalid.yml"), []byte("invalid: ["), 0o600); err != nil {
					t.Fatalf("Failed to create invalid workflow: %v", err)
				}
				return dir, "invalid.yml"
			},
			errorSubstr: "parsing",
		},
		{
			name: "no workflows in directory",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				return t.TempDir(), ""
			},
			errorSubstr: "no workflow files found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir, specificWorkflow := tt.setup(t)
			_, cleanup, err := PrepareWorkflows(srcDir, specificWorkflow)
			if cleanup != nil {
				defer cleanup()
			}

			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.errorSubstr) {
				t.Errorf("Error should contain %q, got: %v", tt.errorSubstr, err)
			}
		})
	}
}

// TestPrepareWorkflows_CleanupOnError tests cleanup is performed on error
func TestPrepareWorkflows_CleanupOnError(t *testing.T) {
	dir := t.TempDir()

	// Create a workflow with invalid YAML
	invalidFile := filepath.Join(dir, "invalid.yml")
	if err := os.WriteFile(invalidFile, []byte("invalid: [yaml"), 0o600); err != nil {
		t.Fatalf("Failed to create invalid workflow: %v", err)
	}

	tmpDir, cleanup, err := PrepareWorkflows(dir, "")
	if err == nil {
		t.Fatal("Expected error for invalid YAML")
	}

	// Cleanup should be nil on error
	if cleanup != nil {
		t.Error("Cleanup function should be nil when PrepareWorkflows fails")
	}

	// Temp directory should not exist (cleaned up automatically on error)
	if tmpDir != "" {
		if _, statErr := os.Stat(tmpDir); !os.IsNotExist(statErr) {
			t.Error("Temp directory should be cleaned up on error")
		}
	}
}

// TestPrepareWorkflows_FilePermissions tests output file permissions
func TestPrepareWorkflows_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	workflow := `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
`
	if err := os.WriteFile(filepath.Join(dir, "test.yml"), []byte(workflow), 0o600); err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}

	tmpDir, cleanup, err := PrepareWorkflows(dir, "")
	if err != nil {
		t.Fatalf("PrepareWorkflows() failed: %v", err)
	}
	defer cleanup()

	// Check file permissions
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read temp directory: %v", err)
	}

	for _, file := range files {
		info, err := os.Stat(filepath.Join(tmpDir, file.Name()))
		if err != nil {
			t.Errorf("Failed to stat file %s: %v", file.Name(), err)
			continue
		}
		// File should have 0600 permissions
		if info.Mode().Perm() != 0o600 {
			t.Errorf("File %s has permissions %o, want 0600", file.Name(), info.Mode().Perm())
		}
	}
}

// timeoutEquals compares two timeout values of type any, handling type conversions
func timeoutEquals(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Convert both values to int64 for comparison
	var aInt, bInt int64
	switch v := a.(type) {
	case int:
		aInt = int64(v)
	case int64:
		aInt = v
	case int32:
		aInt = int64(v)
	case uint:
		aInt = int64(v)
	case uint64:
		aInt = int64(v)
	case uint32:
		aInt = int64(v)
	default:
		return false
	}

	switch v := b.(type) {
	case int:
		bInt = int64(v)
	case int64:
		bInt = v
	case int32:
		bInt = int64(v)
	case uint:
		bInt = int64(v)
	case uint64:
		bInt = int64(v)
	case uint32:
		bInt = int64(v)
	default:
		return false
	}

	return aInt == bInt
}

// TestPrepareWorkflows_ValidationErrors tests that unsupported features are rejected
func TestPrepareWorkflows_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		workflow    string
		wantErr     bool
		errorSubstr string
	}{
		{
			name: "unsupported runs-on macos",
			workflow: `name: Test
on: push
jobs:
  test:
    runs-on: macos-latest
    steps:
      - run: echo test
`,
			wantErr:     true,
			errorSubstr: "macos-latest",
		},
		{
			name: "unsupported runs-on windows",
			workflow: `name: Test
on: push
jobs:
  test:
    runs-on: windows-latest
    steps:
      - run: echo test
`,
			wantErr:     true,
			errorSubstr: "windows-latest",
		},
		{
			name: "reusable workflow",
			workflow: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./.github/workflows/reusable.yml
`,
			wantErr:     true,
			errorSubstr: "reusable workflow",
		},
		{
			name: "services warning allows execution",
			workflow: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
    steps:
      - run: echo test
`,
			wantErr: false, // Services is a warning, not an error - execution should continue
		},
		{
			name: "valid workflow",
			workflow: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: echo test
`,
			wantErr: false,
		},
		{
			name: "multiple unsupported features",
			workflow: `name: Test
on: push
jobs:
  build:
    runs-on: macos-latest
    steps:
      - run: echo build
  test:
    runs-on: windows-latest
    steps:
      - uses: ./.github/workflows/test.yml
`,
			wantErr:     true,
			errorSubstr: "unsupported features",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "test.yml"), []byte(tt.workflow), 0o600); err != nil {
				t.Fatalf("Failed to create workflow: %v", err)
			}

			tmpDir, cleanup, err := PrepareWorkflows(dir, "")
			if cleanup != nil {
				defer cleanup()
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("PrepareWorkflows() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errorSubstr != "" {
				if !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("Error should contain %q, got: %v", tt.errorSubstr, err)
				}
			}

			if !tt.wantErr && tmpDir == "" {
				t.Error("tmpDir should not be empty for valid workflow")
			}
		})
	}
}

// TestPrepareWorkflows_Integration tests the full integration with real workflow files
func TestPrepareWorkflows_Integration(t *testing.T) {
	dir := t.TempDir()

	// Create a complex workflow
	workflow := `name: Complex CI
on:
  push:
    branches: [main]
  pull_request:

env:
  GO_VERSION: '1.21'

jobs:
  build:
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - uses: actions/checkout@v3
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ env.GO_VERSION }}
      - name: Build
        run: go build -v ./...

  test:
    runs-on: ubuntu-latest
    needs: build
    steps:
      - uses: actions/checkout@v3
      - name: Test
        run: go test -v ./...
        timeout-minutes: 20

  lint:
    runs-on: ubuntu-latest
    continue-on-error: true
    steps:
      - uses: actions/checkout@v3
      - name: Lint
        run: golangci-lint run
`

	if err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(workflow), 0o600); err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}

	tmpDir, cleanup, err := PrepareWorkflows(dir, "")
	if err != nil {
		t.Fatalf("PrepareWorkflows() failed: %v", err)
	}
	defer cleanup()

	// Parse the prepared workflow
	preparedFile := filepath.Join(tmpDir, "ci.yml")
	wf, err := ParseWorkflowFile(preparedFile)
	if err != nil {
		t.Fatalf("Failed to parse prepared workflow: %v", err)
	}

	// Validate injections
	// Note: step counts include +2 for job marker steps (start + end)
	tests := []struct {
		jobName            string
		wantContinueOnErr  bool
		wantJobTimeout     bool
		originalTimeout    any
		stepCount          int
		stepWithTimeout    int  // index in original steps (before markers added)
		stepOriginalTimeout any
	}{
		{
			jobName:           "build",
			wantContinueOnErr: true,
			wantJobTimeout:    true,
			originalTimeout:   45, // Should be preserved
			stepCount:         5,  // 3 original + 2 markers
		},
		{
			jobName:            "test",
			wantContinueOnErr:  true,
			wantJobTimeout:     true,
			originalTimeout:    defaultJobTimeoutMinutes, // Should be injected
			stepCount:          4,  // 2 original + 2 markers
			stepWithTimeout:    2,  // index 2 (after start marker) is the "Test" step with timeout
			stepOriginalTimeout: 20, // Should be preserved
		},
		{
			jobName:           "lint",
			wantContinueOnErr: true, // Already true, should remain
			wantJobTimeout:    true,
			stepCount:         4, // 2 original + 2 markers
		},
	}

	for _, tt := range tests {
		t.Run(tt.jobName, func(t *testing.T) {
			job := wf.Jobs[tt.jobName]
			if job == nil {
				t.Fatalf("Job %s not found", tt.jobName)
			}

			// Check continue-on-error
			if job.ContinueOnError != tt.wantContinueOnErr {
				t.Errorf("Job %s: ContinueOnError = %v, want %v", tt.jobName, job.ContinueOnError, tt.wantContinueOnErr)
			}

			// Check timeout
			if tt.wantJobTimeout {
				if job.TimeoutMinutes == nil {
					t.Errorf("Job %s: TimeoutMinutes should be set", tt.jobName)
				} else if tt.originalTimeout != nil && !timeoutEquals(job.TimeoutMinutes, tt.originalTimeout) {
					t.Errorf("Job %s: TimeoutMinutes = %v, want %v", tt.jobName, job.TimeoutMinutes, tt.originalTimeout)
				}
			}

			// Check steps
			if len(job.Steps) != tt.stepCount {
				t.Errorf("Job %s: step count = %d, want %d", tt.jobName, len(job.Steps), tt.stepCount)
			}

			// Check step timeouts
			for i, step := range job.Steps {
				if step.TimeoutMinutes == nil {
					t.Errorf("Job %s, step %d: TimeoutMinutes should be set", tt.jobName, i)
				} else if i == tt.stepWithTimeout && tt.stepOriginalTimeout != nil {
					if !timeoutEquals(step.TimeoutMinutes, tt.stepOriginalTimeout) {
						t.Errorf("Job %s, step %d: TimeoutMinutes = %v, want %v", tt.jobName, i, step.TimeoutMinutes, tt.stepOriginalTimeout)
					}
				}
			}
		})
	}
}
