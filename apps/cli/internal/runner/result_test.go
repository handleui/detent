package runner

import (
	"testing"

	coreerrors "github.com/detentsh/core/errors"
)

// Test HasErrors with no errors
func TestRunResult_HasErrors_NoErrors(t *testing.T) {
	// Create GroupedErrors with only warnings (no errors)
	warnings := []*coreerrors.ExtractedError{
		{
			Message:  "warning message",
			Severity: "warning",
		},
	}
	result := &RunResult{
		Grouped:              coreerrors.GroupByFile(warnings),
		GroupedComprehensive: coreerrors.GroupComprehensive(warnings, ""),
	}
	if result.HasErrors() {
		t.Error("expected no errors")
	}
}

// Test HasErrors with errors
func TestRunResult_HasErrors_WithErrors(t *testing.T) {
	// Create GroupedErrors with actual errors
	errors := []*coreerrors.ExtractedError{
		{
			Message:  "error message 1",
			Severity: "error",
		},
		{
			Message:  "error message 2",
			Severity: "error",
		},
	}
	result := &RunResult{
		Grouped:              coreerrors.GroupByFile(errors),
		GroupedComprehensive: coreerrors.GroupComprehensive(errors, ""),
	}
	if !result.HasErrors() {
		t.Error("expected errors to be detected")
	}
}

// Test HasErrors with nil GroupedComprehensive
func TestRunResult_HasErrors_NilGrouped(t *testing.T) {
	result := &RunResult{
		Grouped:              nil,
		GroupedComprehensive: nil,
	}
	if result.HasErrors() {
		t.Error("expected no errors for nil GroupedComprehensive")
	}
}

// Test Success with exit code 0 and no errors
func TestRunResult_Success_NoErrors(t *testing.T) {
	// Create GroupedErrors with only warnings
	warnings := []*coreerrors.ExtractedError{
		{
			Message:  "warning message",
			Severity: "warning",
		},
	}
	result := &RunResult{
		ExitCode:             0,
		Grouped:              coreerrors.GroupByFile(warnings),
		GroupedComprehensive: coreerrors.GroupComprehensive(warnings, ""),
	}
	if !result.Success() {
		t.Error("expected success")
	}
}

// Test Success with exit code 0 but has errors
func TestRunResult_Success_HasErrors(t *testing.T) {
	// Create GroupedErrors with actual errors
	errors := []*coreerrors.ExtractedError{
		{
			Message:  "error message",
			Severity: "error",
		},
	}
	result := &RunResult{
		ExitCode:             0,
		Grouped:              coreerrors.GroupByFile(errors),
		GroupedComprehensive: coreerrors.GroupComprehensive(errors, ""),
	}
	if result.Success() {
		t.Error("expected failure due to errors")
	}
}

// Test Success with non-zero exit code
func TestRunResult_Success_NonZeroExit(t *testing.T) {
	// Create GroupedErrors with only warnings
	warnings := []*coreerrors.ExtractedError{
		{
			Message:  "warning message",
			Severity: "warning",
		},
	}
	result := &RunResult{
		ExitCode:             1,
		Grouped:              coreerrors.GroupByFile(warnings),
		GroupedComprehensive: coreerrors.GroupComprehensive(warnings, ""),
	}
	if result.Success() {
		t.Error("expected failure due to exit code")
	}
}
