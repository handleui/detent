package errors

import (
	"testing"
)

// TestExtractorIntegration verifies that the refactored extractor produces
// the same output as the original implementation for various error types.
func TestExtractorIntegration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []*ExtractedError
	}{
		{
			name:  "Python exception with traceback",
			input: `  File "/app/main.py", line 42
ValueError: invalid literal for int() with base 10: 'abc'`,
			expected: []*ExtractedError{
				{
					Message:  "ValueError: invalid literal for int() with base 10: 'abc'",
					File:     "/app/main.py",
					Line:     42,
					Severity: "error",
					Category: CategoryRuntime,
					Source:   "python",
				},
			},
		},
		{
			name:  "Python exception standalone",
			input: "TypeError: unsupported operand type(s) for +: 'int' and 'str'",
			expected: []*ExtractedError{
				{
					Message:  "TypeError: unsupported operand type(s) for +: 'int' and 'str'",
					Severity: "error",
					Category: CategoryRuntime,
					Source:   "python",
				},
			},
		},
		{
			name: "ESLint with rule ID",
			input: `/app/src/index.js
  10:5  error  Unexpected var, use let or const instead  no-var`,
			expected: []*ExtractedError{
				{
					Message:  "Unexpected var, use let or const instead",
					File:     "/app/src/index.js",
					Line:     10,
					Column:   5,
					Severity: "error",
					RuleID:   "no-var",
					Category: CategoryLint,
					Source:   "eslint",
				},
			},
		},
		{
			name: "ESLint with scoped rule",
			input: `/app/src/index.ts
  15:8  error  Unsafe assignment of an any value  react/no-unsafe`,
			expected: []*ExtractedError{
				{
					Message:  "Unsafe assignment of an any value",
					File:     "/app/src/index.ts",
					Line:     15,
					Column:   8,
					Severity: "error",
					RuleID:   "react/no-unsafe",
					Category: CategoryLint,
					Source:   "eslint",
				},
			},
		},
		{
			name: "ESLint with @ scoped rule",
			input: `/app/src/component.tsx
  22:10  error  'foo' is assigned a value but never used  @typescript-eslint/no-unused-vars`,
			expected: []*ExtractedError{
				{
					Message:  "'foo' is assigned a value but never used",
					File:     "/app/src/component.tsx",
					Line:     22,
					Column:   10,
					Severity: "error",
					RuleID:   "@typescript-eslint/no-unused-vars",
					Category: CategoryLint,
					Source:   "eslint",
				},
			},
		},
		{
			name:  "Go compiler error",
			input: "main.go:25:10: undefined: foo",
			expected: []*ExtractedError{
				{
					Message:  "undefined: foo",
					File:     "main.go",
					Line:     25,
					Column:   10,
					Severity: "error",
					Category: CategoryCompile,
					Source:   "go",
				},
			},
		},
		{
			name:  "TypeScript error",
			input: "src/index.ts(42,10): error TS2749: Type 'string' is not assignable to type 'number'.",
			expected: []*ExtractedError{
				{
					Message:  "Type 'string' is not assignable to type 'number'.",
					File:     "src/index.ts",
					Line:     42,
					Column:   10,
					Severity: "error",
					RuleID:   "TS2749",
					Category: CategoryTypeCheck,
					Source:   "typescript",
				},
			},
		},
		{
			name: "Rust error with location",
			input: `error[E0308]: mismatched types
 --> src/main.rs:10:5`,
			expected: []*ExtractedError{
				{
					Message:  "mismatched types",
					File:     "src/main.rs",
					Line:     10,
					Column:   5,
					Severity: "error",
					RuleID:   "E0308",
					Category: CategoryCompile,
					Source:   "rust",
				},
			},
		},
		{
			name:  "Go test failure",
			input: "--- FAIL: TestFoo",
			expected: []*ExtractedError{
				{
					Message:  "Test failed: TestFoo",
					Severity: "error",
					Category: CategoryTest,
					Source:   "go-test",
				},
			},
		},
		{
			name:  "Node.js stack trace",
			input: "    at Function.Module._load (internal/modules/cjs/loader.js:892:14)",
			expected: []*ExtractedError{
				{
					Message:  "Node.js error",
					File:     "internal/modules/cjs/loader.js",
					Line:     892,
					Column:   14,
					Severity: "error",
					Category: CategoryRuntime,
					Source:   "nodejs",
				},
			},
		},
		{
			name:  "Docker error",
			input: "Error: No such container: abc123",
			expected: []*ExtractedError{
				{
					Message:  "No such container",
					Category: CategoryRuntime,
					Source:   "docker",
					Severity: "error",
				},
			},
		},
		{
			name:  "Exit code non-zero",
			input: "exit code 1",
			expected: []*ExtractedError{
				{
					Message:  "Exit code 1",
					Severity: "",
					Category: CategoryMetadata,
					Source:   SourceMetadata,
				},
			},
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

			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d errors, got %d", len(tt.expected), len(result))
			}

			for i, expected := range tt.expected {
				actual := result[i]

				if actual.Message != expected.Message {
					t.Errorf("Error %d: Message = %q, want %q", i, actual.Message, expected.Message)
				}
				if actual.File != expected.File {
					t.Errorf("Error %d: File = %q, want %q", i, actual.File, expected.File)
				}
				if actual.Line != expected.Line {
					t.Errorf("Error %d: Line = %d, want %d", i, actual.Line, expected.Line)
				}
				if actual.Column != expected.Column {
					t.Errorf("Error %d: Column = %d, want %d", i, actual.Column, expected.Column)
				}
				if actual.Severity != expected.Severity {
					t.Errorf("Error %d: Severity = %q, want %q", i, actual.Severity, expected.Severity)
				}
				if actual.RuleID != expected.RuleID {
					t.Errorf("Error %d: RuleID = %q, want %q", i, actual.RuleID, expected.RuleID)
				}
				if actual.Category != expected.Category {
					t.Errorf("Error %d: Category = %q, want %q", i, actual.Category, expected.Category)
				}
				if actual.Source != expected.Source {
					t.Errorf("Error %d: Source = %q, want %q", i, actual.Source, expected.Source)
				}
			}
		})
	}
}

// TestExtractorDeduplication verifies that duplicate errors are properly deduplicated.
func TestExtractorDeduplication(t *testing.T) {
	input := `main.go:10:5: undefined: foo
main.go:10:5: undefined: foo
main.go:10:5: undefined: foo`

	extractor := NewExtractor()
	result := extractor.Extract(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 error after deduplication, got %d", len(result))
	}

	if result[0].Message != "undefined: foo" {
		t.Errorf("Message = %q, want %q", result[0].Message, "undefined: foo")
	}
}

// TestExtractorWorkflowContext verifies that workflow context is properly attached.
func TestExtractorWorkflowContext(t *testing.T) {
	input := `[CI/build] | main.go:10:5: undefined: foo`

	extractor := NewExtractor()
	result := extractor.Extract(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(result))
	}

	if result[0].WorkflowContext == nil {
		t.Fatal("Expected workflow context to be set")
	}

	if result[0].WorkflowContext.Job != "CI/build" {
		t.Errorf("Job = %q, want %q", result[0].WorkflowContext.Job, "CI/build")
	}
}

// TestExtractorMultilinePython verifies multi-line Python traceback handling.
func TestExtractorMultilinePython(t *testing.T) {
	input := `Traceback (most recent call last):
  File "/app/main.py", line 42, in <module>
    result = int('abc')
ValueError: invalid literal for int() with base 10: 'abc'`

	extractor := NewExtractor()
	result := extractor.Extract(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(result))
	}

	if result[0].Message != "ValueError: invalid literal for int() with base 10: 'abc'" {
		t.Errorf("Message = %q, want %q", result[0].Message, "ValueError: invalid literal for int() with base 10: 'abc'")
	}
	if result[0].File != "/app/main.py" {
		t.Errorf("File = %q, want %q", result[0].File, "/app/main.py")
	}
	if result[0].Line != 42 {
		t.Errorf("Line = %d, want %d", result[0].Line, 42)
	}
}

// TestExtractorMultilineRust verifies multi-line Rust error handling.
func TestExtractorMultilineRust(t *testing.T) {
	input := `error[E0308]: mismatched types
  --> src/main.rs:10:5
   |
10 |     foo
   |     ^^^ expected i32, found &str`

	extractor := NewExtractor()
	result := extractor.Extract(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(result))
	}

	if result[0].Message != "mismatched types" {
		t.Errorf("Message = %q, want %q", result[0].Message, "mismatched types")
	}
	if result[0].RuleID != "E0308" {
		t.Errorf("RuleID = %q, want %q", result[0].RuleID, "E0308")
	}
	if result[0].File != "src/main.rs" {
		t.Errorf("File = %q, want %q", result[0].File, "src/main.rs")
	}
	if result[0].Line != 10 {
		t.Errorf("Line = %d, want %d", result[0].Line, 10)
	}
}
