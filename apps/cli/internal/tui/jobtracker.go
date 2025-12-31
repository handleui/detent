package tui

import (
	"sync"

	"github.com/detent/cli/internal/ci"
	"github.com/detent/cli/internal/workflow"
)

// TrackedJob represents a job being tracked in the TUI.
type TrackedJob struct {
	ID     string
	Name   string
	Status ci.JobStatus
}

// JobTracker manages job state based on CI output events.
type JobTracker struct {
	mu        sync.RWMutex
	jobs      []*TrackedJob
	jobByName map[string]*TrackedJob
}

// NewJobTracker creates a new job tracker from workflow jobs.
func NewJobTracker(jobs []workflow.JobInfo) *JobTracker {
	t := &JobTracker{
		jobs:      make([]*TrackedJob, 0, len(jobs)),
		jobByName: make(map[string]*TrackedJob),
	}

	for _, j := range jobs {
		tj := &TrackedJob{
			ID:     j.ID,
			Name:   j.Name,
			Status: ci.JobPending,
		}
		t.jobs = append(t.jobs, tj)
		t.jobByName[j.Name] = tj
	}

	return t
}

// ProcessEvent processes a job event and updates job status.
// Returns true if any job status changed.
func (t *JobTracker) ProcessEvent(event *ci.JobEvent) bool {
	if event == nil {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	job := t.jobByName[event.JobName]
	if job == nil {
		return false
	}

	switch event.Action {
	case "start":
		if job.Status == ci.JobPending {
			job.Status = ci.JobRunning
			return true
		}
	case "finish":
		if job.Status == ci.JobRunning || job.Status == ci.JobPending {
			if event.Success {
				job.Status = ci.JobSuccess
			} else {
				job.Status = ci.JobFailed
			}
			return true
		}
	case "skip":
		if job.Status == ci.JobPending {
			job.Status = ci.JobSkipped
			return true
		}
	}

	return false
}

// MarkAllRunningComplete marks all running and pending jobs as complete.
// Called when the entire workflow finishes.
// Jobs that never started (stayed pending) are also marked - this handles cases
// where act fails early (e.g., Docker issues) before emitting start events.
// Skipped jobs are left as skipped (not marked as failed).
func (t *JobTracker) MarkAllRunningComplete(hasErrors bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, job := range t.jobs {
		switch job.Status {
		case ci.JobRunning:
			// Running jobs get their final status based on errors
			if hasErrors {
				job.Status = ci.JobFailed
			} else {
				job.Status = ci.JobSuccess
			}
		case ci.JobPending:
			// Pending jobs that never started should be marked as failed
			// (they didn't run, which is a failure condition)
			job.Status = ci.JobFailed
		case ci.JobSuccess, ci.JobFailed, ci.JobSkipped:
			// Already complete or skipped, no change needed
		}
	}
}

// GetJobs returns all tracked jobs in order.
func (t *JobTracker) GetJobs() []*TrackedJob {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.jobs
}
