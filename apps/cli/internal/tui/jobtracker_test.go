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

	// Lint starts
	changed := tracker.ProcessEvent(&ci.JobEvent{JobName: "Lint", Action: "start"})
	if !changed {
		t.Error("ProcessEvent should return true for start")
	}
	if tracker.jobByName["Lint"].Status != ci.JobRunning {
		t.Errorf("Lint should be running")
	}

	// Lint finishes successfully
	changed = tracker.ProcessEvent(&ci.JobEvent{JobName: "Lint", Action: "finish", Success: true})
	if !changed {
		t.Error("ProcessEvent should return true for finish")
	}
	if tracker.jobByName["Lint"].Status != ci.JobSuccess {
		t.Errorf("Lint should be success")
	}

	// Test starts and fails
	tracker.ProcessEvent(&ci.JobEvent{JobName: "Test", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "Test", Action: "finish", Success: false})
	if tracker.jobByName["Test"].Status != ci.JobFailed {
		t.Errorf("Test should be failed")
	}
}

func TestJobTracker_MarkAllComplete(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
	}

	tracker := NewJobTracker(jobs)

	tracker.ProcessEvent(&ci.JobEvent{JobName: "A", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "B", Action: "start"})

	// Mark all complete without errors
	tracker.MarkAllRunningComplete(false)

	if tracker.jobByName["A"].Status != ci.JobSuccess {
		t.Errorf("A should be success")
	}
	if tracker.jobByName["B"].Status != ci.JobSuccess {
		t.Errorf("B should be success")
	}
}

func TestJobTracker_MarkAllCompleteWithErrors(t *testing.T) {
	jobs := []workflow.JobInfo{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
	}

	tracker := NewJobTracker(jobs)

	tracker.ProcessEvent(&ci.JobEvent{JobName: "A", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "B", Action: "start"})

	// Mark all complete with errors
	tracker.MarkAllRunningComplete(true)

	if tracker.jobByName["A"].Status != ci.JobFailed {
		t.Errorf("A should be failed")
	}
	if tracker.jobByName["B"].Status != ci.JobFailed {
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

	// Jobs start concurrently
	tracker.ProcessEvent(&ci.JobEvent{JobName: "[CLI] Lint", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "[CLI] Test", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "[Web] Lint", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "Release", Action: "start"})

	// All should be running
	for _, name := range []string{"[CLI] Lint", "[CLI] Test", "[Web] Lint", "Release"} {
		if tracker.jobByName[name].Status != ci.JobRunning {
			t.Errorf("%s should be running", name)
		}
	}

	// Some finish successfully, some fail
	tracker.ProcessEvent(&ci.JobEvent{JobName: "[CLI] Lint", Action: "finish", Success: true})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "[CLI] Test", Action: "finish", Success: false})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "[Web] Lint", Action: "finish", Success: true})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "Release", Action: "finish", Success: true})

	if tracker.jobByName["[CLI] Lint"].Status != ci.JobSuccess {
		t.Errorf("[CLI] Lint should be success")
	}
	if tracker.jobByName["[CLI] Test"].Status != ci.JobFailed {
		t.Errorf("[CLI] Test should be failed")
	}
	if tracker.jobByName["[Web] Lint"].Status != ci.JobSuccess {
		t.Errorf("[Web] Lint should be success")
	}
	if tracker.jobByName["Release"].Status != ci.JobSuccess {
		t.Errorf("Release should be success")
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

	changed := tracker.ProcessEvent(&ci.JobEvent{JobName: "Unknown", Action: "start"})
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
	tracker.ProcessEvent(&ci.JobEvent{JobName: "A", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "A", Action: "finish", Success: true})

	// B starts but doesn't finish
	tracker.ProcessEvent(&ci.JobEvent{JobName: "B", Action: "start"})

	// C never starts (stays pending)

	// Mark all complete - workflow had errors (because some jobs didn't finish)
	tracker.MarkAllRunningComplete(true)

	// A was already complete, should stay success
	if tracker.jobByName["A"].Status != ci.JobSuccess {
		t.Errorf("A should be success, got %s", tracker.jobByName["A"].Status)
	}

	// B was running, should be marked failed
	if tracker.jobByName["B"].Status != ci.JobFailed {
		t.Errorf("B should be failed (was running), got %s", tracker.jobByName["B"].Status)
	}

	// C never started, should be marked failed (not left as pending)
	if tracker.jobByName["C"].Status != ci.JobFailed {
		t.Errorf("C should be failed (never started), got %s", tracker.jobByName["C"].Status)
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
	tracker.ProcessEvent(&ci.JobEvent{JobName: "Lint", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "Lint", Action: "finish", Success: false})

	// Test is skipped because lint failed
	changed := tracker.ProcessEvent(&ci.JobEvent{JobName: "Test", Action: "skip"})
	if !changed {
		t.Error("ProcessEvent should return true for skip")
	}
	if tracker.jobByName["Test"].Status != ci.JobSkipped {
		t.Errorf("Test should be skipped, got %s", tracker.jobByName["Test"].Status)
	}

	// Deploy is also skipped
	tracker.ProcessEvent(&ci.JobEvent{JobName: "Deploy", Action: "skip"})
	if tracker.jobByName["Deploy"].Status != ci.JobSkipped {
		t.Errorf("Deploy should be skipped, got %s", tracker.jobByName["Deploy"].Status)
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
	tracker.ProcessEvent(&ci.JobEvent{JobName: "A", Action: "start"})
	tracker.ProcessEvent(&ci.JobEvent{JobName: "A", Action: "finish", Success: true})

	// B is skipped (e.g., conditional job)
	tracker.ProcessEvent(&ci.JobEvent{JobName: "B", Action: "skip"})

	// C is running
	tracker.ProcessEvent(&ci.JobEvent{JobName: "C", Action: "start"})

	// Mark all complete with errors
	tracker.MarkAllRunningComplete(true)

	// A should stay success
	if tracker.jobByName["A"].Status != ci.JobSuccess {
		t.Errorf("A should be success, got %s", tracker.jobByName["A"].Status)
	}

	// B should stay skipped (not changed to failed)
	if tracker.jobByName["B"].Status != ci.JobSkipped {
		t.Errorf("B should remain skipped, got %s", tracker.jobByName["B"].Status)
	}

	// C was running, should be marked failed
	if tracker.jobByName["C"].Status != ci.JobFailed {
		t.Errorf("C should be failed, got %s", tracker.jobByName["C"].Status)
	}
}
