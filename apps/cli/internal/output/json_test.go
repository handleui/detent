package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/detent/cli/internal/errors"
)

func TestFormatJSON(t *testing.T) {
	tests := []struct {
		name     string
		grouped  *errors.GroupedErrors
		validate func(t *testing.T, output string)
	}{
		{
			name: "empty grouped errors",
			grouped: &errors.GroupedErrors{
				ByFile: map[string][]*errors.ExtractedError{},
				NoFile: []*errors.ExtractedError{},
				Total:  0,
			},
			validate: func(t *testing.T, output string) {
				var result errors.GroupedErrors
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result.Total != 0 {
					t.Errorf("Total = %d, want 0", result.Total)
				}
				if len(result.ByFile) != 0 {
					t.Errorf("ByFile length = %d, want 0", len(result.ByFile))
				}
			},
		},
		{
			name: "single error with file",
			grouped: &errors.GroupedErrors{
				ByFile: map[string][]*errors.ExtractedError{
					"main.go": {
						{
							Message:  "undefined: foo",
							File:     "main.go",
							Line:     10,
							Column:   5,
							Severity: "error",
							Source:   errors.SourceGo,
							Category: errors.CategoryCompile,
						},
					},
				},
				NoFile: []*errors.ExtractedError{},
				Total:  1,
			},
			validate: func(t *testing.T, output string) {
				var result errors.GroupedErrors
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result.Total != 1 {
					t.Errorf("Total = %d, want 1", result.Total)
				}
				if len(result.ByFile) != 1 {
					t.Fatalf("ByFile length = %d, want 1", len(result.ByFile))
				}
				errs, ok := result.ByFile["main.go"]
				if !ok {
					t.Fatal("expected main.go in ByFile")
				}
				if len(errs) != 1 {
					t.Fatalf("main.go errors length = %d, want 1", len(errs))
				}
				if errs[0].Message != "undefined: foo" {
					t.Errorf("Message = %q, want %q", errs[0].Message, "undefined: foo")
				}
			},
		},
		{
			name: "multiple files with errors",
			grouped: &errors.GroupedErrors{
				ByFile: map[string][]*errors.ExtractedError{
					"file1.go": {
						{Message: "error 1", Severity: "error"},
						{Message: "warning 1", Severity: "warning"},
					},
					"file2.ts": {
						{Message: "type error", Severity: "error"},
					},
				},
				NoFile: []*errors.ExtractedError{},
				Total:  3,
			},
			validate: func(t *testing.T, output string) {
				var result errors.GroupedErrors
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result.Total != 3 {
					t.Errorf("Total = %d, want 3", result.Total)
				}
				if len(result.ByFile) != 2 {
					t.Fatalf("ByFile length = %d, want 2", len(result.ByFile))
				}
			},
		},
		{
			name: "errors without file location",
			grouped: &errors.GroupedErrors{
				ByFile: map[string][]*errors.ExtractedError{},
				NoFile: []*errors.ExtractedError{
					{Message: "generic error", Severity: "error"},
					{Message: "generic warning", Severity: "warning"},
				},
				Total: 2,
			},
			validate: func(t *testing.T, output string) {
				var result errors.GroupedErrors
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if len(result.NoFile) != 2 {
					t.Fatalf("NoFile length = %d, want 2", len(result.NoFile))
				}
				if result.NoFile[0].Message != "generic error" {
					t.Errorf("NoFile[0].Message = %q, want %q", result.NoFile[0].Message, "generic error")
				}
			},
		},
		{
			name: "complex error with all fields",
			grouped: &errors.GroupedErrors{
				ByFile: map[string][]*errors.ExtractedError{
					"test.ts": {
						{
							Message:    "Type 'string' is not assignable to type 'number'",
							File:       "test.ts",
							Line:       42,
							Column:     10,
							Severity:   "error",
							Raw:        "test.ts(42,10): error TS2322: Type 'string' is not assignable to type 'number'",
							StackTrace: "at Object.<anonymous> (test.ts:42:10)",
							RuleID:     "TS2322",
							Category:   errors.CategoryTypeCheck,
							Source:     errors.SourceTypeScript,
							WorkflowContext: &errors.WorkflowContext{
								Job:  "build",
								Step: "typecheck",
							},
							UnknownPattern: false,
						},
					},
				},
				NoFile: []*errors.ExtractedError{},
				Total:  1,
			},
			validate: func(t *testing.T, output string) {
				var result errors.GroupedErrors
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				errs := result.ByFile["test.ts"]
				if len(errs) != 1 {
					t.Fatalf("test.ts errors length = %d, want 1", len(errs))
				}
				err := errs[0]
				if err.Line != 42 {
					t.Errorf("Line = %d, want 42", err.Line)
				}
				if err.Column != 10 {
					t.Errorf("Column = %d, want 10", err.Column)
				}
				if err.RuleID != "TS2322" {
					t.Errorf("RuleID = %q, want %q", err.RuleID, "TS2322")
				}
				if err.Category != errors.CategoryTypeCheck {
					t.Errorf("Category = %q, want %q", err.Category, errors.CategoryTypeCheck)
				}
				if err.WorkflowContext == nil {
					t.Fatal("WorkflowContext is nil")
				}
				if err.WorkflowContext.Job != "build" {
					t.Errorf("WorkflowContext.Job = %q, want %q", err.WorkflowContext.Job, "build")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := FormatJSON(&buf, tt.grouped)
			if err != nil {
				t.Fatalf("FormatJSON() error = %v", err)
			}

			output := buf.String()
			if output == "" {
				t.Fatal("FormatJSON() produced empty output")
			}

			tt.validate(t, output)
		})
	}
}

func TestFormatJSON_IndentedOutput(t *testing.T) {
	grouped := &errors.GroupedErrors{
		ByFile: map[string][]*errors.ExtractedError{
			"test.go": {
				{Message: "test error", Severity: "error"},
			},
		},
		NoFile: []*errors.ExtractedError{},
		Total:  1,
	}

	var buf bytes.Buffer
	err := FormatJSON(&buf, grouped)
	if err != nil {
		t.Fatalf("FormatJSON() error = %v", err)
	}

	output := buf.String()
	// Check that output is indented (contains newlines and spaces)
	if len(output) < 10 {
		t.Error("output appears to not be indented")
	}

	// Verify it contains proper JSON structure with indentation
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestFormatJSONDetailed(t *testing.T) {
	tests := []struct {
		name     string
		grouped  *errors.ComprehensiveErrorGroup
		validate func(t *testing.T, output string)
	}{
		{
			name: "empty comprehensive group",
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
					UniqueFiles:  0,
					UniqueRules:  0,
				},
			},
			validate: func(t *testing.T, output string) {
				var result errors.ComprehensiveErrorGroup
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result.Total != 0 {
					t.Errorf("Total = %d, want 0", result.Total)
				}
				if result.Stats.ErrorCount != 0 {
					t.Errorf("Stats.ErrorCount = %d, want 0", result.Stats.ErrorCount)
				}
			},
		},
		{
			name: "comprehensive group with statistics",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{
					"file1.go": {
						{Message: "error 1", Severity: "error", Category: errors.CategoryLint},
					},
					"file2.ts": {
						{Message: "warning 1", Severity: "warning", Category: errors.CategoryTypeCheck},
					},
				},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryLint: {
						{Message: "error 1", Severity: "error"},
					},
					errors.CategoryTypeCheck: {
						{Message: "warning 1", Severity: "warning"},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{
					"lint-job": {
						{Message: "error 1", Severity: "error"},
					},
					"typecheck-job": {
						{Message: "warning 1", Severity: "warning"},
					},
				},
				NoFile: []*errors.ExtractedError{},
				Total:  2,
				Stats: errors.ErrorStats{
					ErrorCount:   1,
					WarningCount: 1,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryLint:      1,
						errors.CategoryTypeCheck: 1,
					},
					BySource: map[string]int{
						errors.SourceESLint:     1,
						errors.SourceTypeScript: 1,
					},
					UniqueFiles: 2,
					UniqueRules: 0,
				},
			},
			validate: func(t *testing.T, output string) {
				var result errors.ComprehensiveErrorGroup
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result.Total != 2 {
					t.Errorf("Total = %d, want 2", result.Total)
				}
				if result.Stats.ErrorCount != 1 {
					t.Errorf("Stats.ErrorCount = %d, want 1", result.Stats.ErrorCount)
				}
				if result.Stats.WarningCount != 1 {
					t.Errorf("Stats.WarningCount = %d, want 1", result.Stats.WarningCount)
				}
				if result.Stats.UniqueFiles != 2 {
					t.Errorf("Stats.UniqueFiles = %d, want 2", result.Stats.UniqueFiles)
				}
				if len(result.ByCategory) != 2 {
					t.Errorf("ByCategory length = %d, want 2", len(result.ByCategory))
				}
				if len(result.ByWorkflow) != 2 {
					t.Errorf("ByWorkflow length = %d, want 2", len(result.ByWorkflow))
				}
			},
		},
		{
			name: "workflow grouping",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile:     map[string][]*errors.ExtractedError{},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{},
				ByWorkflow: map[string][]*errors.ExtractedError{
					"build": {
						{Message: "compile error", Severity: "error"},
						{Message: "another error", Severity: "error"},
					},
					"test": {
						{Message: "test failure", Severity: "error"},
					},
				},
				NoFile: []*errors.ExtractedError{},
				Total:  3,
				Stats: errors.ErrorStats{
					ErrorCount:   3,
					WarningCount: 0,
					ByCategory:   map[errors.ErrorCategory]int{},
					BySource:     map[string]int{},
					UniqueFiles:  0,
					UniqueRules:  0,
				},
			},
			validate: func(t *testing.T, output string) {
				var result errors.ComprehensiveErrorGroup
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if len(result.ByWorkflow) != 2 {
					t.Errorf("ByWorkflow length = %d, want 2", len(result.ByWorkflow))
				}
				buildErrs := result.ByWorkflow["build"]
				if len(buildErrs) != 2 {
					t.Errorf("build workflow errors length = %d, want 2", len(buildErrs))
				}
				testErrs := result.ByWorkflow["test"]
				if len(testErrs) != 1 {
					t.Errorf("test workflow errors length = %d, want 1", len(testErrs))
				}
			},
		},
		{
			name: "category statistics",
			grouped: &errors.ComprehensiveErrorGroup{
				ByFile: map[string][]*errors.ExtractedError{},
				ByCategory: map[errors.ErrorCategory][]*errors.ExtractedError{
					errors.CategoryLint: {
						{Message: "lint 1", Severity: "warning"},
						{Message: "lint 2", Severity: "warning"},
					},
					errors.CategoryTest: {
						{Message: "test fail", Severity: "error"},
					},
					errors.CategoryCompile: {
						{Message: "compile error 1", Severity: "error"},
						{Message: "compile error 2", Severity: "error"},
					},
				},
				ByWorkflow: map[string][]*errors.ExtractedError{},
				NoFile:     []*errors.ExtractedError{},
				Total:      5,
				Stats: errors.ErrorStats{
					ErrorCount:   3,
					WarningCount: 2,
					ByCategory: map[errors.ErrorCategory]int{
						errors.CategoryLint:    2,
						errors.CategoryTest:    1,
						errors.CategoryCompile: 2,
					},
					BySource:    map[string]int{},
					UniqueFiles: 0,
					UniqueRules: 0,
				},
			},
			validate: func(t *testing.T, output string) {
				var result errors.ComprehensiveErrorGroup
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}
				if result.Stats.ByCategory[errors.CategoryLint] != 2 {
					t.Errorf("CategoryLint count = %d, want 2", result.Stats.ByCategory[errors.CategoryLint])
				}
				if result.Stats.ByCategory[errors.CategoryTest] != 1 {
					t.Errorf("CategoryTest count = %d, want 1", result.Stats.ByCategory[errors.CategoryTest])
				}
				if result.Stats.ByCategory[errors.CategoryCompile] != 2 {
					t.Errorf("CategoryCompile count = %d, want 2", result.Stats.ByCategory[errors.CategoryCompile])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := FormatJSONDetailed(&buf, tt.grouped)
			if err != nil {
				t.Fatalf("FormatJSONDetailed() error = %v", err)
			}

			output := buf.String()
			if output == "" {
				t.Fatal("FormatJSONDetailed() produced empty output")
			}

			tt.validate(t, output)
		})
	}
}

func TestFormatJSONDetailed_IndentedOutput(t *testing.T) {
	grouped := &errors.ComprehensiveErrorGroup{
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
			UniqueFiles:  0,
			UniqueRules:  0,
		},
	}

	var buf bytes.Buffer
	err := FormatJSONDetailed(&buf, grouped)
	if err != nil {
		t.Fatalf("FormatJSONDetailed() error = %v", err)
	}

	output := buf.String()
	// Verify JSON is valid and indented
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}
