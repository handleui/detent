package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// InjectContinueOnError modifies a workflow to add continue-on-error: true to all steps
func InjectContinueOnError(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}
	for _, job := range wf.Jobs {
		if job == nil || job.Steps == nil {
			continue
		}
		for _, step := range job.Steps {
			if step == nil {
				continue
			}
			step.ContinueOnError = true
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
