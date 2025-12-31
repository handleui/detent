package tui

import (
	"sync"

	"github.com/detent/cli/internal/ci"
	"github.com/detent/cli/internal/workflow"
)

// TrackedStep represents a step being tracked in the TUI.
type TrackedStep struct {
	Index  int
	Name   string
	Status ci.StepStatus
}

// TrackedJob represents a job being tracked in the TUI.
type TrackedJob struct {
	ID          string
	Name        string
	Status      ci.JobStatus
	IsReusable  bool           // True for jobs with uses: (reusable workflows)
	IsSensitive bool           // True for jobs that may publish, release, or deploy
	Steps       []*TrackedStep // Steps in this job (empty for reusable)
	CurrentStep int            // Index of currently running step (-1 if not started)
}

// JobTracker manages job state based on CI output events.
type JobTracker struct {
	mu      sync.RWMutex
	jobs    []*TrackedJob
	jobByID map[string]*TrackedJob // Changed from jobByName for correct ID-based lookup
}

// NewJobTracker creates a new job tracker from workflow jobs.
// This is the legacy constructor for backward compatibility.
func NewJobTracker(jobs []workflow.JobInfo) *JobTracker {
	t := &JobTracker{
		jobs:    make([]*TrackedJob, 0, len(jobs)),
		jobByID: make(map[string]*TrackedJob),
	}

	for _, j := range jobs {
		tj := &TrackedJob{
			ID:          j.ID,
			Name:        j.Name,
			Status:      ci.JobPending,
			CurrentStep: -1,
		}
		t.jobs = append(t.jobs, tj)
		t.jobByID[j.ID] = tj
	}

	return t
}

// NewJobTrackerFromManifest creates a job tracker from a parsed manifest.
// This is the preferred constructor for manifest-first architecture.
func NewJobTrackerFromManifest(manifest *ci.ManifestInfo) *JobTracker {
	if manifest == nil {
		return &JobTracker{
			jobs:    make([]*TrackedJob, 0),
			jobByID: make(map[string]*TrackedJob),
		}
	}

	t := &JobTracker{
		jobs:    make([]*TrackedJob, 0, len(manifest.Jobs)),
		jobByID: make(map[string]*TrackedJob),
	}

	for _, mj := range manifest.Jobs {
		tj := &TrackedJob{
			ID:          mj.ID,
			Name:        mj.Name,
			Status:      ci.JobPending,
			IsReusable:  mj.Uses != "",
			IsSensitive: mj.Sensitive,
			CurrentStep: -1,
		}

		// Create tracked steps from manifest
		if len(mj.Steps) > 0 {
			tj.Steps = make([]*TrackedStep, len(mj.Steps))
			for i, stepName := range mj.Steps {
				tj.Steps[i] = &TrackedStep{
					Index:  i,
					Name:   stepName,
					Status: ci.StepPending,
				}
			}
		}

		t.jobs = append(t.jobs, tj)
		t.jobByID[mj.ID] = tj
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

	job := t.jobByID[event.JobID]
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
			// Mark all remaining pending steps based on outcome
			t.finalizeJobSteps(job, event.Success)

			if event.Success {
				job.Status = ci.JobSuccess
			} else {
				job.Status = ci.JobFailed
			}
			return true
		}
	case "skip":
		if job.Status == ci.JobPending {
			// Mark all steps as skipped
			for _, step := range job.Steps {
				step.Status = ci.StepSkipped
			}
			// Use JobSkippedSecurity for sensitive jobs to show lock icon
			if job.IsSensitive {
				job.Status = ci.JobSkippedSecurity
			} else {
				job.Status = ci.JobSkipped
			}
			return true
		}
	}

	return false
}

// ProcessStepEvent processes a step event and updates step status.
// Returns true if any step status changed.
func (t *JobTracker) ProcessStepEvent(event *ci.StepEvent) bool {
	if event == nil {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	job := t.jobByID[event.JobID]
	if job == nil || event.StepIdx < 0 || event.StepIdx >= len(job.Steps) {
		return false
	}

	// Mark previous running step as completed (success assumed if next step started)
	if job.CurrentStep >= 0 && job.CurrentStep < len(job.Steps) {
		prevStep := job.Steps[job.CurrentStep]
		if prevStep.Status == ci.StepRunning {
			prevStep.Status = ci.StepSuccess
		}
	}

	// Update current step
	job.CurrentStep = event.StepIdx
	step := job.Steps[event.StepIdx]
	step.Status = ci.StepRunning

	return true
}

// finalizeJobSteps marks remaining steps based on job outcome.
func (t *JobTracker) finalizeJobSteps(job *TrackedJob, success bool) {
	for _, step := range job.Steps {
		switch step.Status {
		case ci.StepRunning:
			// Current step - mark based on job outcome
			switch {
			case success:
				step.Status = ci.StepSuccess
			default:
				step.Status = ci.StepFailed
			}
		case ci.StepPending:
			// Never ran - cancelled or skipped due to failure
			switch {
			case success:
				// Job succeeded but step never ran? Mark as success (must have run)
				step.Status = ci.StepSuccess
			default:
				step.Status = ci.StepCancelled
			}
		case ci.StepSuccess, ci.StepFailed, ci.StepSkipped, ci.StepCancelled:
			// Already in final state, no action needed
		}
	}
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
			// Finalize steps based on error status
			t.finalizeJobSteps(job, !hasErrors)

			// Running jobs get their final status based on errors
			if hasErrors {
				job.Status = ci.JobFailed
			} else {
				job.Status = ci.JobSuccess
			}
		case ci.JobPending:
			// Mark all steps as failed/cancelled
			for _, step := range job.Steps {
				step.Status = ci.StepCancelled
			}
			// Sensitive jobs that never started should be marked as security-skipped
			// (they were intentionally not run to prevent accidental releases)
			// Other pending jobs are marked as failed (they didn't run, which is a failure condition)
			if job.IsSensitive {
				job.Status = ci.JobSkippedSecurity
			} else {
				job.Status = ci.JobFailed
			}
		case ci.JobSuccess, ci.JobFailed, ci.JobSkipped, ci.JobSkippedSecurity:
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

// GetJob returns a job by ID.
func (t *JobTracker) GetJob(jobID string) *TrackedJob {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.jobByID[jobID]
}
