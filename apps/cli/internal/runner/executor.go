package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/detent/cli/internal/act"
	actparser "github.com/detent/cli/internal/ci/act"
	"github.com/detent/cli/internal/debug"
	internalerrors "github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/git"
	"github.com/detent/cli/internal/tui"
)

// ActExecutor handles running the act process and capturing output.
// It supports both TUI and non-TUI execution modes.
type ActExecutor struct {
	config       *RunConfig
	tmpDir       string
	worktreeInfo *git.WorktreeInfo
}

// ExecuteResult contains the result of act execution including processed errors.
type ExecuteResult struct {
	ActResult            *act.RunResult
	Extracted            []*internalerrors.ExtractedError
	Grouped              *internalerrors.GroupedErrors
	GroupedComprehensive *internalerrors.ComprehensiveErrorGroup
	StartTime            time.Time
	Duration             time.Duration
	Cancelled            bool
	CompletionOutput     string // TUI completion output to print after exit
}

// NewActExecutor creates a new ActExecutor with the given configuration.
func NewActExecutor(config *RunConfig, tmpDir string, worktreeInfo *git.WorktreeInfo) *ActExecutor {
	return &ActExecutor{
		config:       config,
		tmpDir:       tmpDir,
		worktreeInfo: worktreeInfo,
	}
}

// Execute runs the workflow using act and returns the raw result.
// This is for non-TUI mode where output is streamed directly.
func (e *ActExecutor) Execute(ctx context.Context) (*ExecuteResult, error) {
	if err := git.ValidateWorktreeInitialized(e.worktreeInfo); err != nil {
		return nil, err
	}

	startTime := time.Now()
	actConfig := e.buildActConfig(nil)

	actResult, err := act.Run(ctx, actConfig)
	if err != nil {
		return nil, err
	}

	return &ExecuteResult{
		ActResult: actResult,
		StartTime: startTime,
		Duration:  actResult.Duration,
		Cancelled: false,
	}, nil
}

// ExecuteWithTUI runs the workflow using act with TUI integration.
// It streams output to the log channel and sends progress updates to the TUI program.
// Returns the execution result, whether the run was cancelled, and any error.
func (e *ActExecutor) ExecuteWithTUI(ctx context.Context, logChan chan string, program *tea.Program) (*ExecuteResult, bool, error) {
	if err := git.ValidateWorktreeInitialized(e.worktreeInfo); err != nil {
		return nil, false, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	startTime := time.Now()
	actConfig := e.buildActConfig(logChan)

	resultChan := make(chan tuiExecuteResult, 1)

	var wg sync.WaitGroup
	wg.Add(2)

	e.startActRunnerGoroutine(ctx, actConfig, logChan, program, resultChan, &wg)
	e.startLogProcessorGoroutine(ctx, logChan, program, &wg)

	finalModel, err := program.Run()
	if err != nil {
		cancel()
		wg.Wait()
		return nil, false, err
	}

	checkModel, ok := finalModel.(*tui.CheckModel)
	var wasCancelled bool
	var completionOutput string
	if ok {
		wasCancelled = checkModel.Cancelled
		completionOutput = checkModel.GetCompletionOutput()
	}

	tuiRes := <-resultChan
	if tuiRes.err != nil {
		cancel()
		wg.Wait()
		return nil, false, tuiRes.err
	}

	wg.Wait()

	return &ExecuteResult{
		ActResult:            tuiRes.result,
		Extracted:            tuiRes.extracted,
		Grouped:              tuiRes.grouped,
		GroupedComprehensive: tuiRes.groupedComprehensive,
		StartTime:            startTime,
		Duration:             tuiRes.result.Duration,
		Cancelled:            wasCancelled,
		CompletionOutput:     completionOutput,
	}, wasCancelled, nil
}

// tuiExecuteResult encapsulates the result from the act runner goroutine.
type tuiExecuteResult struct {
	result               *act.RunResult
	extracted            []*internalerrors.ExtractedError
	grouped              *internalerrors.GroupedErrors
	groupedComprehensive *internalerrors.ComprehensiveErrorGroup
	err                  error
}

// startActRunnerGoroutine starts a goroutine to run act and send results.
// Error processing happens in this goroutine so errors can be sent to the TUI
// via DoneMsg before the TUI exits.
func (e *ActExecutor) startActRunnerGoroutine(
	ctx context.Context,
	actConfig *act.RunConfig,
	logChan chan string,
	program *tea.Program,
	resultChan chan tuiExecuteResult,
	wg *sync.WaitGroup,
) {
	go func() {
		defer wg.Done()
		defer close(logChan)
		defer func() {
			if rec := recover(); rec != nil {
				err := fmt.Errorf("act.Run panicked: %v", rec)
				resultChan <- tuiExecuteResult{err: err}
				sendToTUI(program, tui.ErrMsg(err))
			}
		}()

		result, err := act.Run(ctx, actConfig)
		if err != nil {
			resultChan <- tuiExecuteResult{err: err}
			sendToTUI(program, tui.ErrMsg(err))
			return
		}

		// Process errors in this goroutine so they can be sent to the TUI
		processor := NewErrorProcessor(e.config.RepoRoot)
		processed := processor.Process(result)

		cancelled := errors.Is(ctx.Err(), context.Canceled)

		// Send done message to TUI with processed errors
		program.Send(tui.DoneMsg{
			Duration:  result.Duration,
			ExitCode:  result.ExitCode,
			Errors:    processed.GroupedComprehensive,
			Cancelled: cancelled,
		})

		resultChan <- tuiExecuteResult{
			result:               result,
			extracted:            processed.Extracted,
			grouped:              processed.Grouped,
			groupedComprehensive: processed.GroupedComprehensive,
		}
	}()
}

// startLogProcessorGoroutine starts a goroutine to process log messages.
func (e *ActExecutor) startLogProcessorGoroutine(
	ctx context.Context,
	logChan chan string,
	program *tea.Program,
	wg *sync.WaitGroup,
) {
	parser := actparser.New()

	go func() {
		defer wg.Done()
		for {
			select {
			case line, ok := <-logChan:
				if !ok {
					return
				}
				sendToTUI(program, tui.LogMsg(line))
				if event, ok := parser.ParseLine(line); ok {
					debug.Log("Event: Job=%q Action=%q Success=%v", event.JobName, event.Action, event.Success)
					sendToTUI(program, tui.JobEventMsg{Event: event})
				}
			case <-ctx.Done():
				for range logChan {
				}
				return
			}
		}
	}()
}

// buildActConfig constructs an act.RunConfig with appropriate settings.
func (e *ActExecutor) buildActConfig(logChan chan string) *act.RunConfig {
	return &act.RunConfig{
		WorkflowPath: e.tmpDir,
		Event:        e.config.Event,
		Verbose:      false,
		WorkDir:      e.worktreeInfo.Path,
		StreamOutput: logChan == nil && e.config.StreamOutput,
		LogChan:      logChan,
	}
}

// sendToTUI sends a message to the TUI program in a non-blocking manner.
func sendToTUI(program *tea.Program, msg tea.Msg) {
	go program.Send(msg)
}
