package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/detent/cli/internal/workflow"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

var injectCmd = &cobra.Command{
	Use:   "inject [repo-path]",
	Short: "Preview workflow injections (dry-run)",
	Long: `Preview the continue-on-error and timeout injections that the check command applies.

This is a dry-run command that shows what modifications would be made to your workflow
files without actually changing them. The actual injection happens automatically when
you run the check command.

Outputs modified YAML to stdout for inspection.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInject,
}

func runInject(cmd *cobra.Command, args []string) error {
	repoPath := "."
	if len(args) > 0 {
		repoPath = args[0]
	}

	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path: %w", err)
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
