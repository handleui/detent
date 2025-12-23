package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/act"
	"github.com/detent/cli/internal/docker"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/output"
	"github.com/detent/cli/internal/persistence"
	"github.com/detent/cli/internal/signal"
	"github.com/detent/cli/internal/tui"
	"github.com/detent/cli/internal/workflow"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

const (
	actTimeout               = 30 * time.Minute
	preflightVisualDelay     = 200 * time.Millisecond
	preflightTransitionDelay = 100 * time.Millisecond
	preflightCompletionPause = 300 * time.Millisecond
)

var (
	workflowsDir string
	outputFormat string
	event        string
	verbose      bool

	// Pre-compiled regex for parseActProgress
	jobStepPattern = regexp.MustCompile(`^\[([^\]]+)\]\s+(.+)`)
)

var checkCmd = &cobra.Command{
	Use:   "check [repo-path]",
	Short: "Check workflows for errors by running them locally",
	Long: `Runs GitHub Actions locally using act, injecting continue-on-error
to ensure all steps run. Extracts and groups errors by file for debugging.`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runCheck,
	SilenceUsage:  true, // Don't show usage on runtime errors
	SilenceErrors: true, // We handle errors ourselves
}

func init() {
	checkCmd.Flags().StringVarP(&workflowsDir, "workflows", "w", ".github/workflows", "Path to workflows directory")
	checkCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json, json-detailed")
	checkCmd.Flags().StringVarP(&event, "event", "e", "push", "Event to trigger")
	checkCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show act logs in real-time")
}

func runCheck(cmd *cobra.Command, args []string) error {
	if outputFormat != "text" && outputFormat != "json" && outputFormat != "json-detailed" {
		return fmt.Errorf("invalid output format %q: must be 'text', 'json', or 'json-detailed'", outputFormat)
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

	baseCtx := cmd.Context()
	ctx, cancel := context.WithTimeout(baseCtx, actTimeout)
	defer cancel()

	// Check if we're in a TTY and not in verbose mode (use TUI)
	useTUI := !verbose && isatty.IsTerminal(os.Stdout.Fd())

	var tmpDir string
	var cleanup func()

	if useTUI {
		// Run pre-flight checks with visual feedback
		tmpDir, cleanup, err = runPreflightChecks(ctx, workflowPath)
		if err != nil {
			return err
		}
		defer cleanup()
	} else {
		// Traditional flow without pre-flight display
		tmpDir, cleanup, err = workflow.PrepareWorkflows(workflowPath)
		if err != nil {
			return fmt.Errorf("preparing workflows: %w", err)
		}
		defer cleanup()
	}

	var result *act.RunResult
	var grouped *internalerrors.GroupedErrors
	var cancelled bool

	if useTUI {
		result, grouped, cancelled, err = runWithTUIAndExtract(ctx, absRepoPath, tmpDir)
		// Check for cancellation in TUI mode
		if cancelled {
			signal.PrintCancellationMessage("check")
			return nil
		}
	} else {
		result, err = runWithoutTUI(ctx, absRepoPath, tmpDir)
	}

	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			signal.PrintCancellationMessage("check")
			return nil
		}
		return fmt.Errorf("running act: %w", err)
	}

	// Extract errors if not already done (TUI mode extracts early)
	var extracted []*internalerrors.ExtractedError
	if grouped == nil {
		combinedOutput := result.Stdout + result.Stderr
		extractor := internalerrors.NewExtractor()
		extracted = extractor.Extract(combinedOutput)
		grouped = internalerrors.GroupByFileWithBase(extracted, absRepoPath)
	} else {
		// If grouped exists, flatten it to get extracted errors for persistence
		for _, errs := range grouped.ByFile {
			extracted = append(extracted, errs...)
		}
		extracted = append(extracted, grouped.NoFile...)
	}

	// Persist results to .detent/ directory
	recorder, err := persistence.NewRecorder(absRepoPath, workflowPath, event)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to initialize persistence: %v\n", err)
	} else {
		// Record all findings
		for _, finding := range extracted {
			if err := recorder.RecordFinding(finding); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to record finding: %v\n", err)
			}
		}

		// Finalize the run with exit code
		if err := recorder.Finalize(result.ExitCode); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to finalize persistence: %v\n", err)
		} else if !useTUI && outputFormat == "text" {
			// Inform user of the output location
			_, _ = fmt.Fprintf(os.Stderr, "\nResults saved to: %s\n", recorder.GetOutputPath())
		}
	}

	// Only print error report in non-TUI mode (TUI shows it in completion view)
	if !useTUI {
		switch outputFormat {
		case "json":
			if err := output.FormatJSON(os.Stdout, grouped); err != nil {
				return fmt.Errorf("formatting JSON output: %w", err)
			}
		case "json-detailed":
			// Use already-extracted errors to create comprehensive V2 grouping
			groupedV2 := internalerrors.GroupComprehensive(extracted, absRepoPath)
			if err := output.FormatJSONV2(os.Stdout, groupedV2); err != nil {
				return fmt.Errorf("formatting JSON detailed output: %w", err)
			}
		default:
			output.FormatText(os.Stdout, grouped)
		}
	}

	// Return error if act failed or if there are actual errors (not just warnings)
	if result.ExitCode != 0 {
		return fmt.Errorf("workflow execution failed with exit code %d", result.ExitCode)
	}

	// Check if there are any actual errors (not warnings)
	hasErrors := false
	for _, issues := range grouped.ByFile {
		for _, issue := range issues {
			if issue.Severity == "error" {
				hasErrors = true
				break
			}
		}
		if hasErrors {
			break
		}
	}

	if !hasErrors {
		for _, issue := range grouped.NoFile {
			if issue.Severity == "error" {
				hasErrors = true
				break
			}
		}
	}

	if hasErrors {
		return fmt.Errorf("found errors in workflow execution")
	}

	return nil
}

func runWithTUIAndExtract(ctx context.Context, repoPath, tmpDir string) (*act.RunResult, *internalerrors.GroupedErrors, bool, error) {
	model := tui.NewCheckModel()
	program := tea.NewProgram(&model) // Inline mode - no AltScreen

	logChan := make(chan string, 100)

	// Result structure to pass data from log processor to main thread
	type tuiResult struct {
		result  *act.RunResult
		grouped *internalerrors.GroupedErrors
		err     error
	}
	tuiResultChan := make(chan tuiResult, 1)

	// Start act in a goroutine
	go func() {
		cfg := &act.RunConfig{
			WorkflowPath: tmpDir,
			Event:        event,
			Verbose:      false,
			WorkDir:      repoPath,
			StreamOutput: false,
			LogChan:      logChan,
		}

		result, err := act.Run(ctx, cfg)
		close(logChan)

		// Send result through tuiResultChan after processing
		if err != nil {
			tuiResultChan <- tuiResult{err: err}
			program.Send(tui.ErrMsg(err))
			return
		}

		// Extract errors once before sending to TUI
		combinedOutput := result.Stdout + result.Stderr
		extractor := internalerrors.NewExtractor()
		extracted := extractor.Extract(combinedOutput)
		grouped := internalerrors.GroupByFileWithBase(extracted, repoPath)

		// Check if context was cancelled
		cancelled := errors.Is(ctx.Err(), context.Canceled)

		// Send to TUI for display
		program.Send(tui.DoneMsg{
			Duration:  result.Duration,
			ExitCode:  result.ExitCode,
			Errors:    grouped,
			Cancelled: cancelled,
		})

		// Send to result channel for return value
		tuiResultChan <- tuiResult{
			result:  result,
			grouped: grouped,
		}
	}()

	// Log processor forwards logs to TUI
	go func() {
		for line := range logChan {
			program.Send(tui.LogMsg(line))

			// Parse for progress information
			if progress := parseActProgress(line); progress != nil {
				program.Send(*progress)
			}
		}
	}()

	// Wait for TUI to finish
	finalModel, err := program.Run()
	if err != nil {
		return nil, nil, false, err
	}

	// Extract cancellation status from the model
	checkModel, ok := finalModel.(*tui.CheckModel)
	var wasCancelled bool
	if ok {
		wasCancelled = checkModel.Cancelled
	}

	// Get the result from the channel (already extracted and processed)
	result := <-tuiResultChan
	if result.err != nil {
		return nil, nil, false, result.err
	}

	return result.result, result.grouped, wasCancelled, nil
}

func runWithoutTUI(ctx context.Context, repoPath, tmpDir string) (*act.RunResult, error) {
	if verbose {
		_, _ = fmt.Fprintf(os.Stderr, "Repo path: %s\n", repoPath)
		_, _ = fmt.Fprintf(os.Stderr, "Modified workflows in: %s\n", tmpDir)
		_, _ = fmt.Fprintf(os.Stderr, "\n> Running workflows with act\n\n")
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Running workflows... ")
	}

	cfg := &act.RunConfig{
		WorkflowPath: tmpDir,
		Event:        event,
		Verbose:      verbose,
		WorkDir:      repoPath,
		StreamOutput: verbose,
	}

	result, err := act.Run(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if verbose {
		_, _ = fmt.Fprintf(os.Stderr, "\n> Completed in %s (exit code %d)\n", result.Duration, result.ExitCode)
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "done (%s)\n", result.Duration)
	}

	return result, nil
}

// parseActProgress extracts progress information from act output
func parseActProgress(line string) *tui.ProgressMsg {
	// Pattern: [Job Name/Step Name] or similar
	// act outputs lines like: "[job-name] 🚀  Start image..."
	// "[job-name]   ✅  Success - Step Name"
	// We'll parse these to extract current step info

	// Match job/step patterns using pre-compiled regex
	matches := jobStepPattern.FindStringSubmatch(line)

	if len(matches) >= 3 {
		jobName := matches[1]
		stepInfo := strings.TrimSpace(matches[2])

		// Clean up step info
		stepInfo = strings.TrimPrefix(stepInfo, "🚀  ")
		stepInfo = strings.TrimPrefix(stepInfo, "✅  ")
		stepInfo = strings.TrimPrefix(stepInfo, "❌  ")

		status := fmt.Sprintf("%s: %s", jobName, stepInfo)

		return &tui.ProgressMsg{
			Status:      status,
			CurrentStep: 0,
			TotalSteps:  0,
		}
	}

	return nil
}

// runPreflightChecks performs and displays pre-flight checks before running act
func runPreflightChecks(ctx context.Context, workflowPath string) (tmpDir string, cleanup func(), err error) {
	checks := []string{
		"Checking Docker availability",
		"Preparing workflows (injecting continue-on-error)",
		"Creating temporary workspace",
	}

	display := tui.NewPreflightDisplay(checks)
	display.Render()

	// Check 1: Docker availability
	time.Sleep(preflightVisualDelay)
	display.UpdateCheck("Checking Docker availability", "running", nil)
	display.Render()

	err = docker.IsAvailable(ctx)
	if err != nil {
		display.UpdateCheck("Checking Docker availability", "error", err)
		display.RenderFinal()
		return "", nil, fmt.Errorf("docker is not available: %w", err)
	}

	display.UpdateCheck("Checking Docker availability", "success", nil)
	display.Render()

	// Check 2: Prepare workflows
	time.Sleep(preflightTransitionDelay)
	display.UpdateCheck("Preparing workflows (injecting continue-on-error)", "running", nil)
	display.Render()

	tmpDir, cleanup, err = workflow.PrepareWorkflows(workflowPath)
	if err != nil {
		display.UpdateCheck("Preparing workflows (injecting continue-on-error)", "error", err)
		display.RenderFinal()
		return "", nil, fmt.Errorf("preparing workflows: %w", err)
	}

	display.UpdateCheck("Preparing workflows (injecting continue-on-error)", "success", nil)
	display.UpdateCheck("Creating temporary workspace", "success", nil)
	display.Render()

	// Small pause to show all checks passed
	time.Sleep(preflightCompletionPause)

	// Add a blank line before TUI starts
	fmt.Fprintln(os.Stderr)

	return tmpDir, cleanup, nil
}
