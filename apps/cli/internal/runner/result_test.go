package runner

import (
	"testing"

	internalerrors "github.com/detent/cli/internal/errors"
)

// Test HasErrors with no errors
func TestRunResult_HasErrors_NoErrors(t *testing.T) {
	// Create GroupedErrors with only warnings (no errors)
	warnings := []*internalerrors.ExtractedError{
		{
			Message:  "warning message",
			Severity: "warning",
		},
	}
	result := &RunResult{
		Grouped:              internalerrors.GroupByFile(warnings),
		GroupedComprehensive: internalerrors.GroupComprehensive(warnings, ""),
	}
	if result.HasErrors() {
		t.Error("expected no errors")
	}
}

// Test HasErrors with errors
func TestRunResult_HasErrors_WithErrors(t *testing.T) {
	// Create GroupedErrors with actual errors
	errors := []*internalerrors.ExtractedError{
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
		Grouped:              internalerrors.GroupByFile(errors),
		GroupedComprehensive: internalerrors.GroupComprehensive(errors, ""),
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
	warnings := []*internalerrors.ExtractedError{
		{
			Message:  "warning message",
			Severity: "warning",
		},
	}
	result := &RunResult{
		ExitCode:             0,
		Grouped:              internalerrors.GroupByFile(warnings),
		GroupedComprehensive: internalerrors.GroupComprehensive(warnings, ""),
	}
	if !result.Success() {
		t.Error("expected success")
	}
}

// Test Success with exit code 0 but has errors
func TestRunResult_Success_HasErrors(t *testing.T) {
	// Create GroupedErrors with actual errors
	errors := []*internalerrors.ExtractedError{
		{
			Message:  "error message",
			Severity: "error",
		},
	}
	result := &RunResult{
		ExitCode:             0,
		Grouped:              internalerrors.GroupByFile(errors),
		GroupedComprehensive: internalerrors.GroupComprehensive(errors, ""),
	}
	if result.Success() {
		t.Error("expected failure due to errors")
	}
}

// Test Success with non-zero exit code
func TestRunResult_Success_NonZeroExit(t *testing.T) {
	// Create GroupedErrors with only warnings
	warnings := []*internalerrors.ExtractedError{
		{
			Message:  "warning message",
			Severity: "warning",
		},
	}
	result := &RunResult{
		ExitCode:             1,
		Grouped:              internalerrors.GroupByFile(warnings),
		GroupedComprehensive: internalerrors.GroupComprehensive(warnings, ""),
	}
	if result.Success() {
		t.Error("expected failure due to exit code")
	}
}
