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
	}

	return false
}

// MarkAllRunningComplete marks all running jobs as complete.
// Called when the entire workflow finishes.
func (t *JobTracker) MarkAllRunningComplete(hasErrors bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, job := range t.jobs {
		if job.Status == ci.JobRunning {
			if hasErrors {
				job.Status = ci.JobFailed
			} else {
				job.Status = ci.JobSuccess
			}
		}
	}
}

// GetJobs returns all tracked jobs in order.
func (t *JobTracker) GetJobs() []*TrackedJob {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.jobs
}
