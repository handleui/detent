package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/goccy/go-yaml"
	"golang.org/x/sync/errgroup"
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

const (
	// Timeout values in minutes to prevent hanging Docker operations
	defaultJobTimeoutMinutes  = 30 // Default timeout for jobs
	defaultStepTimeoutMinutes = 15 // Default timeout for steps
)

// InjectTimeouts adds reasonable timeout values to prevent hanging Docker operations.
// Jobs default to 30 minutes, steps to 15 minutes. Only applied if not already set.
func InjectTimeouts(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}

	for _, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Set job timeout if not already specified
		if job.TimeoutMinutes == nil {
			job.TimeoutMinutes = defaultJobTimeoutMinutes
		}

		// Set step timeouts if not already specified
		if job.Steps != nil {
			for _, step := range job.Steps {
				if step == nil {
					continue
				}
				if step.TimeoutMinutes == nil {
					step.TimeoutMinutes = defaultStepTimeoutMinutes
				}
			}
		}
	}
}

// PrepareWorkflows processes workflows and returns temp directory path.
// If specificWorkflow is provided, only that workflow is processed.
// Otherwise, all workflows in srcDir are discovered and processed.
func PrepareWorkflows(srcDir, specificWorkflow string) (tmpDir string, cleanup func(), err error) {
	var workflows []string

	if specificWorkflow != "" {
		// Validate path BEFORE cleaning to catch patterns like ./file
		if filepath.IsAbs(specificWorkflow) || specificWorkflow != "" && specificWorkflow[0] == '.' {
			return "", nil, fmt.Errorf("workflow path must be relative and cannot reference parent directories")
		}

		// Clean the path after validation
		cleanWorkflow := filepath.Clean(specificWorkflow)

		// Get absolute paths for validation
		absSrcDir, absErr := filepath.Abs(srcDir)
		if absErr != nil {
			return "", nil, fmt.Errorf("resolving source directory: %w", absErr)
		}

		// Process specific workflow file
		workflowPath := filepath.Join(absSrcDir, cleanWorkflow)
		absPath, absPathErr := filepath.Abs(workflowPath)
		if absPathErr != nil {
			return "", nil, fmt.Errorf("resolving workflow path: %w", absPathErr)
		}

		// Validate the resolved path is within the source directory using filepath.Rel
		relPath, relErr := filepath.Rel(absSrcDir, absPath)
		if relErr != nil || len(relPath) >= 2 && relPath[:2] == ".." {
			return "", nil, fmt.Errorf("workflow path must be within the workflows directory")
		}

		// Validate file exists and is a workflow file
		fileInfo, statErr := os.Lstat(absPath)
		if statErr != nil {
			return "", nil, fmt.Errorf("workflow file not found: %w", statErr)
		}

		// Reject symlinks to prevent path traversal
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("workflow file cannot be a symlink")
		}

		ext := filepath.Ext(cleanWorkflow)
		if ext != ".yml" && ext != ".yaml" {
			return "", nil, fmt.Errorf("workflow file must have .yml or .yaml extension")
		}

		workflows = []string{absPath}
	} else {
		// Discover all workflows
		workflows, err = DiscoverWorkflows(srcDir)
		if err != nil {
			return "", nil, err
		}

		if len(workflows) == 0 {
			return "", nil, fmt.Errorf("no workflow files found in %s", srcDir)
		}
	}

	tmpDir, err = os.MkdirTemp("", "detent-workflows-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp directory: %w", err)
	}

	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	// Process workflows in parallel using errgroup
	// Each workflow is independent, so parallel processing is safe
	var g errgroup.Group
	var mu sync.Mutex // Protects file writes to tmpDir

	// Set a reasonable concurrency limit to avoid resource exhaustion
	// This limits the number of concurrent workflow processing goroutines
	g.SetLimit(10)

	for _, wfPath := range workflows {
		wfPath := wfPath // Capture loop variable for goroutine
		g.Go(func() error {
			// Parse workflow file
			wf, parseErr := ParseWorkflowFile(wfPath)
			if parseErr != nil {
				return fmt.Errorf("parsing %s: %w", wfPath, parseErr)
			}

			// Apply modifications
			InjectContinueOnError(wf)
			InjectTimeouts(wf)

			// Marshal to YAML
			data, marshalErr := yaml.Marshal(wf)
			if marshalErr != nil {
				return fmt.Errorf("marshaling %s: %w", wfPath, marshalErr)
			}

			// Write to temp directory (mutex-protected to ensure thread-safe file writes)
			filename := filepath.Base(wfPath)
			mu.Lock()
			writeErr := os.WriteFile(filepath.Join(tmpDir, filename), data, 0o600)
			mu.Unlock()

			if writeErr != nil {
				return fmt.Errorf("writing %s: %w", filename, writeErr)
			}

			return nil
		})
	}

	// Wait for all goroutines to complete and check for errors
	if err := g.Wait(); err != nil {
		cleanup()
		return "", nil, err
	}

	return tmpDir, cleanup, nil
}
