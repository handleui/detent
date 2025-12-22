package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// InjectContinueOnError modifies a workflow to add continue-on-error: true to all jobs and steps.
// This ensures that Docker failures, job-level failures, and step-level failures don't stop execution,
// allowing Detent to capture ALL errors instead of just the first failure.
func InjectContinueOnError(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}
	for _, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Inject at JOB level ONLY (critical for Docker failures and continuing past step failures)
		// Job.ContinueOnError is `any` type to support bool or expressions
		// NOTE: We intentionally do NOT inject at step level because it suppresses step output in act,
		// preventing error extraction. Job-level continue-on-error is sufficient to prevent workflow truncation.
		if job.ContinueOnError == nil || job.ContinueOnError == false {
			job.ContinueOnError = true
		}
	}
}

// InjectTimeouts adds reasonable timeout values to prevent hanging Docker operations.
// Jobs default to 30 minutes, steps to 15 minutes. Only applied if not already set.
func InjectTimeouts(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}

	const (
		defaultJobTimeout  = 30 // 30 minutes for jobs
		defaultStepTimeout = 15 // 15 minutes for steps
	)

	for _, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Set job timeout if not already specified
		if job.TimeoutMinutes == nil {
			job.TimeoutMinutes = defaultJobTimeout
		}

		// Set step timeouts if not already specified
		if job.Steps != nil {
			for _, step := range job.Steps {
				if step == nil {
					continue
				}
				if step.TimeoutMinutes == nil {
					step.TimeoutMinutes = defaultStepTimeout
				}
			}
		}
	}
}

// PrepareWorkflows processes all workflows and returns temp directory path
func PrepareWorkflows(srcDir string) (tmpDir string, cleanup func(), err error) {
	workflows, err := DiscoverWorkflows(srcDir)
	if err != nil {
		return "", nil, err
	}

	if len(workflows) == 0 {
		return "", nil, fmt.Errorf("no workflow files found in %s", srcDir)
	}

	tmpDir, err = os.MkdirTemp("", "detent-workflows-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp directory: %w", err)
	}

	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	for _, wfPath := range workflows {
		wf, err := ParseWorkflowFile(wfPath)
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("parsing %s: %w", wfPath, err)
		}

		InjectContinueOnError(wf)
		InjectTimeouts(wf)

		data, err := yaml.Marshal(wf)
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("marshaling %s: %w", wfPath, err)
		}

		filename := filepath.Base(wfPath)
		if err := os.WriteFile(filepath.Join(tmpDir, filename), data, 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("writing %s: %w", filename, err)
		}
	}

	return tmpDir, cleanup, nil
}
