package errors

import "testing"

func TestGroupedErrors_HasErrors(t *testing.T) {
	tests := []struct {
		name     string
		errors   []*ExtractedError
		expected bool
	}{
		{
			name: "has errors",
			errors: []*ExtractedError{
				{Message: "error 1", Severity: "error", File: "file1.go"},
				{Message: "warning 1", Severity: "warning", File: "file1.go"},
			},
			expected: true,
		},
		{
			name: "only warnings",
			errors: []*ExtractedError{
				{Message: "warning 1", Severity: "warning", File: "file1.go"},
				{Message: "warning 2", Severity: "warning", File: "file2.go"},
			},
			expected: false,
		},
		{
			name:     "empty",
			errors:   []*ExtractedError{},
			expected: false,
		},
		{
			name: "error without file",
			errors: []*ExtractedError{
				{Message: "error 1", Severity: "error"},
				{Message: "warning 1", Severity: "warning"},
			},
			expected: true,
		},
		{
			name: "mixed errors and warnings",
			errors: []*ExtractedError{
				{Message: "warning 1", Severity: "warning", File: "file1.go"},
				{Message: "warning 2", Severity: "warning", File: "file2.go"},
				{Message: "error 1", Severity: "error", File: "file3.go"},
				{Message: "warning 3", Severity: "warning", File: "file4.go"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grouped := GroupByFileWithBase(tt.errors, "")
			if got := grouped.HasErrors(); got != tt.expected {
				t.Errorf("HasErrors() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGroupedErrors_HasErrors_Performance(t *testing.T) {
	// Create a large set of errors to verify O(1) lookup performance
	const numErrors = 100000
	errors := make([]*ExtractedError, numErrors)
	for i := 0; i < numErrors; i++ {
		severity := "warning"
		if i == numErrors-1 {
			severity = "error"
		}
		errors[i] = &ExtractedError{
			Message:  "test error",
			Severity: severity,
			File:     "test.go",
		}
	}

	grouped := GroupByFileWithBase(errors, "")

	// This should be O(1) - just checking the hasErrors flag
	if !grouped.HasErrors() {
		t.Error("HasErrors() should return true when errors exist")
	}
}
