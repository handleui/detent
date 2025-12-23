package errors

import "testing"

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

// TestInferSeverityIntegration verifies that the extractor correctly assigns severity.
func TestInferSeverityIntegration(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedSeverity string
	}{
		{
			name:             "Go compile error gets error severity",
			input:            "main.go:10:5: undefined: foo",
			expectedSeverity: "error",
		},
		{
			name:             "TypeScript error gets error severity",
			input:            "src/index.ts(42,10): error TS2749: Type error",
			expectedSeverity: "error",
		},
		{
			name:             "Python runtime error gets error severity",
			input:            "ValueError: invalid value",
			expectedSeverity: "error",
		},
		{
			name: "Rust compile error gets error severity",
			input: `error[E0308]: mismatched types
 --> src/main.rs:10:5`,
			expectedSeverity: "error",
		},
		{
			name:             "Go test failure gets error severity",
			input:            "--- FAIL: TestFoo",
			expectedSeverity: "error",
		},
		{
			name: "ESLint error keeps error severity",
			input: `/app/src/index.js
  10:5  error  Message  rule-name`,
			expectedSeverity: "error",
		},
		{
			name: "ESLint warning keeps warning severity",
			input: `/app/src/index.js
  10:5  warning  Message  rule-name`,
			expectedSeverity: "warning",
		},
		{
			name:             "Docker error gets error severity",
			input:            "Error: No such container: abc123",
			expectedSeverity: "error",
		},
		{
			name:             "Generic error gets warning severity (CategoryUnknown)",
			input:            "error: something went wrong",
			expectedSeverity: "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewExtractor()
			result := extractor.Extract(tt.input)
			// Apply severity inference as post-processing (mimics cmd/check.go behavior)
			for _, err := range result {
				err.Severity = InferSeverity(err)
			}

			if len(result) == 0 {
				t.Fatal("Expected at least one error to be extracted")
			}

			if result[0].Severity != tt.expectedSeverity {
				t.Errorf("Severity = %q, want %q (Category: %q, Source: %q)",
					result[0].Severity, tt.expectedSeverity, result[0].Category, result[0].Source)
			}
		})
	}
}
