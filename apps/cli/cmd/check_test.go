package cmd

import (
	"testing"
	"time"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/runner"
)

func TestCheckCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "check command has correct use",
			wantUse: "check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if checkCmd.Use != tt.wantUse {
				t.Errorf("checkCmd.Use = %q, want %q", checkCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestCheckCommandFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		shorthand string
		wantType  string
	}{
		{
			name:      "output flag exists",
			flagName:  "output",
			shorthand: "o",
			wantType:  "string",
		},
		{
			name:      "event flag exists",
			flagName:  "event",
			shorthand: "e",
			wantType:  "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := checkCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("Flag %q not found", tt.flagName)
				return
			}

			if flag.Value.Type() != tt.wantType {
				t.Errorf("Flag %q has type %q, want %q", tt.flagName, flag.Value.Type(), tt.wantType)
			}

			if flag.Shorthand != tt.shorthand {
				t.Errorf("Flag %q has shorthand %q, want %q", tt.flagName, flag.Shorthand, tt.shorthand)
			}
		})
	}
}

func TestCheckCommandNoArgs(t *testing.T) {
	if checkCmd.Args == nil {
		t.Error("checkCmd.Args should be set to NoArgs")
	}
}

func TestCheckWorkflowStatus(t *testing.T) {
	tests := []struct {
		name     string
		result   *runner.RunResult
		wantErr  bool
		errCheck func(error) bool
	}{
		{
			name: "success - no errors",
			result: &runner.RunResult{
				ExitCode:  0,
				Extracted: []*errors.ExtractedError{},
			},
			wantErr: false,
		},
		{
			name: "workflow failed with exit code",
			result: &runner.RunResult{
				ExitCode: 1,
			},
			wantErr: true,
		},
		{
			name: "errors found",
			result: &runner.RunResult{
				ExitCode: 0,
				Extracted: []*errors.ExtractedError{
					{Severity: "error"},
				},
				GroupedComprehensive: &errors.ComprehensiveErrorGroup{
					Stats: errors.ErrorStats{
						ErrorCount: 1,
					},
				},
			},
			wantErr: true,
			errCheck: func(err error) bool {
				return err == ErrFoundErrors
			},
		},
		{
			name: "warnings only - no error",
			result: &runner.RunResult{
				ExitCode: 0,
				Extracted: []*errors.ExtractedError{
					{Severity: "warning"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkWorkflowStatus(tt.result)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkWorkflowStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.errCheck != nil && !tt.errCheck(err) {
				t.Errorf("checkWorkflowStatus() error = %v, failed custom check", err)
			}
		})
	}
}

func TestPrintCompletionSummary(t *testing.T) {
	tests := []struct {
		name   string
		result *runner.RunResult
	}{
		{
			name: "successful run",
			result: &runner.RunResult{
				ExitCode:  0,
				Duration:  5 * time.Second,
				Extracted: []*errors.ExtractedError{},
			},
		},
		{
			name: "run with errors",
			result: &runner.RunResult{
				ExitCode: 0,
				Duration: 10 * time.Second,
				Extracted: []*errors.ExtractedError{
					{Severity: "error", Message: "test error 1"},
					{Severity: "error", Message: "test error 2"},
				},
			},
		},
		{
			name: "run with warnings",
			result: &runner.RunResult{
				ExitCode: 0,
				Duration: 7 * time.Second,
				Extracted: []*errors.ExtractedError{
					{Severity: "warning", Message: "test warning"},
				},
			},
		},
		{
			name: "run with mixed errors and warnings",
			result: &runner.RunResult{
				ExitCode: 0,
				Duration: 12 * time.Second,
				Extracted: []*errors.ExtractedError{
					{Severity: "error", Message: "test error"},
					{Severity: "warning", Message: "test warning 1"},
					{Severity: "warning", Message: "test warning 2"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printCompletionSummary writes to stderr, we're just testing it doesn't panic
			printCompletionSummary(tt.result)
		})
	}
}

func TestPrintExitMessage(t *testing.T) {
	// Set StartTime for test
	StartTime = time.Now().Add(-5 * time.Second)

	tests := []struct {
		name   string
		result *runner.RunResult
	}{
		{
			name: "no errors",
			result: &runner.RunResult{
				Extracted: []*errors.ExtractedError{},
			},
		},
		{
			name: "single error",
			result: &runner.RunResult{
				Extracted: []*errors.ExtractedError{
					{Severity: "error", Message: "test error"},
				},
			},
		},
		{
			name: "multiple errors",
			result: &runner.RunResult{
				Extracted: []*errors.ExtractedError{
					{Severity: "error", Message: "test error 1"},
					{Severity: "error", Message: "test error 2"},
				},
			},
		},
		{
			name: "warnings don't count",
			result: &runner.RunResult{
				Extracted: []*errors.ExtractedError{
					{Severity: "warning", Message: "test warning"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printExitMessage writes to stderr, we're just testing it doesn't panic
			printExitMessage(tt.result)
		})
	}
}

func TestErrFoundErrorsConstant(t *testing.T) {
	expectedMsg := "found errors in workflow execution"
	if ErrFoundErrors.Error() != expectedMsg {
		t.Errorf("ErrFoundErrors.Error() = %q, want %q", ErrFoundErrors.Error(), expectedMsg)
	}
}

func TestCheckCommandSilenceSettings(t *testing.T) {
	if !checkCmd.SilenceUsage {
		t.Error("checkCmd.SilenceUsage should be true")
	}
	if !checkCmd.SilenceErrors {
		t.Error("checkCmd.SilenceErrors should be true")
	}
}

func TestLogChannelBufferSize(t *testing.T) {
	if logChannelBufferSize != 100 {
		t.Errorf("logChannelBufferSize = %d, want 100", logChannelBufferSize)
	}
}
