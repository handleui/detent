package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/detent/cli/internal/errors"
)

func TestFormatText(t *testing.T) {
	tests := []struct {
		name     string
		grouped  *errors.ComprehensiveErrorGroup
		validate func(t *testing.T, output string)
	}{
		{
			name: "no errors or warnings",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile:     map[string][]*errors.ExtractedError{},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile:     []*errors.ExtractedError{},
				Total:      0,
				Stats: errors.ErrorStats{
					ErrorCount:   0,
					WarningCount: 0,
					ByCategory:   map[errors.ErrorCategory]int{},
					BySource:     map[string]int{},
				},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "No problems found") {
					t.Error("expected 'No problems found' message")
				}
				if !strings.Contains(output, "✓") {
					t.Error("expected checkmark symbol")
				}
			},
		},
		{
			name: "single error with file",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{
					"main.go": {
						{
							Message:  "undefined: foo",
							File:     "main.go",
							Line:     10,
							Column:   5,
							Severity: "error",
							Category: errors.CategoryCompile,
						},
					},
				},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryCompile: {
						{
							Message:  "undefined: foo",
							File:     "main.go",
							Line:     10,
							Column:   5,
							Severity: "error",
							Category: errors.CategoryCompile,
						},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile:     []*errors.ExtractedError{},
				Total:      1,
				Stats: errors.ErrorStats{
					ErrorCount:   1,
					WarningCount: 0,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryCompile: 1,
					},
					BySource: map[string]int{},
				},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "undefined: foo") {
					t.Error("expected error message in output")
				}
				if !strings.Contains(output, "main.go") {
					t.Error("expected file name in output")
				}
				if !strings.Contains(output, "10:5") {
					t.Error("expected line:column in output")
				}
				if !strings.Contains(output, "Build Errors") {
					t.Error("expected 'Build Errors' category header")
				}
				if !strings.Contains(output, "1 error") {
					t.Error("expected error count in output")
				}
			},
		},
		{
			name: "multiple categories",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{
					"test.ts": {
						{Message: "lint issue", Severity: "warning", Category: errors.CategoryLint},
						{Message: "type error", Severity: "error", Category: errors.CategoryTypeCheck},
					},
				},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryLint: {
						{Message: "lint issue", File: "test.ts", Severity: "warning"},
					},
					errors.CategoryTypeCheck: {
						{Message: "type error", File: "test.ts", Severity: "error"},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile:     []*errors.ExtractedError{},
				Total:      2,
				Stats: errors.ErrorStats{
					ErrorCount:   1,
					WarningCount: 1,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryLint:      1,
						errors.CategoryTypeCheck: 1,
					},
					BySource: map[string]int{},
				},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "Lint Issues") {
					t.Error("expected 'Lint Issues' category")
				}
				if !strings.Contains(output, "Type Errors") {
					t.Error("expected 'Type Errors' category")
				}
				if !strings.Contains(output, "lint issue") {
					t.Error("expected lint message")
				}
				if !strings.Contains(output, "type error") {
					t.Error("expected type error message")
				}
			},
		},
		{
			name: "errors with and without files",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{
					"app.js": {
						{Message: "syntax error", File: "app.js", Severity: "error", Category: errors.CategoryCompile},
					},
				},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryCompile: {
						{Message: "syntax error", File: "app.js", Severity: "error"},
					},
					errors.CategoryUnknown: {
						{Message: "generic error", Severity: "error"},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile: []*errors.ExtractedError{
					{Message: "generic error", Severity: "error", Category: errors.CategoryUnknown},
				},
				Total: 2,
				Stats: errors.ErrorStats{
					ErrorCount:   2,
					WarningCount: 0,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryCompile: 1,
						errors.CategoryUnknown: 1,
					},
					BySource: map[string]int{},
				},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "app.js") {
					t.Error("expected file with error")
				}
				if !strings.Contains(output, "syntax error") {
					t.Error("expected file-specific error")
				}
				if !strings.Contains(output, "generic error") {
					t.Error("expected generic error")
				}
			},
		},
		{
			name: "errors with rule IDs",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{
					"code.ts": {
						{
							Message:  "Type error",
							File:     "code.ts",
							Line:     5,
							Column:   10,
							Severity: "error",
							RuleID:   "TS2322",
							Category: errors.CategoryTypeCheck,
						},
					},
				},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryTypeCheck: {
						{
							Message:  "Type error",
							File:     "code.ts",
							Line:     5,
							Column:   10,
							Severity: "error",
							RuleID:   "TS2322",
						},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile:     []*errors.ExtractedError{},
				Total:      1,
				Stats: errors.ErrorStats{
					ErrorCount:   1,
					WarningCount: 0,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryTypeCheck: 1,
					},
					BySource: map[string]int{},
				},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "[TS2322]") {
					t.Error("expected rule ID in brackets")
				}
				if !strings.Contains(output, "Type error [TS2322]") {
					t.Error("expected message with rule ID")
				}
			},
		},
		{
			name: "warnings only",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{
					"lib.js": {
						{Message: "unused variable", File: "lib.js", Severity: "warning", Category: errors.CategoryLint},
					},
				},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryLint: {
						{Message: "unused variable", File: "lib.js", Severity: "warning"},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile:     []*errors.ExtractedError{},
				Total:      1,
				Stats: errors.ErrorStats{
					ErrorCount:   0,
					WarningCount: 1,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryLint: 1,
					},
					BySource: map[string]int{},
				},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "0 error") {
					t.Error("expected 0 errors")
				}
				if !strings.Contains(output, "1 warning") {
					t.Error("expected 1 warning")
				}
				if !strings.Contains(output, "unused variable") {
					t.Error("expected warning message")
				}
			},
		},
		{
			name: "multiple files same category",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{
					"file1.go": {
						{Message: "error 1", File: "file1.go", Severity: "error", Category: errors.CategoryCompile},
					},
					"file2.go": {
						{Message: "error 2", File: "file2.go", Severity: "error", Category: errors.CategoryCompile},
					},
				},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryCompile: {
						{Message: "error 1", File: "file1.go", Severity: "error"},
						{Message: "error 2", File: "file2.go", Severity: "error"},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile:     []*errors.ExtractedError{},
				Total:      2,
				Stats: errors.ErrorStats{
					ErrorCount:   2,
					WarningCount: 0,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryCompile: 2,
					},
					BySource: map[string]int{},
				},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "file1.go") {
					t.Error("expected file1.go in output")
				}
				if !strings.Contains(output, "file2.go") {
					t.Error("expected file2.go in output")
				}
				if !strings.Contains(output, "2 files") {
					t.Error("expected file count")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			FormatText(&buf, tt.grouped)
			output := buf.String()

			if output == "" {
				t.Fatal("FormatText() produced empty output")
			}

			tt.validate(t, output)
		})
	}
}

func TestGetCategoryColor(t *testing.T) {
	tests := []struct {
		name     string
		category errors.ErrorCategory
		expected string
	}{
		{
			name:     "lint category",
			category: errors.CategoryLint,
			expected: colorYellow,
		},
		{
			name:     "type check category",
			category: errors.CategoryTypeCheck,
			expected: colorRed,
		},
		{
			name:     "test category",
			category: errors.CategoryTest,
			expected: colorRed,
		},
		{
			name:     "compile category",
			category: errors.CategoryCompile,
			expected: colorRed,
		},
		{
			name:     "runtime category",
			category: errors.CategoryRuntime,
			expected: colorRed,
		},
		{
			name:     "unknown category",
			category: errors.CategoryUnknown,
			expected: colorYellow,
		},
		{
			name:     "metadata category",
			category: errors.CategoryMetadata,
			expected: colorYellow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCategoryColor(tt.category)
			if result != tt.expected {
				t.Errorf("getCategoryColor(%q) = %q, want %q", tt.category, result, tt.expected)
			}
		})
	}
}

func TestGetSeverityColor(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		expected string
	}{
		{
			name:     "error severity",
			severity: "error",
			expected: colorRed,
		},
		{
			name:     "warning severity",
			severity: "warning",
			expected: colorYellow,
		},
		{
			name:     "unknown severity",
			severity: "info",
			expected: colorGray,
		},
		{
			name:     "empty severity",
			severity: "",
			expected: colorGray,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSeverityColor(tt.severity)
			if result != tt.expected {
				t.Errorf("getSeverityColor(%q) = %q, want %q", tt.severity, result, tt.expected)
			}
		})
	}
}

func TestGetSeveritySymbol(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		expected string
	}{
		{
			name:     "error symbol",
			severity: "error",
			expected: "✖",
		},
		{
			name:     "warning symbol",
			severity: "warning",
			expected: "⚠",
		},
		{
			name:     "unknown severity symbol",
			severity: "info",
			expected: "●",
		},
		{
			name:     "empty severity symbol",
			severity: "",
			expected: "●",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSeveritySymbol(tt.severity)
			if result != tt.expected {
				t.Errorf("getSeveritySymbol(%q) = %q, want %q", tt.severity, result, tt.expected)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected string
	}{
		{
			name:     "zero count",
			count:    0,
			expected: "s",
		},
		{
			name:     "one count",
			count:    1,
			expected: "",
		},
		{
			name:     "two count",
			count:    2,
			expected: "s",
		},
		{
			name:     "large count",
			count:    100,
			expected: "s",
		},
		{
			name:     "negative count",
			count:    -1,
			expected: "s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := plural(tt.count)
			if result != tt.expected {
				t.Errorf("plural(%d) = %q, want %q", tt.count, result, tt.expected)
			}
		})
	}
}

func TestCountBySeverity(t *testing.T) {
	tests := []struct {
		name     string
		errs     []*errors.ExtractedError
		severity string
		expected int
	}{
		{
			name:     "empty list",
			errs:     []*errors.ExtractedError{},
			severity: "error",
			expected: 0,
		},
		{
			name: "all errors",
			errs: []*errors.ExtractedError{
				{Severity: "error"},
				{Severity: "error"},
				{Severity: "error"},
			},
			severity: "error",
			expected: 3,
		},
		{
			name: "all warnings",
			errs: []*errors.ExtractedError{
				{Severity: "warning"},
				{Severity: "warning"},
			},
			severity: "warning",
			expected: 2,
		},
		{
			name: "mixed severities count errors",
			errs: []*errors.ExtractedError{
				{Severity: "error"},
				{Severity: "warning"},
				{Severity: "error"},
				{Severity: "warning"},
				{Severity: "error"},
			},
			severity: "error",
			expected: 3,
		},
		{
			name: "mixed severities count warnings",
			errs: []*errors.ExtractedError{
				{Severity: "error"},
				{Severity: "warning"},
				{Severity: "error"},
				{Severity: "warning"},
				{Severity: "error"},
			},
			severity: "warning",
			expected: 2,
		},
		{
			name: "no matches",
			errs: []*errors.ExtractedError{
				{Severity: "error"},
				{Severity: "error"},
			},
			severity: "warning",
			expected: 0,
		},
		{
			name: "case sensitive",
			errs: []*errors.ExtractedError{
				{Severity: "Error"},
				{Severity: "ERROR"},
			},
			severity: "error",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countBySeverity(tt.errs, tt.severity)
			if result != tt.expected {
				t.Errorf("countBySeverity() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestFormatLocation(t *testing.T) {
	tests := []struct {
		name     string
		err      *errors.ExtractedError
		expected string
	}{
		{
			name: "line and column",
			err: &errors.ExtractedError{
				Line:   42,
				Column: 10,
			},
			expected: "42:10",
		},
		{
			name: "line only",
			err: &errors.ExtractedError{
				Line:   15,
				Column: 0,
			},
			expected: "15",
		},
		{
			name: "no line or column",
			err: &errors.ExtractedError{
				Line:   0,
				Column: 0,
			},
			expected: "",
		},
		{
			name: "column without line",
			err: &errors.ExtractedError{
				Line:   0,
				Column: 5,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLocation(tt.err)
			if result != tt.expected {
				t.Errorf("formatLocation() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		err      *errors.ExtractedError
		validate func(t *testing.T, output string)
	}{
		{
			name: "error with all fields",
			err: &errors.ExtractedError{
				Message:  "Type error",
				File:     "test.ts",
				Line:     10,
				Column:   5,
				Severity: "error",
				RuleID:   "TS2322",
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "10:5") {
					t.Error("expected location in output")
				}
				if !strings.Contains(output, "Type error [TS2322]") {
					t.Error("expected message with rule ID")
				}
				if !strings.Contains(output, "✖") {
					t.Error("expected error symbol")
				}
			},
		},
		{
			name: "warning without rule ID",
			err: &errors.ExtractedError{
				Message:  "Unused variable",
				File:     "app.js",
				Line:     5,
				Severity: "warning",
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "5:") {
					t.Error("expected line number")
				}
				if !strings.Contains(output, "Unused variable") {
					t.Error("expected message")
				}
				if !strings.Contains(output, "⚠") {
					t.Error("expected warning symbol")
				}
			},
		},
		{
			name: "error without location",
			err: &errors.ExtractedError{
				Message:  "Build failed",
				Severity: "error",
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "Build failed") {
					t.Error("expected message in output")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatError(&buf, tt.err)
			output := buf.String()

			if output == "" {
				t.Fatal("formatError() produced empty output")
			}

			tt.validate(t, output)
		})
	}
}

func TestDivider(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		expected string
	}{
		{
			name:     "width 0",
			width:    0,
			expected: "",
		},
		{
			name:     "width 1",
			width:    1,
			expected: "─",
		},
		{
			name:     "width 5",
			width:    5,
			expected: "─────",
		},
		{
			name:     "width 60",
			width:    60,
			expected: strings.Repeat("─", 60),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := divider(tt.width)
			if result != tt.expected {
				t.Errorf("divider(%d) length = %d, want %d", tt.width, len(result), len(tt.expected))
			}
			if result != tt.expected {
				t.Errorf("divider(%d) = %q, want %q", tt.width, result, tt.expected)
			}
		})
	}
}

func TestCategoryOrder(t *testing.T) {
	// Verify categoryOrder contains all expected categories in the right order
	expectedOrder := []errors.ErrorCategory{
		errors.CategoryLint,
		errors.CategoryTypeCheck,
		errors.CategoryTest,
		errors.CategoryCompile,
		errors.CategoryRuntime,
		errors.CategoryMetadata,
		errors.CategoryUnknown,
	}

	if len(categoryOrder) != len(expectedOrder) {
		t.Errorf("categoryOrder length = %d, want %d", len(categoryOrder), len(expectedOrder))
	}

	for i, expected := range expectedOrder {
		if i >= len(categoryOrder) {
			break
		}
		if categoryOrder[i] != expected {
			t.Errorf("categoryOrder[%d] = %q, want %q", i, categoryOrder[i], expected)
		}
	}
}

func TestCategoryNames(t *testing.T) {
	// Verify all categories have names
	expectedNames := map[errors.ErrorCategory]string{
		errors.CategoryLint:      "Lint Issues",
		errors.CategoryTypeCheck: "Type Errors",
		errors.CategoryTest:      "Test Failures",
		errors.CategoryCompile:   "Build Errors",
		errors.CategoryRuntime:   "Runtime Errors",
		errors.CategoryMetadata:  "Metadata Issues",
		errors.CategoryUnknown:   "Other Issues",
	}

	for category, expectedName := range expectedNames {
		if name, ok := categoryNames[category]; !ok {
			t.Errorf("categoryNames missing entry for %q", category)
		} else if name != expectedName {
			t.Errorf("categoryNames[%q] = %q, want %q", category, name, expectedName)
		}
	}
}

func TestFormatText_CategoryOrderRespected(t *testing.T) {
	// Create errors in reverse order and verify they're displayed in correct order
	grouped := &errors.ComprehensiveErrorGroup{
		ByFile: map[string][]*errors.ExtractedError{},
		ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
			errors.CategoryUnknown:   {{Message: "unknown", Severity: "error"}},
			errors.CategoryRuntime:   {{Message: "runtime", Severity: "error"}},
			errors.CategoryCompile:   {{Message: "compile", Severity: "error"}},
			errors.CategoryTest:      {{Message: "test", Severity: "error"}},
			errors.CategoryTypeCheck: {{Message: "typecheck", Severity: "error"}},
			errors.CategoryLint:      {{Message: "lint", Severity: "warning"}},
		},
		ByWorkflow: map[string][]*errors.ExtractedError{},
		NoFile:     []*errors.ExtractedError{},
		Total:      6,
		Stats: errors.ErrorStats{
			ErrorCount:   5,
			WarningCount: 1,
			ByCategory:   map[errors.ErrorCategory]int{},
			BySource:     map[string]int{},
		},
	}

	var buf bytes.Buffer
	FormatText(&buf, grouped)
	output := buf.String()

	// Find positions of category headers
	lintPos := strings.Index(output, "Lint Issues")
	typePos := strings.Index(output, "Type Errors")
	testPos := strings.Index(output, "Test Failures")
	compilePos := strings.Index(output, "Build Errors")
	runtimePos := strings.Index(output, "Runtime Errors")

	// Verify order
	if !(lintPos < typePos && typePos < testPos && testPos < compilePos && compilePos < runtimePos) {
		t.Error("categories are not in the correct order")
	}
}

func TestFormatText_EmptyCategories(t *testing.T) {
	// Categories with no errors should not appear in output
	grouped := &errors.ComprehensiveErrorGroup{
		ByFile: map[string][]*errors.ExtractedError{},
		ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
			errors.CategoryLint: {{Message: "lint", Severity: "warning"}},
		},
		ByWorkflow: map[string][]*errors.ExtractedError{},
		NoFile:     []*errors.ExtractedError{},
		Total:      1,
		Stats: errors.ErrorStats{
			ErrorCount:   0,
			WarningCount: 1,
			ByCategory:   map[errors.ErrorCategory]int{errors.CategoryLint: 1},
			BySource:     map[string]int{},
		},
	}

	var buf bytes.Buffer
	FormatText(&buf, grouped)
	output := buf.String()

	// Should only have Lint Issues, not other categories
	if !strings.Contains(output, "Lint Issues") {
		t.Error("expected 'Lint Issues' in output")
	}
	if strings.Contains(output, "Type Errors") {
		t.Error("did not expect 'Type Errors' in output")
	}
	if strings.Contains(output, "Test Failures") {
		t.Error("did not expect 'Test Failures' in output")
	}
}
