package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/detent/cli/internal/workflow"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

var injectCmd = &cobra.Command{
	Use:   "inject",
	Short: "Preview workflow modifications (dry-run)",
	Long: `Preview the continue-on-error and timeout injections that check command applies
to your workflow files. This is a dry-run command that shows modifications without
changing any files.

The check command automatically performs these injections:
  - continue-on-error: true (ensures all steps run)
  - timeout-minutes: 30 (prevents infinite hangs)

This command is useful for:
  - Understanding what detent does to your workflows
  - Debugging workflow modification issues
  - Verifying injection behavior before running check

Outputs modified YAML to stdout for inspection.`,
	Example: `  # Preview injections for all workflows
  detent inject

  # Preview specific workflow
  detent inject --workflow ci.yml

  # Save preview to file
  detent inject > modified-workflows.yml`,
	Args: cobra.NoArgs,
	RunE: runInject,
}

func runInject(cmd *cobra.Command, args []string) error {
	absRepoPath, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	workflowPath := filepath.Join(absRepoPath, workflowsDir)

	workflows, err := workflow.DiscoverWorkflows(workflowPath)
	if err != nil {
		return fmt.Errorf("discovering workflows: %w", err)
	}

	if len(workflows) == 0 {
		return fmt.Errorf("no workflow files found in %s", workflowPath)
	}

	for i, wfPath := range workflows {
		wf, err := workflow.ParseWorkflowFile(wfPath)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", wfPath, err)
		}

		// Apply the same injections that check command uses
		workflow.InjectContinueOnError(wf)
		workflow.InjectTimeouts(wf)

		// Marshal to YAML
		data, err := yaml.Marshal(wf)
		if err != nil {
			return fmt.Errorf("marshaling %s: %w", wfPath, err)
		}

		filename := filepath.Base(wfPath)

		// Output to stdout with separators
		if i > 0 {
			fmt.Println("\n---")
		}
		fmt.Printf("# File: %s\n", filename)
		fmt.Println(string(data))
	}

	return nil
}
