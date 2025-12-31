package workflow

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/detent/cli/internal/ci"
)

// extractManifestJSON extracts the JSON content from a base64-encoded manifest marker.
// The marker format is: echo '::detent::manifest::v2::b64::{base64}'
// Returns the decoded JSON string or empty string if not found or invalid.
func extractManifestJSON(manifestRun string) string {
	const prefix = "::detent::manifest::v2::b64::"
	idx := strings.Index(manifestRun, prefix)
	if idx < 0 {
		return ""
	}
	encoded := manifestRun[idx+len(prefix):]
	// Remove trailing single quote and anything after
	if quoteIdx := strings.Index(encoded, "'"); quoteIdx >= 0 {
		encoded = encoded[:quoteIdx]
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return string(decoded)
}

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
				// Manifest should contain all jobs in JSON format (decode base64 first)
				manifestJSON := extractManifestJSON(manifestStep.Run)
				if !strings.Contains(manifestJSON, `"id":"alpha"`) {
					t.Error("Manifest should contain alpha job")
				}
				if !strings.Contains(manifestJSON, `"id":"beta"`) {
					t.Error("Manifest should contain beta job")
				}
				if !strings.Contains(manifestJSON, `"id":"zebra"`) {
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

				// Manifest should include both jobs (v2 JSON format, decode base64 first)
				manifestStep := regularJob.Steps[0]
				manifestJSON := extractManifestJSON(manifestStep.Run)
				if !strings.Contains(manifestJSON, `"id":"regular"`) {
					t.Error("Manifest should include regular job")
				}
				if !strings.Contains(manifestJSON, `"id":"reusable"`) {
					t.Error("Manifest should include reusable job")
				}
				// Reusable job should have "uses" field in manifest
				if !strings.Contains(manifestJSON, `"uses":`) {
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

				// Manifest should only include valid job (v2 JSON format, decode base64 first)
				manifestStep := validJob.Steps[0]
				manifestJSON := extractManifestJSON(manifestStep.Run)
				if !strings.Contains(manifestJSON, `"id":"valid_job"`) {
					t.Error("Manifest should include valid_job")
				}
				if strings.Contains(manifestJSON, "exploit") || strings.Contains(manifestJSON, "rm") {
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

			tmpDir, cleanup, err := PrepareWorkflows(srcDir, specificWorkflow, nil)
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

			_, cleanup, err := PrepareWorkflows(dir, tt.specificWorkflow, nil)
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
			_, cleanup, err := PrepareWorkflows(srcDir, specificWorkflow, nil)
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

	tmpDir, cleanup, err := PrepareWorkflows(dir, "", nil)
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

	tmpDir, cleanup, err := PrepareWorkflows(dir, "", nil)
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

			tmpDir, cleanup, err := PrepareWorkflows(dir, "", nil)
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

	tmpDir, cleanup, err := PrepareWorkflows(dir, "", nil)
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

// TestIsSensitiveJob tests detection of sensitive jobs that should not get if: always()
func TestIsSensitiveJob(t *testing.T) {
	tests := []struct {
		name      string
		jobID     string
		job       *Job
		sensitive bool
	}{
		// Job name detection
		{
			name:      "release job by name",
			jobID:     "release",
			job:       &Job{Name: "Release", Steps: []*Step{{Run: "echo test"}}},
			sensitive: true,
		},
		{
			name:      "publish job by name",
			jobID:     "publish",
			job:       &Job{Steps: []*Step{{Run: "echo test"}}},
			sensitive: true,
		},
		{
			name:      "deploy job by ID",
			jobID:     "deploy-prod",
			job:       &Job{Steps: []*Step{{Run: "echo test"}}},
			sensitive: true,
		},
		{
			name:      "production job",
			jobID:     "production-deploy",
			job:       &Job{Steps: []*Step{{Run: "echo test"}}},
			sensitive: true,
		},
		{
			name:      "staging job",
			jobID:     "staging",
			job:       &Job{Steps: []*Step{{Run: "echo test"}}},
			sensitive: true,
		},
		{
			name:      "ship job",
			jobID:     "ship-to-users",
			job:       &Job{Steps: []*Step{{Run: "echo test"}}},
			sensitive: true,
		},
		{
			name:      "upload job",
			jobID:     "upload-artifacts",
			job:       &Job{Steps: []*Step{{Run: "echo test"}}},
			sensitive: true,
		},
		// Action detection
		{
			name:      "changesets action",
			jobID:     "version",
			job:       &Job{Steps: []*Step{{Uses: "changesets/action@v1"}}},
			sensitive: true,
		},
		{
			name:      "goreleaser action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "goreleaser/goreleaser-action@v5"}}},
			sensitive: true,
		},
		{
			name:      "docker build-push action",
			jobID:     "build-image",
			job:       &Job{Steps: []*Step{{Uses: "docker/build-push-action@v5"}}},
			sensitive: true,
		},
		{
			name:      "docker login action",
			jobID:     "docker-build",
			job:       &Job{Steps: []*Step{{Uses: "docker/login-action@v3"}}},
			sensitive: true,
		},
		{
			name:      "pypi publish action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "pypa/gh-action-pypi-publish@v1"}}},
			sensitive: true,
		},
		{
			name:      "github pages deploy action",
			jobID:     "docs",
			job:       &Job{Steps: []*Step{{Uses: "JamesIves/github-pages-deploy-action@v4"}}},
			sensitive: true,
		},
		{
			name:      "generic deploy action pattern",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "some-org/custom-deploy@v1"}}},
			sensitive: true,
		},
		{
			name:      "generic publish action pattern",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "some-org/npm-publish@v1"}}},
			sensitive: true,
		},
		{
			name:      "generic release action pattern",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "some-org/create-release@v1"}}},
			sensitive: true,
		},
		// Command detection
		{
			name:      "npm publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "npm publish --access public"}}},
			sensitive: true,
		},
		{
			name:      "yarn publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "yarn publish"}}},
			sensitive: true,
		},
		{
			name:      "pnpm publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "pnpm publish --no-git-checks"}}},
			sensitive: true,
		},
		{
			name:      "docker push command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "docker push myimage:latest"}}},
			sensitive: true,
		},
		{
			name:      "docker buildx push command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "docker buildx push myimage"}}},
			sensitive: true,
		},
		{
			name:      "git push tags command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "git push --tags"}}},
			sensitive: true,
		},
		{
			name:      "kubectl apply command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "kubectl apply -f deployment.yaml"}}},
			sensitive: true,
		},
		{
			name:      "helm upgrade command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "helm upgrade myrelease ./chart"}}},
			sensitive: true,
		},
		{
			name:      "terraform apply command",
			jobID:     "infra",
			job:       &Job{Steps: []*Step{{Run: "terraform apply -auto-approve"}}},
			sensitive: true,
		},
		{
			name:      "vercel prod command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "vercel --prod"}}},
			sensitive: true,
		},
		{
			name:      "cargo publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "cargo publish"}}},
			sensitive: true,
		},
		{
			name:      "twine upload command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "twine upload dist/*"}}},
			sensitive: true,
		},
		{
			name:      "mvn deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "mvn deploy"}}},
			sensitive: true,
		},
		{
			name:      "gradle publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "gradle publish"}}},
			sensitive: true,
		},
		{
			name:      "aws s3 sync command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "aws s3 sync ./dist s3://mybucket"}}},
			sensitive: true,
		},
		{
			name:      "gcloud app deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "gcloud app deploy"}}},
			sensitive: true,
		},
		{
			name:      "gh release create command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "gh release create v1.0.0"}}},
			sensitive: true,
		},
		// Safe jobs
		{
			name:      "safe test job",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Run: "go test ./..."}}},
			sensitive: false,
		},
		{
			name:      "safe build job",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "go build ./..."}}},
			sensitive: false,
		},
		{
			name:      "safe lint job",
			jobID:     "lint",
			job:       &Job{Steps: []*Step{{Run: "golangci-lint run"}}},
			sensitive: false,
		},
		{
			name:      "safe checkout action",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Uses: "actions/checkout@v4"}}},
			sensitive: false,
		},
		{
			name:      "safe setup-go action",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Uses: "actions/setup-go@v5"}}},
			sensitive: false,
		},
		// === Destructive Operations ===
		// Terraform destroy
		{
			name:      "terraform destroy command",
			jobID:     "cleanup",
			job:       &Job{Steps: []*Step{{Run: "terraform destroy -auto-approve"}}},
			sensitive: true,
		},
		// kubectl delete
		{
			name:      "kubectl delete command",
			jobID:     "cleanup",
			job:       &Job{Steps: []*Step{{Run: "kubectl delete -f deployment.yaml"}}},
			sensitive: true,
		},
		{
			name:      "kubectl delete pods command",
			jobID:     "maintenance",
			job:       &Job{Steps: []*Step{{Run: "kubectl delete pods --all"}}},
			sensitive: true,
		},
		// helm uninstall
		{
			name:      "helm uninstall command",
			jobID:     "teardown",
			job:       &Job{Steps: []*Step{{Run: "helm uninstall myrelease"}}},
			sensitive: true,
		},
		{
			name:      "helm delete command",
			jobID:     "teardown",
			job:       &Job{Steps: []*Step{{Run: "helm delete myrelease"}}},
			sensitive: true,
		},
		// pulumi destroy
		{
			name:      "pulumi destroy command",
			jobID:     "cleanup",
			job:       &Job{Steps: []*Step{{Run: "pulumi destroy --yes"}}},
			sensitive: true,
		},
		// Database migrations - prisma
		{
			name:      "prisma migrate deploy command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "prisma migrate deploy"}}},
			sensitive: true,
		},
		{
			name:      "prisma db push command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "prisma db push"}}},
			sensitive: true,
		},
		{
			name:      "prisma migrate reset command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "prisma migrate reset --force"}}},
			sensitive: true,
		},
		// Database migrations - alembic
		{
			name:      "alembic upgrade command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "alembic upgrade head"}}},
			sensitive: true,
		},
		{
			name:      "alembic downgrade command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "alembic downgrade -1"}}},
			sensitive: true,
		},
		// Database migrations - flyway
		{
			name:      "flyway migrate command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "flyway migrate"}}},
			sensitive: true,
		},
		{
			name:      "flyway repair command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "flyway repair"}}},
			sensitive: true,
		},
		// === Cloud Platforms ===
		// Railway
		{
			name:      "railway deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "railway deploy"}}},
			sensitive: true,
		},
		{
			name:      "railway up command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "railway up"}}},
			sensitive: true,
		},
		{
			name:      "railway action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "railwayapp/railway-action@v1"}}},
			sensitive: true,
		},
		// Fly.io
		{
			name:      "flyctl deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "flyctl deploy"}}},
			sensitive: true,
		},
		{
			name:      "fly deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "fly deploy --remote-only"}}},
			sensitive: true,
		},
		{
			name:      "fly launch command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "fly launch --now"}}},
			sensitive: true,
		},
		{
			name:      "flyctl action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "superfly/flyctl-actions@v1"}}},
			sensitive: true,
		},
		// Heroku
		{
			name:      "heroku deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "heroku deploy"}}},
			sensitive: true,
		},
		{
			name:      "heroku container release command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "heroku container:release web"}}},
			sensitive: true,
		},
		{
			name:      "git push heroku command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "git push heroku main"}}},
			sensitive: true,
		},
		{
			name:      "heroku deploy action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "akhileshns/heroku-deploy@v3"}}},
			sensitive: true,
		},
		// Cloudflare Workers
		{
			name:      "wrangler deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "wrangler deploy"}}},
			sensitive: true,
		},
		{
			name:      "wrangler publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "wrangler publish"}}},
			sensitive: true,
		},
		{
			name:      "npx wrangler deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "npx wrangler deploy"}}},
			sensitive: true,
		},
		{
			name:      "wrangler action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "cloudflare/wrangler-action@v3"}}},
			sensitive: true,
		},
		// Firebase
		{
			name:      "firebase deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "firebase deploy"}}},
			sensitive: true,
		},
		{
			name:      "firebase hosting channel deploy command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "firebase hosting:channel:deploy preview"}}},
			sensitive: true,
		},
		{
			name:      "firebase action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "w9jds/firebase-action@v12"}}},
			sensitive: true,
		},
		{
			name:      "firebase hosting deploy action",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Uses: "FirebaseExtended/action-hosting-deploy@v0"}}},
			sensitive: true,
		},
		// === Package Managers ===
		// Ruby - gem push
		{
			name:      "gem push command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "gem push mypackage-1.0.0.gem"}}},
			sensitive: true,
		},
		{
			name:      "gem release command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "gem release"}}},
			sensitive: true,
		},
		{
			name:      "rake release command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "rake release"}}},
			sensitive: true,
		},
		{
			name:      "bundle exec rake release command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "bundle exec rake release"}}},
			sensitive: true,
		},
		// .NET - dotnet nuget push
		{
			name:      "dotnet nuget push command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "dotnet nuget push *.nupkg"}}},
			sensitive: true,
		},
		{
			name:      "nuget push command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "nuget push MyPackage.1.0.0.nupkg"}}},
			sensitive: true,
		},
		// Python - poetry publish
		{
			name:      "poetry publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "poetry publish --build"}}},
			sensitive: true,
		},
		{
			name:      "twine upload command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "twine upload dist/*"}}},
			sensitive: true,
		},
		// Rust - cargo publish
		{
			name:      "cargo publish command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "cargo publish --token $CARGO_TOKEN"}}},
			sensitive: true,
		},
		// === Verify NO false positives for safe jobs ===
		{
			name:      "safe test job with go test command",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Run: "go test -v ./..."}}},
			sensitive: false,
		},
		{
			name:      "safe build job with go build command",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "go build -o ./bin/app ./cmd/app"}}},
			sensitive: false,
		},
		{
			name:      "safe lint job with golangci-lint command",
			jobID:     "lint",
			job:       &Job{Steps: []*Step{{Run: "golangci-lint run ./..."}}},
			sensitive: false,
		},
		{
			name:      "safe typecheck job with tsc command",
			jobID:     "typecheck",
			job:       &Job{Steps: []*Step{{Run: "tsc --noEmit"}}},
			sensitive: false,
		},
		{
			name:      "safe format job with prettier command",
			jobID:     "format",
			job:       &Job{Steps: []*Step{{Run: "prettier --check src/"}}},
			sensitive: false,
		},
		{
			name:      "safe test job with npm test command",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Run: "npm test"}}},
			sensitive: false,
		},
		{
			name:      "safe test job with pytest command",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Run: "pytest tests/"}}},
			sensitive: false,
		},
		{
			name:      "safe build job with docker build (no push)",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "docker build -t myapp:test ."}}},
			sensitive: false,
		},
		{
			name:      "safe terraform plan job",
			jobID:     "plan",
			job:       &Job{Steps: []*Step{{Run: "terraform plan -out=tfplan"}}},
			sensitive: false,
		},
		{
			name:      "safe kubectl get job",
			jobID:     "check",
			job:       &Job{Steps: []*Step{{Run: "kubectl get pods"}}},
			sensitive: false,
		},
		{
			name:      "safe helm lint job",
			jobID:     "lint",
			job:       &Job{Steps: []*Step{{Run: "helm lint ./chart"}}},
			sensitive: false,
		},
		{
			name:      "safe cargo test job",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Run: "cargo test --all"}}},
			sensitive: false,
		},
		{
			name:      "safe cargo build job",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "cargo build --release"}}},
			sensitive: false,
		},
		{
			name:      "safe dotnet build job",
			jobID:     "build",
			job:       &Job{Steps: []*Step{{Run: "dotnet build --configuration Release"}}},
			sensitive: false,
		},
		{
			name:      "safe dotnet test job",
			jobID:     "test",
			job:       &Job{Steps: []*Step{{Run: "dotnet test"}}},
			sensitive: false,
		},
		{
			name:      "safe poetry install job",
			jobID:     "setup",
			job:       &Job{Steps: []*Step{{Run: "poetry install"}}},
			sensitive: false,
		},
		{
			name:      "safe prisma generate job",
			jobID:     "setup",
			job:       &Job{Steps: []*Step{{Run: "prisma generate"}}},
			sensitive: false,
		},
		{
			name:      "safe prisma validate job",
			jobID:     "validate",
			job:       &Job{Steps: []*Step{{Run: "prisma validate"}}},
			sensitive: false,
		},
		// === Additional edge cases ===
		{
			name:      "liquibase update command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "liquibase update"}}},
			sensitive: true,
		},
		{
			name:      "knex migrate latest command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "knex migrate:latest"}}},
			sensitive: true,
		},
		{
			name:      "sequelize db migrate command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "sequelize db:migrate"}}},
			sensitive: true,
		},
		{
			name:      "typeorm migration run command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "typeorm migration:run"}}},
			sensitive: true,
		},
		{
			name:      "goose up command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "goose up"}}},
			sensitive: true,
		},
		{
			name:      "dbmate up command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "dbmate up"}}},
			sensitive: true,
		},
		{
			name:      "atlas migrate apply command",
			jobID:     "db",
			job:       &Job{Steps: []*Step{{Run: "atlas migrate apply"}}},
			sensitive: true,
		},
		{
			name:      "nil job",
			jobID:     "test",
			job:       nil,
			sensitive: false,
		},
		{
			name:      "job with nil steps",
			jobID:     "test",
			job:       &Job{Steps: nil},
			sensitive: false,
		},
		{
			name:      "job with nil step",
			jobID:     "test",
			job:       &Job{Steps: []*Step{nil, {Run: "echo test"}}},
			sensitive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSensitiveJob(tt.jobID, tt.job)
			if got != tt.sensitive {
				t.Errorf("isSensitiveJob(%q) = %v, want %v", tt.jobID, got, tt.sensitive)
			}
		})
	}
}

// TestInjectAlwaysForDependentJobs tests injecting if: always() for jobs with dependencies
func TestInjectAlwaysForDependentJobs(t *testing.T) {
	tests := []struct {
		name     string
		workflow *Workflow
		validate func(*testing.T, *Workflow)
	}{
		{
			name: "inject always() for safe job with single dependency",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"lint": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "lint"}}},
					"build": {
						RunsOn: "ubuntu-latest",
						Needs:  "lint",
						Steps:  []*Step{{Run: "go build"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["build"].If != "always()" {
					t.Errorf("build should have if: always(), got %q", wf.Jobs["build"].If)
				}
				if wf.Jobs["lint"].If != "" {
					t.Errorf("lint should not have if condition, got %q", wf.Jobs["lint"].If)
				}
			},
		},
		{
			name: "inject always() for safe job with multiple dependencies",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"lint":  {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "lint"}}},
					"test":  {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "test"}}},
					"build": {
						RunsOn: "ubuntu-latest",
						Needs:  []any{"lint", "test"},
						Steps:  []*Step{{Run: "go build"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["build"].If != "always()" {
					t.Errorf("build should have if: always(), got %q", wf.Jobs["build"].If)
				}
			},
		},
		{
			name: "preserve and combine existing if condition",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"lint": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "lint"}}},
					"test": {
						RunsOn: "ubuntu-latest",
						Needs:  "lint",
						If:     "github.event_name == 'push'",
						Steps:  []*Step{{Run: "test"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				expected := "always() && (github.event_name == 'push')"
				if wf.Jobs["test"].If != expected {
					t.Errorf("test if = %q, want %q", wf.Jobs["test"].If, expected)
				}
			},
		},
		{
			name: "skip jobs without dependencies",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"standalone": {
						RunsOn: "ubuntu-latest",
						Steps:  []*Step{{Run: "test"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["standalone"].If != "" {
					t.Errorf("standalone should not have if condition, got %q", wf.Jobs["standalone"].If)
				}
			},
		},
		{
			name: "skip reusable workflows",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "build"}}},
					"reusable": {
						Uses:  "./.github/workflows/reusable.yml",
						Needs: "build",
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["reusable"].If != "" {
					t.Errorf("reusable workflow should not get if condition, got %q", wf.Jobs["reusable"].If)
				}
			},
		},
		{
			name: "skip release job even with dependency",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "test"}}},
					"release": {
						RunsOn: "ubuntu-latest",
						Needs:  "test",
						Steps:  []*Step{{Uses: "changesets/action@v1"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["release"].If != "" {
					t.Errorf("release should NOT get always(), got %q", wf.Jobs["release"].If)
				}
			},
		},
		{
			name: "skip deploy job with npm publish",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "build"}}},
					"publish": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "npm publish"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["publish"].If != "" {
					t.Errorf("publish should NOT get always(), got %q", wf.Jobs["publish"].If)
				}
			},
		},
		{
			name: "skip job by name even with safe steps",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "test"}}},
					"deploy": {
						RunsOn: "ubuntu-latest",
						Needs:  "test",
						Steps:  []*Step{{Run: "echo deploying"}}, // Safe step but job name is sensitive
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["deploy"].If != "" {
					t.Errorf("deploy should NOT get always() due to name, got %q", wf.Jobs["deploy"].If)
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
					"test":  {RunsOn: "ubuntu-latest", Needs: "build", Steps: []*Step{{Run: "test"}}},
					"build": nil,
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// Valid job should get always()
				if wf.Jobs["test"].If != "always()" {
					t.Errorf("test should have always(), got %q", wf.Jobs["test"].If)
				}
			},
		},
		{
			name: "mixed safe and sensitive jobs",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"lint": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "lint"}}},
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "test"}}},
					"build": {
						RunsOn: "ubuntu-latest",
						Needs:  []any{"lint", "test"},
						Steps:  []*Step{{Run: "go build"}},
					},
					"release": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Uses: "goreleaser/goreleaser-action@v5"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				// build should get always() (safe job with deps)
				if wf.Jobs["build"].If != "always()" {
					t.Errorf("build should have always(), got %q", wf.Jobs["build"].If)
				}
				// release should NOT get always() (sensitive action)
				if wf.Jobs["release"].If != "" {
					t.Errorf("release should NOT get always(), got %q", wf.Jobs["release"].If)
				}
				// lint and test should have no if (no deps)
				if wf.Jobs["lint"].If != "" {
					t.Errorf("lint should not have if, got %q", wf.Jobs["lint"].If)
				}
				if wf.Jobs["test"].If != "" {
					t.Errorf("test should not have if, got %q", wf.Jobs["test"].If)
				}
			},
		},
		// === Destructive operations should NOT get always() ===
		{
			name: "skip terraform destroy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"plan": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "terraform plan"}}},
					"destroy": {
						RunsOn: "ubuntu-latest",
						Needs:  "plan",
						Steps:  []*Step{{Run: "terraform destroy -auto-approve"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["destroy"].If != "" {
					t.Errorf("destroy should NOT get always() for terraform destroy, got %q", wf.Jobs["destroy"].If)
				}
			},
		},
		{
			name: "skip kubectl delete job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "go build"}}},
					"cleanup": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "kubectl delete -f deployment.yaml"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["cleanup"].If != "" {
					t.Errorf("cleanup should NOT get always() for kubectl delete, got %q", wf.Jobs["cleanup"].If)
				}
			},
		},
		{
			name: "skip helm uninstall job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "go build"}}},
					"teardown": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "helm uninstall myrelease"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["teardown"].If != "" {
					t.Errorf("teardown should NOT get always() for helm uninstall, got %q", wf.Jobs["teardown"].If)
				}
			},
		},
		{
			name: "skip pulumi destroy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"preview": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "pulumi preview"}}},
					"destroy": {
						RunsOn: "ubuntu-latest",
						Needs:  "preview",
						Steps:  []*Step{{Run: "pulumi destroy --yes"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["destroy"].If != "" {
					t.Errorf("destroy should NOT get always() for pulumi destroy, got %q", wf.Jobs["destroy"].If)
				}
			},
		},
		// === Database migrations should NOT get always() ===
		{
			name: "skip prisma migrate deploy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm run build"}}},
					"migrate": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "prisma migrate deploy"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["migrate"].If != "" {
					t.Errorf("migrate should NOT get always() for prisma migrate, got %q", wf.Jobs["migrate"].If)
				}
			},
		},
		{
			name: "skip alembic upgrade job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "pytest"}}},
					"migrate": {
						RunsOn: "ubuntu-latest",
						Needs:  "test",
						Steps:  []*Step{{Run: "alembic upgrade head"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["migrate"].If != "" {
					t.Errorf("migrate should NOT get always() for alembic upgrade, got %q", wf.Jobs["migrate"].If)
				}
			},
		},
		{
			name: "skip flyway migrate job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "mvn package"}}},
					"migrate": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "flyway migrate"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["migrate"].If != "" {
					t.Errorf("migrate should NOT get always() for flyway migrate, got %q", wf.Jobs["migrate"].If)
				}
			},
		},
		// === Cloud platforms should NOT get always() ===
		{
			name: "skip railway deploy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm run build"}}},
					"deploy": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "railway deploy"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["deploy"].If != "" {
					t.Errorf("deploy should NOT get always() for railway deploy, got %q", wf.Jobs["deploy"].If)
				}
			},
		},
		{
			name: "skip flyctl deploy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "go test"}}},
					"deploy": {
						RunsOn: "ubuntu-latest",
						Needs:  "test",
						Steps:  []*Step{{Run: "flyctl deploy"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["deploy"].If != "" {
					t.Errorf("deploy should NOT get always() for flyctl deploy, got %q", wf.Jobs["deploy"].If)
				}
			},
		},
		{
			name: "skip heroku deploy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "bundle exec rails test"}}},
					"deploy": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "git push heroku main"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["deploy"].If != "" {
					t.Errorf("deploy should NOT get always() for heroku deploy, got %q", wf.Jobs["deploy"].If)
				}
			},
		},
		{
			name: "skip wrangler deploy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm run build"}}},
					"deploy": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "wrangler deploy"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["deploy"].If != "" {
					t.Errorf("deploy should NOT get always() for wrangler deploy, got %q", wf.Jobs["deploy"].If)
				}
			},
		},
		{
			name: "skip firebase deploy job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm run build"}}},
					"deploy": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "firebase deploy"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["deploy"].If != "" {
					t.Errorf("deploy should NOT get always() for firebase deploy, got %q", wf.Jobs["deploy"].If)
				}
			},
		},
		// === Package managers should NOT get always() ===
		{
			name: "skip gem push job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "bundle exec rspec"}}},
					"publish": {
						RunsOn: "ubuntu-latest",
						Needs:  "test",
						Steps:  []*Step{{Run: "gem push mypackage-1.0.0.gem"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["publish"].If != "" {
					t.Errorf("publish should NOT get always() for gem push, got %q", wf.Jobs["publish"].If)
				}
			},
		},
		{
			name: "skip dotnet nuget push job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "dotnet build"}}},
					"publish": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "dotnet nuget push *.nupkg"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["publish"].If != "" {
					t.Errorf("publish should NOT get always() for dotnet nuget push, got %q", wf.Jobs["publish"].If)
				}
			},
		},
		{
			name: "skip poetry publish job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "pytest"}}},
					"publish": {
						RunsOn: "ubuntu-latest",
						Needs:  "test",
						Steps:  []*Step{{Run: "poetry publish --build"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["publish"].If != "" {
					t.Errorf("publish should NOT get always() for poetry publish, got %q", wf.Jobs["publish"].If)
				}
			},
		},
		{
			name: "skip cargo publish job",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"test": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "cargo test"}}},
					"publish": {
						RunsOn: "ubuntu-latest",
						Needs:  "test",
						Steps:  []*Step{{Run: "cargo publish"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["publish"].If != "" {
					t.Errorf("publish should NOT get always() for cargo publish, got %q", wf.Jobs["publish"].If)
				}
			},
		},
		// === Safe jobs with dependencies SHOULD get always() ===
		{
			name: "safe test job with go test gets always()",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "go build"}}},
					"test": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "go test ./..."}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["test"].If != "always()" {
					t.Errorf("test should get always() for go test, got %q", wf.Jobs["test"].If)
				}
			},
		},
		{
			name: "safe lint job with golangci-lint gets always()",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "go build"}}},
					"lint": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "golangci-lint run ./..."}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["lint"].If != "always()" {
					t.Errorf("lint should get always() for golangci-lint, got %q", wf.Jobs["lint"].If)
				}
			},
		},
		{
			name: "safe typecheck job with tsc gets always()",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"install": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "npm install"}}},
					"typecheck": {
						RunsOn: "ubuntu-latest",
						Needs:  "install",
						Steps:  []*Step{{Run: "tsc --noEmit"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["typecheck"].If != "always()" {
					t.Errorf("typecheck should get always() for tsc, got %q", wf.Jobs["typecheck"].If)
				}
			},
		},
		{
			name: "safe terraform plan job gets always()",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"init": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "terraform init"}}},
					"plan": {
						RunsOn: "ubuntu-latest",
						Needs:  "init",
						Steps:  []*Step{{Run: "terraform plan -out=tfplan"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["plan"].If != "always()" {
					t.Errorf("plan should get always() for terraform plan, got %q", wf.Jobs["plan"].If)
				}
			},
		},
		{
			name: "safe kubectl get job gets always()",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"auth": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "kubectl config use-context prod"}}},
					"check": {
						RunsOn: "ubuntu-latest",
						Needs:  "auth",
						Steps:  []*Step{{Run: "kubectl get pods"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["check"].If != "always()" {
					t.Errorf("check should get always() for kubectl get, got %q", wf.Jobs["check"].If)
				}
			},
		},
		{
			name: "safe helm lint job gets always()",
			workflow: &Workflow{
				Jobs: map[string]*Job{
					"build": {RunsOn: "ubuntu-latest", Steps: []*Step{{Run: "go build"}}},
					"lint": {
						RunsOn: "ubuntu-latest",
						Needs:  "build",
						Steps:  []*Step{{Run: "helm lint ./chart"}},
					},
				},
			},
			validate: func(t *testing.T, wf *Workflow) {
				t.Helper()
				if wf.Jobs["lint"].If != "always()" {
					t.Errorf("lint should get always() for helm lint, got %q", wf.Jobs["lint"].If)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InjectAlwaysForDependentJobs(tt.workflow, nil)
			tt.validate(t, tt.workflow)
		})
	}
}

// TestSanitizeForShellEcho_Unicode verifies sanitizeForShellEcho handles Unicode correctly
func TestSanitizeForShellEcho_Unicode(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple ascii", "Build project"},
		{"unicode chinese", "构建项目"},
		{"unicode japanese", "ビルド"},
		{"unicode emoji", "Build 🚀 Deploy"},
		{"mixed unicode", "Test 日本語 步骤"},
		{"newline attack", "test\n; rm -rf /"},
		{"single quote attack", "test'; rm -rf /; echo '"},
		{"carriage return attack", "test\r; rm -rf /"},
		{"tab injection", "test\t; rm -rf /"},
		{"null byte attack", "test\x00rm -rf /"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeForShellEcho(tt.input)

			// Result should not contain unescaped newlines
			if strings.Contains(result, "\n") {
				t.Errorf("sanitizeForShellEcho(%q) contains newline", tt.input)
			}

			// Result should not contain carriage returns
			if strings.Contains(result, "\r") {
				t.Errorf("sanitizeForShellEcho(%q) contains carriage return", tt.input)
			}

			// Result should not contain tabs
			if strings.Contains(result, "\t") {
				t.Errorf("sanitizeForShellEcho(%q) contains tab", tt.input)
			}

			// Result should not contain null bytes
			if strings.Contains(result, "\x00") {
				t.Errorf("sanitizeForShellEcho(%q) contains null byte", tt.input)
			}

			// Single quotes should be properly escaped as '\''
			// Each original quote ' becomes '\'' (end-quote, backslash-quote, start-quote = 3 quotes)
			escapedQuoteCount := strings.Count(result, "'\\''")
			totalQuoteCount := strings.Count(result, "'")
			expectedEscapeCount := strings.Count(tt.input, "'")
			if escapedQuoteCount != expectedEscapeCount {
				t.Errorf("sanitizeForShellEcho(%q): expected %d escaped quotes, got %d", tt.input, expectedEscapeCount, escapedQuoteCount)
			}
			// After escaping, total quotes = original_quotes * 3 (each ' becomes '\'')
			expectedTotalQuotes := expectedEscapeCount * 3
			if totalQuoteCount != expectedTotalQuotes {
				t.Errorf("sanitizeForShellEcho(%q): expected %d total quotes, got %d", tt.input, expectedTotalQuotes, totalQuoteCount)
			}
		})
	}
}

// TestTopologicalSortManifest_CircularDependencies verifies topological sort handles cycles
func TestTopologicalSortManifest_CircularDependencies(t *testing.T) {
	// Clear any existing warnings
	_ = GetAndClearCycleWarnings()

	// Create jobs with circular dependencies: a -> b -> c -> a
	jobInfoMap := map[string]*ci.ManifestJob{
		"a": {ID: "a", Name: "Job A", Needs: []string{"c"}},
		"b": {ID: "b", Name: "Job B", Needs: []string{"a"}},
		"c": {ID: "c", Name: "Job C", Needs: []string{"b"}},
		"d": {ID: "d", Name: "Job D", Needs: []string{}}, // No deps, should be first
	}

	result := topologicalSortManifest(jobInfoMap)

	// All jobs should still be included
	if len(result) != 4 {
		t.Errorf("expected 4 jobs, got %d", len(result))
	}

	// Job D should be first (no dependencies)
	if result[0].ID != "d" {
		t.Errorf("expected job 'd' first, got %q", result[0].ID)
	}

	// Check that cycle warnings were recorded
	warnings := GetAndClearCycleWarnings()
	if len(warnings) != 3 {
		t.Errorf("expected 3 cycle warnings (a,b,c), got %d: %v", len(warnings), warnings)
	}

	// Verify all cycle jobs are in warnings
	warningSet := make(map[string]bool)
	for _, w := range warnings {
		warningSet[w] = true
	}
	for _, cycleJob := range []string{"a", "b", "c"} {
		if !warningSet[cycleJob] {
			t.Errorf("expected job %q in cycle warnings", cycleJob)
		}
	}
}

// TestInjectJobMarkers_SpecialStepNames verifies markers handle special characters in step names
func TestInjectJobMarkers_SpecialStepNames(t *testing.T) {
	wf := &Workflow{
		Jobs: map[string]*Job{
			"test": {
				RunsOn: "ubuntu-latest",
				Steps: []*Step{
					{Name: "Build with 日本語"},
					{Name: "Test\nwith\nnewlines"},
					{Name: "Deploy'; echo 'pwned"},
				},
			},
		},
	}

	InjectJobMarkers(wf)

	job := wf.Jobs["test"]
	// Expected steps: manifest + job-start + (step-marker + original) * 3 + job-end = 1 + 1 + 6 + 1 = 9
	if len(job.Steps) != 9 {
		t.Errorf("expected 9 steps, got %d", len(job.Steps))
	}

	// Find and verify step marker steps
	for i, step := range job.Steps {
		if strings.HasPrefix(step.Name, "detent: step") {
			// Marker steps should have safe echo commands
			if strings.Contains(step.Run, "\n") && !strings.HasPrefix(step.Run, "echo") {
				t.Errorf("step %d: marker step has unsafe newline in non-echo content: %s", i, step.Run)
			}
			// Verify the echo command doesn't contain unescaped dangerous characters
			// The Run field should be: echo '::detent::step-start::test::N::sanitized_name'
			if !strings.HasPrefix(step.Run, "echo '::detent::step-start::") {
				t.Errorf("step %d: unexpected marker format: %s", i, step.Run)
			}
			// Extract the step name part and verify it's safe
			// Format: echo '::detent::step-start::jobID::index::stepName'
			parts := strings.Split(step.Run, "::")
			if len(parts) >= 5 {
				stepName := strings.TrimSuffix(parts[4], "'")
				// Step name should not contain raw newlines
				if strings.Contains(stepName, "\n") {
					t.Errorf("step %d: step name contains unescaped newline: %q", i, stepName)
				}
			}
		}
	}

	// Verify original steps are preserved
	originalStepIndices := []int{3, 5, 7} // After manifest, job-start, and marker steps
	expectedNames := []string{"Build with 日本語", "Test\nwith\nnewlines", "Deploy'; echo 'pwned"}
	for j, idx := range originalStepIndices {
		if idx >= len(job.Steps) {
			t.Errorf("expected step at index %d, but only have %d steps", idx, len(job.Steps))
			continue
		}
		if job.Steps[idx].Name != expectedNames[j] {
			t.Errorf("original step %d: expected name %q, got %q", j, expectedNames[j], job.Steps[idx].Name)
		}
	}
}
