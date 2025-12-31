package tui

import (
	"testing"

	"github.com/detent/cli/internal/ci"
	"github.com/detent/cli/internal/workflow"
)

func TestJobTracker_BasicFlow(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "lint", Name: "Lint"},
		{ID: "test", Name: "Test"},
		{ID: "build", Name: "Build"},
	}

	tracker := NewJobTracker(jobs)

	// Initially all pending
	for _, job := range tracker.GetJobs() {
		if job.Status != ci.JobPending {
			t.Errorf("job %s should be pending, got %s", job.Name, job.Status)
		}
	}

	// Lint starts (using job ID)
	changed := tracker.ProcessEvent(&ci.JobEvent{JobID: "lint", Action: "start"})
	if !changed {
		t.Error("ProcessEvent should return true for start")
	}
	if tracker.jobByID["lint"].Status != ci.JobRunning {
		t.Errorf("Lint should be running")
	}

	// Lint finishes successfully
	changed = tracker.ProcessEvent(&ci.JobEvent{JobID: "lint", Action: "finish", Success: true})
	if !changed {
		t.Error("ProcessEvent should return true for finish")
	}
	if tracker.jobByID["lint"].Status != ci.JobSuccess {
		t.Errorf("Lint should be success")
	}

	// Test starts and fails
	tracker.ProcessEvent(&ci.JobEvent{JobID: "test", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "test", Action: "finish", Success: false})
	if tracker.jobByID["test"].Status != ci.JobFailed {
		t.Errorf("Test should be failed")
	}
}

func TestJobTracker_MarkAllComplete(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
	}

	tracker := NewJobTracker(jobs)

	tracker.ProcessEvent(&ci.JobEvent{JobID: "a", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "b", Action: "start"})

	// Mark all complete without errors
	tracker.MarkAllRunningComplete(false)

	if tracker.jobByID["a"].Status != ci.JobSuccess {
		t.Errorf("A should be success")
	}
	if tracker.jobByID["b"].Status != ci.JobSuccess {
		t.Errorf("B should be success")
	}
}

func TestJobTracker_MarkAllCompleteWithErrors(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
	}

	tracker := NewJobTracker(jobs)

	tracker.ProcessEvent(&ci.JobEvent{JobID: "a", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "b", Action: "start"})

	// Mark all complete with errors
	tracker.MarkAllRunningComplete(true)

	if tracker.jobByID["a"].Status != ci.JobFailed {
		t.Errorf("A should be failed")
	}
	if tracker.jobByID["b"].Status != ci.JobFailed {
		t.Errorf("B should be failed")
	}
}

func TestJobTracker_ConcurrentJobs(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "cli-lint", Name: "[CLI] Lint"},
		{ID: "cli-test", Name: "[CLI] Test"},
		{ID: "web-lint", Name: "[Web] Lint"},
		{ID: "release", Name: "Release"},
	}

	tracker := NewJobTracker(jobs)

	// Jobs start concurrently (using job IDs)
	tracker.ProcessEvent(&ci.JobEvent{JobID: "cli-lint", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "cli-test", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "web-lint", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "release", Action: "start"})

	// All should be running
	for _, id := range []string{"cli-lint", "cli-test", "web-lint", "release"} {
		if tracker.jobByID[id].Status != ci.JobRunning {
			t.Errorf("%s should be running", id)
		}
	}

	// Some finish successfully, some fail
	tracker.ProcessEvent(&ci.JobEvent{JobID: "cli-lint", Action: "finish", Success: true})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "cli-test", Action: "finish", Success: false})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "web-lint", Action: "finish", Success: true})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "release", Action: "finish", Success: true})

	if tracker.jobByID["cli-lint"].Status != ci.JobSuccess {
		t.Errorf("cli-lint should be success")
	}
	if tracker.jobByID["cli-test"].Status != ci.JobFailed {
		t.Errorf("cli-test should be failed")
	}
	if tracker.jobByID["web-lint"].Status != ci.JobSuccess {
		t.Errorf("web-lint should be success")
	}
	if tracker.jobByID["release"].Status != ci.JobSuccess {
		t.Errorf("release should be success")
	}
}

func TestJobTracker_NilEvent(t *testing.T) {
	jobs := []workflow.JobInfo{{ID: "a", Name: "A"}}
	tracker := NewJobTracker(jobs)

	changed := tracker.ProcessEvent(nil)
	if changed {
		t.Error("ProcessEvent(nil) should return false")
	}
}

func TestJobTracker_UnknownJob(t *testing.T) {
	jobs := []workflow.JobInfo{{ID: "a", Name: "A"}}
	tracker := NewJobTracker(jobs)

	changed := tracker.ProcessEvent(&ci.JobEvent{JobID: "unknown", Action: "start"})
	if changed {
		t.Error("ProcessEvent for unknown job should return false")
	}
}

func TestJobTracker_PendingJobsMarkedFailed(t *testing.T) {
	// This tests the case where some jobs never start (e.g., due to Docker issues)
	// and should be marked as failed when the workflow completes
	jobs := []workflow.JobInfo{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
		{ID: "c", Name: "C"},
	}

	tracker := NewJobTracker(jobs)

	// Only A starts and completes
	tracker.ProcessEvent(&ci.JobEvent{JobID: "a", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "a", Action: "finish", Success: true})

	// B starts but doesn't finish
	tracker.ProcessEvent(&ci.JobEvent{JobID: "b", Action: "start"})

	// C never starts (stays pending)

	// Mark all complete - workflow had errors (because some jobs didn't finish)
	tracker.MarkAllRunningComplete(true)

	// A was already complete, should stay success
	if tracker.jobByID["a"].Status != ci.JobSuccess {
		t.Errorf("A should be success, got %s", tracker.jobByID["a"].Status)
	}

	// B was running, should be marked failed
	if tracker.jobByID["b"].Status != ci.JobFailed {
		t.Errorf("B should be failed (was running), got %s", tracker.jobByID["b"].Status)
	}

	// C never started, should be marked failed (not left as pending)
	if tracker.jobByID["c"].Status != ci.JobFailed {
		t.Errorf("C should be failed (never started), got %s", tracker.jobByID["c"].Status)
	}
}

func TestJobTracker_SkippedJob(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "lint", Name: "Lint"},
		{ID: "test", Name: "Test", Needs: []string{"lint"}},
		{ID: "deploy", Name: "Deploy", Needs: []string{"test"}},
	}

	tracker := NewJobTracker(jobs)

	// Lint starts and fails
	tracker.ProcessEvent(&ci.JobEvent{JobID: "lint", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "lint", Action: "finish", Success: false})

	// Test is skipped because lint failed
	changed := tracker.ProcessEvent(&ci.JobEvent{JobID: "test", Action: "skip"})
	if !changed {
		t.Error("ProcessEvent should return true for skip")
	}
	if tracker.jobByID["test"].Status != ci.JobSkipped {
		t.Errorf("Test should be skipped, got %s", tracker.jobByID["test"].Status)
	}

	// Deploy is also skipped
	tracker.ProcessEvent(&ci.JobEvent{JobID: "deploy", Action: "skip"})
	if tracker.jobByID["deploy"].Status != ci.JobSkipped {
		t.Errorf("Deploy should be skipped, got %s", tracker.jobByID["deploy"].Status)
	}
}

func TestJobTracker_SkippedJobsNotMarkedFailed(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
		{ID: "c", Name: "C"},
	}

	tracker := NewJobTracker(jobs)

	// A completes successfully
	tracker.ProcessEvent(&ci.JobEvent{JobID: "a", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobID: "a", Action: "finish", Success: true})

	// B is skipped (e.g., conditional job)
	tracker.ProcessEvent(&ci.JobEvent{JobID: "b", Action: "skip"})

	// C is running
	tracker.ProcessEvent(&ci.JobEvent{JobID: "c", Action: "start"})

	// Mark all complete with errors
	tracker.MarkAllRunningComplete(true)

	// A should stay success
	if tracker.jobByID["a"].Status != ci.JobSuccess {
		t.Errorf("A should be success, got %s", tracker.jobByID["a"].Status)
	}

	// B should stay skipped (not changed to failed)
	if tracker.jobByID["b"].Status != ci.JobSkipped {
		t.Errorf("B should remain skipped, got %s", tracker.jobByID["b"].Status)
	}

	// C was running, should be marked failed
	if tracker.jobByID["c"].Status != ci.JobFailed {
		t.Errorf("C should be failed, got %s", tracker.jobByID["c"].Status)
	}
}

func TestJobTrackerFromManifest(t *testing.T) {
	manifest := &ci.ManifestInfo{
		Version: 2,
		Jobs: []ci.ManifestJob{
			{ID: "build", Name: "Build", Steps: []string{"Checkout", "Install", "Build"}},
			{ID: "deploy", Name: "Deploy", Uses: "org/repo/.github/workflows/deploy.yml@main"},
		},
	}

	tracker := NewJobTrackerFromManifest(manifest)

	jobs := tracker.GetJobs()
	if len(jobs) != 2 {
		t.Fatalf("Expected 2 jobs, got %d", len(jobs))
	}

	// Check first job (regular with steps)
	buildJob := tracker.GetJob("build")
	if buildJob == nil {
		t.Fatal("build job not found")
	}
	if buildJob.ID != "build" {
		t.Errorf("build job ID = %q, want %q", buildJob.ID, "build")
	}
	if buildJob.Name != "Build" {
		t.Errorf("build job Name = %q, want %q", buildJob.Name, "Build")
	}
	if len(buildJob.Steps) != 3 {
		t.Errorf("build job Steps = %d, want 3", len(buildJob.Steps))
	}
	if buildJob.IsReusable {
		t.Error("build job IsReusable = true, want false")
	}

	// Check second job (reusable workflow)
	deployJob := tracker.GetJob("deploy")
	if deployJob == nil {
		t.Fatal("deploy job not found")
	}
	if !deployJob.IsReusable {
		t.Error("deploy job IsReusable = false, want true")
	}
	if len(deployJob.Steps) != 0 {
		t.Errorf("deploy job Steps = %d, want 0 for reusable workflow", len(deployJob.Steps))
	}
}

func TestJobTracker_StepEvents(t *testing.T) {
	manifest := &ci.ManifestInfo{
		Version: 2,
		Jobs: []ci.ManifestJob{
			{ID: "build", Name: "Build", Steps: []string{"Checkout", "Install", "Build"}},
		},
	}

	tracker := NewJobTrackerFromManifest(manifest)

	// Start the job
	tracker.ProcessEvent(&ci.JobEvent{JobID: "build", Action: "start"})

	// Process step events
	changed := tracker.ProcessStepEvent(&ci.StepEvent{JobID: "build", StepIdx: 0, StepName: "Checkout"})
	if !changed {
		t.Error("ProcessStepEvent should return true for first step")
	}

	buildJob := tracker.GetJob("build")
	if buildJob.CurrentStep != 0 {
		t.Errorf("CurrentStep = %d, want 0", buildJob.CurrentStep)
	}
	if buildJob.Steps[0].Status != ci.StepRunning {
		t.Errorf("Step 0 status = %s, want running", buildJob.Steps[0].Status)
	}

	// Move to next step
	tracker.ProcessStepEvent(&ci.StepEvent{JobID: "build", StepIdx: 1, StepName: "Install"})

	// Previous step should be marked as success
	if buildJob.Steps[0].Status != ci.StepSuccess {
		t.Errorf("Step 0 status = %s, want success", buildJob.Steps[0].Status)
	}
	if buildJob.Steps[1].Status != ci.StepRunning {
		t.Errorf("Step 1 status = %s, want running", buildJob.Steps[1].Status)
	}
	if buildJob.CurrentStep != 1 {
		t.Errorf("CurrentStep = %d, want 1", buildJob.CurrentStep)
	}
}

func TestJobTracker_StepEventInvalidJob(t *testing.T) {
	manifest := &ci.ManifestInfo{
		Version: 2,
		Jobs: []ci.ManifestJob{
			{ID: "build", Name: "Build", Steps: []string{"Checkout"}},
		},
	}

	tracker := NewJobTrackerFromManifest(manifest)

	// Step event for unknown job
	changed := tracker.ProcessStepEvent(&ci.StepEvent{JobID: "unknown", StepIdx: 0, StepName: "Step"})
	if changed {
		t.Error("ProcessStepEvent for unknown job should return false")
	}
}

func TestJobTracker_NilManifest(t *testing.T) {
	tracker := NewJobTrackerFromManifest(nil)

	if len(tracker.GetJobs()) != 0 {
		t.Error("Expected empty jobs list for nil manifest")
	}
}
