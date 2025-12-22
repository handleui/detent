package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/detent/cli/internal/act"
	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/workflow"
	"github.com/spf13/cobra"
)

const actTimeout = 30 * time.Minute

var (
	workflowsDir string
	outputFormat string
	event        string
	verbose      bool
)

var parseCmd = &cobra.Command{
	Use:   "parse [repo-path]",
	Short: "Run act and parse errors from workflow output",
	Long: `Runs GitHub Actions locally using act, injecting continue-on-error
to ensure all steps run. Extracts and groups errors by file for debugging.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runParse,
}

func init() {
	parseCmd.Flags().StringVarP(&workflowsDir, "workflows", "w", ".github/workflows", "Path to workflows directory")
	parseCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json")
	parseCmd.Flags().StringVarP(&event, "event", "e", "push", "Event to trigger")
	parseCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}

func runParse(_ *cobra.Command, args []string) error {
	if outputFormat != "text" && outputFormat != "json" {
		return fmt.Errorf("invalid output format %q: must be 'text' or 'json'", outputFormat)
	}

	repoPath := "."
	if len(args) > 0 {
		repoPath = args[0]
	}

	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path: %w", err)
	}

	workflowPath := filepath.Join(absRepoPath, workflowsDir)

	if verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Repo path: %s\n", absRepoPath)
		_, _ = fmt.Fprintf(os.Stderr, "Workflows: %s\n", workflowPath)
	}

	tmpDir, cleanup, err := workflow.PrepareWorkflows(workflowPath)
	if err != nil {
		return fmt.Errorf("preparing workflows: %w", err)
	}
	defer cleanup()

	if verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Modified workflows in: %s\n", tmpDir)
	}

	cfg := &act.RunConfig{
		WorkflowPath: tmpDir,
		Event:        event,
		Verbose:      verbose,
		WorkDir:      absRepoPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), actTimeout)
	defer cancel()

	result, err := act.Run(ctx, cfg)
	if err != nil {
		return fmt.Errorf("running act: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Act completed in %s with exit code %d\n", result.Duration, result.ExitCode)
	}

	combinedOutput := result.Stdout + result.Stderr
	var extractor errors.Extractor
	extracted := extractor.Extract(combinedOutput)
	grouped := errors.GroupByFile(extracted)

	switch outputFormat {
	case "json":
		if err := output.FormatJSON(os.Stdout, grouped); err != nil {
			return fmt.Errorf("formatting JSON output: %w", err)
		}
	default:
		output.FormatText(os.Stdout, grouped)
	}

	return nil
}
