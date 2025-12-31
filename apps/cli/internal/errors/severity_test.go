package errors

import (
	"testing"
)

// TestInferSeverity verifies that severity is correctly inferred based on category.
func TestInferSeverity(t *testing.T) {
	tests := []struct {
		name     string
		err      *ExtractedError
		expected string
	}{
		{
			name: "Compile error",
			err: &ExtractedError{
				Category: CategoryCompile,
			},
			expected: "error",
		},
		{
			name: "Type check error",
			err: &ExtractedError{
				Category: CategoryTypeCheck,
			},
			expected: "error",
		},
		{
			name: "Test error",
			err: &ExtractedError{
				Category: CategoryTest,
			},
			expected: "error",
		},
		{
			name: "Runtime error",
			err: &ExtractedError{
				Category: CategoryRuntime,
			},
			expected: "error",
		},
		{
			name: "Lint issue",
			err: &ExtractedError{
				Category: CategoryLint,
			},
			expected: "warning",
		},
		{
			name: "Unknown issue",
			err: &ExtractedError{
				Category: CategoryUnknown,
			},
			expected: "warning",
		},
		{
			name: "Preserve ESLint error severity",
			err: &ExtractedError{
				Category: CategoryLint,
				Severity: "error",
				Source:   "eslint",
			},
			expected: "error",
		},
		{
			name: "Preserve ESLint warning severity",
			err: &ExtractedError{
				Category: CategoryLint,
				Severity: "warning",
				Source:   "eslint",
			},
			expected: "warning",
		},
		{
			name: "Preserve Docker error severity",
			err: &ExtractedError{
				Category: CategoryRuntime,
				Severity: "error",
				Source:   "docker",
			},
			expected: "error",
		},
		{
			name: "Empty category defaults to warning",
			err: &ExtractedError{
				Category: "",
			},
			expected: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferSeverity(tt.err)
			if result != tt.expected {
				t.Errorf("InferSeverity() = %q, want %q", result, tt.expected)
			}
		})
	}
}

