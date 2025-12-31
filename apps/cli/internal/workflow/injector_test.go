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
				// With step markers: manifest + job-start + step-0 + original + job-end = 5 steps
				if len(job.Steps) != 5 {
					t.Errorf("Expected 5 steps, got %d", len(job.Steps))
					return
				}

				// Check manifest step (first step of first job)
				manifestStep := job.Steps[0]
				if manifestStep.Name != "detent: manifest" {
					t.Errorf("First step name = %q, want %q", manifestStep.Name, "detent: manifest")
				}
				if !strings.Contains(manifestStep.Run, "::detent::manifest::v2::") {
					t.Error("Manifest step should contain v2 manifest marker")
				}

				// Check job-start step
				startStep := job.Steps[1]
				if startStep.Name != "detent: job start" {
					t.Errorf("Second step name = %q, want %q", startStep.Name, "detent: job start")
				}
				if !strings.Contains(startStep.Run, "::detent::job-start::build") {
					t.Error("Job start step should contain job-start marker")
				}

				// Check step marker
				stepMarker := job.Steps[2]
				if stepMarker.Name != "detent: step 0" {
					t.Errorf("Third step name = %q, want %q", stepMarker.Name, "detent: step 0")
				}
				if !strings.Contains(stepMarker.Run, "::detent::step-start::build::0::") {
					t.Error("Step marker should contain step-start marker")
				}

				// Check original step is preserved
				if job.Steps[3].Run != "echo test" {
					t.Errorf("Fourth step should be original, got %q", job.Steps[3].Run)
				}

				// Check end marker
				endStep := job.Steps[4]
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
				// alpha (first alphabetically) gets manifest: 5 steps
				// beta, zebra: 4 steps each (no manifest step)
				expectedSteps := map[string]int{
					"alpha": 5, // manifest + job-start + step-0 + original + job-end
					"beta":  4, // job-start + step-0 + original + job-end
					"zebra": 4,
				}

				for jobID, job := range wf.Jobs {
					expected := expectedSteps[jobID]
					if len(job.Steps) != expected {
						t.Errorf("Job %s: expected %d steps, got %d", jobID, expected, len(job.Steps))
						continue
					}
				}

				// Manifest is only in alpha (first alphabetically)
				alphaJob := wf.Jobs["alpha"]
				manifestStep := alphaJob.Steps[0]
				if !strings.Contains(manifestStep.Run, "::detent::manifest::v2::") {
					t.Errorf("Alpha should have manifest step, got %q", manifestStep.Run)
				}
				// Manifest should contain all jobs in JSON format
				if !strings.Contains(manifestStep.Run, `"id":"alpha"`) {
					t.Error("Manifest should contain alpha job")
				}
				if !strings.Contains(manifestStep.Run, `"id":"beta"`) {
					t.Error("Manifest should contain beta job")
				}
				if !strings.Contains(manifestStep.Run, `"id":"zebra"`) {
					t.Error("Manifest should contain zebra job")
				}

				// Each job should have its own job-start marker
				for jobID, job := range wf.Jobs {
					jobStartIdx := 0
					if jobID == "alpha" {
						jobStartIdx = 1 // After manifest
					}
					startStep := job.Steps[jobStartIdx]
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
				// Regular job should have markers (5 steps: manifest + job-start + step-0 + original + job-end)
				regularJob := wf.Jobs["regular"]
				if len(regularJob.Steps) != 5 {
					t.Errorf("Regular job: expected 5 steps, got %d", len(regularJob.Steps))
				}

				// Reusable job should be unchanged (no Steps)
				reusableJob := wf.Jobs["reusable"]
				if reusableJob.Steps != nil {
					t.Error("Reusable job should not have steps added")
				}

				// Manifest should include both jobs (v2 JSON format)
				manifestStep := regularJob.Steps[0]
				if !strings.Contains(manifestStep.Run, `"id":"regular"`) {
					t.Error("Manifest should include regular job")
				}
				if !strings.Contains(manifestStep.Run, `"id":"reusable"`) {
					t.Error("Manifest should include reusable job")
				}
				// Reusable job should have "uses" field in manifest
				if !strings.Contains(manifestStep.Run, `"uses":`) {
					t.Error("Manifest should include reusable workflow uses field")
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
				// Should add manifest, start and end markers even with empty steps
				// manifest + job-start + job-end = 3 steps
				if len(job.Steps) != 3 {
					t.Errorf("Expected 3 steps (manifest + markers), got %d", len(job.Steps))
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
				// valid is first valid job alphabetically, gets manifest
				validJob := wf.Jobs["valid"]
				if len(validJob.Steps) != 5 {
					t.Errorf("Valid job should have markers, got %d steps", len(validJob.Steps))
				}
			},
		},
		{
			name: "skip invalid job IDs - shell injection attempts",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"valid_job":       {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo valid"}}},
					"exploit`whoami`": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo bad"}}},
					"$(rm -rf /)":     {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo bad"}}},
					"test;ls":         {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo bad"}}},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Valid job should have markers (5 steps: manifest + job-start + step-0 + original + job-end)
				validJob := wf.Jobs["valid_job"]
				if len(validJob.Steps) != 5 {
					t.Errorf("valid_job should have markers, got %d steps", len(validJob.Steps))
				}

				// Invalid jobs should NOT have markers (original step count preserved)
				for _, invalidID := range []string{"exploit`whoami`", "$(rm -rf /)", "test;ls"} {
					job := wf.Jobs[invalidID]
					if len(job.Steps) != 1 {
						t.Errorf("Job %q should NOT have markers (security), got %d steps", invalidID, len(job.Steps))
					}
				}

				// Manifest should only include valid job (v2 JSON format)
				manifestStep := validJob.Steps[0]
				if !strings.Contains(manifestStep.Run, `"id":"valid_job"`) {
					t.Error("Manifest should include valid_job")
				}
				if strings.Contains(manifestStep.Run, "exploit") || strings.Contains(manifestStep.Run, "rm") {
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
					"build-test":   {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
					"_private":     {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
					"Test_Job-123": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "echo"}}},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// All these should be valid and get markers
				// First job alphabetically gets manifest (5 steps), others get 4 steps
				// Sort order: "Test_Job-123" < "_private" < "build-test"
				expectedSteps := map[string]int{
					"Test_Job-123": 5, // First alphabetically, gets manifest
					"_private":     4,
					"build-test":   4,
				}
				for jobID, job := range wf.Jobs {
					expected := expectedSteps[jobID]
					if len(job.Steps) != expected {
						t.Errorf("Job %q should have %d steps, got %d", jobID, expected, len(job.Steps))
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
						// Verify job markers are injected (first step is either manifest or job start)
						if len(job.Steps) < 3 {
							t.Errorf("Job %s should have at least 3 steps (markers)", jobName)
							continue
						}
						firstStep := job.Steps[0]
						// First step should be either manifest or job start marker
						if firstStep.Name != "detent: manifest" && firstStep.Name != "detent: job start" {
							t.Errorf("Job %s: first step should be manifest or job start marker, got %q", jobName, firstStep.Name)
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
	// Note: step counts include manifest (first job), job-start, step markers, original steps, and job-end
	// For N original steps: first job has 1 + 1 + 2N + 1 = 2N + 3 steps, others have 1 + 2N + 1 = 2N + 2 steps
	tests := []struct {
		jobName             string
		wantContinueOnErr   bool
		wantJobTimeout      bool
		originalTimeout     any
		stepCount           int
		stepWithTimeout     int // index in prepared steps
		stepOriginalTimeout any
	}{
		{
			jobName:           "build",
			wantContinueOnErr: true,
			wantJobTimeout:    true,
			originalTimeout:   45,               // Should be preserved
			stepCount:         9,                // 3 original + manifest + job-start + 3 step-markers + job-end = 9
		},
		{
			jobName:             "test",
			wantContinueOnErr:   true,
			wantJobTimeout:      true,
			originalTimeout:     defaultJobTimeoutMinutes, // Should be injected
			stepCount:           6,                        // 2 original + job-start + 2 step-markers + job-end = 6
			stepWithTimeout:     4,                        // job-start(0), step-0(1), checkout(2), step-1(3), test(4), job-end(5)
			stepOriginalTimeout: 20,                       // Should be preserved
		},
		{
			jobName:           "lint",
			wantContinueOnErr: true, // Already true, should remain
			wantJobTimeout:    true,
			stepCount:         6, // 2 original + job-start + 2 step-markers + job-end = 6
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

// TestBuildCombinedManifest tests building a combined manifest from multiple workflows.
func TestBuildCombinedManifest(t *testing.T) {
	tests := []struct {
		name      string
		workflows map[string]*Workflow
		wantJobs  []string // Expected job IDs in order
	}{
		{
			name:      "empty workflows",
			workflows: map[string]*Workflow{},
			wantJobs:  []string{},
		},
		{
			name: "single workflow",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"lint": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm run lint"}}},
						"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm test"}}},
					},
				},
			},
			wantJobs: []string{"lint", "test"},
		},
		{
			name: "multiple workflows - jobs combined",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"lint": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm run lint"}}},
						"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm test"}}},
					},
				},
				"/path/to/release.yml": {
					Jobs: map[string]*Job{
						"release": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm publish"}}},
					},
				},
			},
			wantJobs: []string{"lint", "release", "test"}, // Alphabetically sorted
		},
		{
			name: "reusable workflow jobs included",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "go build"}}},
						"deploy": {Uses: "org/repo/.github/workflows/deploy.yml@main"},
					},
				},
			},
			wantJobs: []string{"build", "deploy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := BuildCombinedManifest(tt.workflows)

			if manifest.Version != 2 {
				t.Errorf("Version = %d, want 2", manifest.Version)
			}

			if len(manifest.Jobs) != len(tt.wantJobs) {
				t.Errorf("Job count = %d, want %d", len(manifest.Jobs), len(tt.wantJobs))
				return
			}

			gotIDs := make([]string, len(manifest.Jobs))
			for i, job := range manifest.Jobs {
				gotIDs[i] = job.ID
			}

			for i, wantID := range tt.wantJobs {
				if gotIDs[i] != wantID {
					t.Errorf("Job[%d].ID = %q, want %q", i, gotIDs[i], wantID)
				}
			}
		})
	}
}

// TestFindFirstJobAcrossWorkflows tests finding the first job across multiple workflows.
func TestFindFirstJobAcrossWorkflows(t *testing.T) {
	tests := []struct {
		name      string
		workflows map[string]*Workflow
		wantPath  string
		wantJobID string
	}{
		{
			name:      "empty workflows",
			workflows: map[string]*Workflow{},
			wantPath:  "",
			wantJobID: "",
		},
		{
			name: "single workflow with jobs - no dependencies",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "test"}}},
						"lint": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "lint"}}},
					},
				},
			},
			wantPath:  "/path/to/ci.yml",
			wantJobID: "lint", // Alphabetically first among jobs without needs
		},
		{
			name: "prefer job without needs over alphabetically earlier job with needs",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"build": {RunsOn: "ubuntu-latest", Needs: []any{"lint", "test"}, Steps: []*Step{{Run: "build"}}},
						"lint":  {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "lint"}}},
						"test":  {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "test"}}},
					},
				},
			},
			wantPath:  "/path/to/ci.yml",
			wantJobID: "lint", // lint has no needs, build does
		},
		{
			name: "prefer job without needs - string needs format",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"deploy": {RunsOn: "ubuntu-latest", Needs: "build", Steps: []*Step{{Run: "deploy"}}},
						"build":  {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "build"}}},
					},
				},
			},
			wantPath:  "/path/to/ci.yml",
			wantJobID: "build", // build has no needs
		},
		{
			name: "multiple workflows - prefer workflow with job without needs",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"build": {RunsOn: "ubuntu-latest", Needs: []any{"lint"}, Steps: []*Step{{Run: "build"}}},
					},
				},
				"/path/to/release.yml": {
					Jobs: map[string]*Job{
						"release": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "release"}}},
					},
				},
			},
			wantPath:  "/path/to/release.yml",
			wantJobID: "release", // release has no needs, build does
		},
		{
			name: "skip reusable workflow jobs",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"deploy": {Uses: "org/repo/.github/workflows/deploy.yml@main"},
						"build":  {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "build"}}},
					},
				},
			},
			wantPath:  "/path/to/ci.yml",
			wantJobID: "build", // deploy skipped because it's a reusable workflow
		},
		{
			name: "fallback to job with needs if all have needs",
			workflows: map[string]*Workflow{
				"/path/to/ci.yml": {
					Jobs: map[string]*Job{
						"deploy": {RunsOn: "ubuntu-latest", Needs: []any{"build"}, Steps: []*Step{{Run: "deploy"}}},
						"build":  {RunsOn: "ubuntu-latest", Needs: []any{"test"}, Steps: []*Step{{Run: "build"}}},
					},
				},
			},
			wantPath:  "/path/to/ci.yml",
			wantJobID: "build", // Alphabetically first fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotJobID := findFirstJobAcrossWorkflows(tt.workflows)

			if gotPath != tt.wantPath {
				t.Errorf("workflowPath = %q, want %q", gotPath, tt.wantPath)
			}
			if gotJobID != tt.wantJobID {
				t.Errorf("jobID = %q, want %q", gotJobID, tt.wantJobID)
			}
		})
	}
}

// TestInjectJobMarkersWithManifest tests injecting markers with an external manifest.
func TestInjectJobMarkersWithManifest(t *testing.T) {
	t.Run("inject manifest to designated job only", func(t *testing.T) {
		wf := &Workflow{
			Jobs: map[string]*Job{
				"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Name: "Build", Run: "go build"}}},
				"test":  {RunsOn: "ubuntu-latest", Steps: []*Step{{Name: "Test", Run: "go test"}}},
			},
		}

		manifestJSON := []byte(`{"v":2,"jobs":[{"id":"build"},{"id":"test"}]}`)

		// Inject with build as manifest job
		InjectJobMarkersWithManifest(wf, manifestJSON, "build")

		// Build job should have manifest step
		buildSteps := wf.Jobs["build"].Steps
		hasManifest := false
		for _, step := range buildSteps {
			if step.Name == "detent: manifest" {
				hasManifest = true
				break
			}
		}
		if !hasManifest {
			t.Error("build job should have manifest step")
		}

		// Test job should NOT have manifest step
		testSteps := wf.Jobs["test"].Steps
		for _, step := range testSteps {
			if step.Name == "detent: manifest" {
				t.Error("test job should NOT have manifest step")
			}
		}
	})

	t.Run("nil manifest skips manifest injection", func(t *testing.T) {
		wf := &Workflow{
			Jobs: map[string]*Job{
				"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Name: "Build", Run: "go build"}}},
			},
		}

		// Inject with nil manifest
		InjectJobMarkersWithManifest(wf, nil, "")

		// Build job should NOT have manifest step
		for _, step := range wf.Jobs["build"].Steps {
			if step.Name == "detent: manifest" {
				t.Error("build job should NOT have manifest step when manifestJSON is nil")
			}
		}

		// But should have job-start and job-end markers
		hasJobStart := false
		hasJobEnd := false
		for _, step := range wf.Jobs["build"].Steps {
			if step.Name == "detent: job start" {
				hasJobStart = true
			}
			if step.Name == "detent: job end" {
				hasJobEnd = true
			}
		}
		if !hasJobStart {
			t.Error("build job should have job-start marker")
		}
		if !hasJobEnd {
			t.Error("build job should have job-end marker")
		}
	})
}
