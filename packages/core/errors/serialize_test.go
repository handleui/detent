package errors

import (
	"testing"
)

func TestComprehensiveErrorGroup_ForOrchestrator(t *testing.T) {
	tests := []struct {
		name           string
		group          *ComprehensiveErrorGroup
		expectedNil    bool
		expectedLen    int
		expectedStats  ErrorStats
		checkWorkflows map[int]string // index -> expected WorkflowJob
	}{
		{
			name:        "nil receiver returns nil",
			group:       nil,
			expectedNil: true,
		},
		{
			name: "empty group returns empty result",
			group: &ComprehensiveErrorGroup{
				ByFile: make(map[string][]*ExtractedError),
				NoFile: []*ExtractedError{},
				Stats:  ErrorStats{Total: 0},
			},
			expectedNil: false,
			expectedLen: 0,
			expectedStats: ErrorStats{
				Total: 0,
			},
		},
		{
			name: "errors without WorkflowContext",
			group: &ComprehensiveErrorGroup{
				ByFile: map[string][]*ExtractedError{
					"file1.go": {
						{
							File:     "file1.go",
							Line:     10,
							Message:  "undefined variable",
							Severity: "error",
							Category: CategoryTypeCheck,
							Source:   SourceTypeScript,
							RuleID:   "TS2304",
						},
					},
				},
				NoFile: []*ExtractedError{},
				Total:  1,
				Stats: ErrorStats{
					Total:      1,
					ErrorCount: 1,
				},
			},
			expectedNil:   false,
			expectedLen:   1,
			checkWorkflows: map[int]string{0: ""},
		},
		{
			name: "errors with WorkflowContext populated",
			group: &ComprehensiveErrorGroup{
				ByFile: map[string][]*ExtractedError{
					"src/app.ts": {
						{
							File:     "src/app.ts",
							Line:     25,
							Message:  "missing semicolon",
							Severity: "warning",
							Category: CategoryLint,
							Source:   SourceESLint,
							RuleID:   "semi",
							WorkflowContext: &WorkflowContext{
								Job: "lint-job",
							},
						},
						{
							File:     "src/app.ts",
							Line:     30,
							Message:  "type error",
							Severity: "error",
							Category: CategoryTypeCheck,
							Source:   SourceTypeScript,
							WorkflowContext: &WorkflowContext{
								Job: "typecheck-job",
							},
						},
					},
				},
				NoFile: []*ExtractedError{},
				Total:  2,
				Stats: ErrorStats{
					Total:        2,
					ErrorCount:   1,
					WarningCount: 1,
				},
			},
			expectedNil: false,
			expectedLen: 2,
			checkWorkflows: map[int]string{
				0: "lint-job",
				1: "typecheck-job",
			},
		},
		{
			name: "errors with empty Job in WorkflowContext",
			group: &ComprehensiveErrorGroup{
				ByFile: map[string][]*ExtractedError{
					"main.go": {
						{
							File:            "main.go",
							Line:            5,
							Message:         "unused variable",
							Severity:        "warning",
							Category:        CategoryLint,
							Source:          SourceGo,
							WorkflowContext: &WorkflowContext{Job: ""},
						},
					},
				},
				NoFile: []*ExtractedError{},
				Total:  1,
				Stats: ErrorStats{
					Total:        1,
					WarningCount: 1,
				},
			},
			expectedNil:    false,
			expectedLen:    1,
			checkWorkflows: map[int]string{0: ""},
		},
		{
			name: "errors in NoFile slice",
			group: &ComprehensiveErrorGroup{
				ByFile: make(map[string][]*ExtractedError),
				NoFile: []*ExtractedError{
					{
						Message:  "general error without file",
						Severity: "error",
						Category: CategoryRuntime,
						Source:   SourceGeneric,
					},
				},
				Total: 1,
				Stats: ErrorStats{
					Total:      1,
					ErrorCount: 1,
				},
			},
			expectedNil: false,
			expectedLen: 1,
		},
		{
			name: "mixed ByFile and NoFile errors",
			group: &ComprehensiveErrorGroup{
				ByFile: map[string][]*ExtractedError{
					"test.go": {
						{
							File:     "test.go",
							Line:     1,
							Message:  "test failed",
							Severity: "error",
							Category: CategoryTest,
							Source:   SourceGoTest,
						},
					},
				},
				NoFile: []*ExtractedError{
					{
						Message:  "build failed",
						Severity: "error",
						Category: CategoryCompile,
						Source:   SourceGo,
					},
				},
				Total: 2,
				Stats: ErrorStats{
					Total:      2,
					ErrorCount: 2,
				},
			},
			expectedNil: false,
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.group.ForOrchestrator()

			if tt.expectedNil {
				if result != nil {
					t.Errorf("ForOrchestrator() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("ForOrchestrator() = nil, want non-nil")
			}

			if len(result.Errors) != tt.expectedLen {
				t.Errorf("ForOrchestrator() returned %d errors, want %d", len(result.Errors), tt.expectedLen)
			}

			for idx, expectedJob := range tt.checkWorkflows {
				if idx >= len(result.Errors) {
					t.Errorf("checkWorkflows index %d out of bounds (len=%d)", idx, len(result.Errors))
					continue
				}
				if result.Errors[idx].WorkflowJob != expectedJob {
					t.Errorf("Errors[%d].WorkflowJob = %q, want %q", idx, result.Errors[idx].WorkflowJob, expectedJob)
				}
			}
		})
	}
}

func TestComprehensiveErrorGroup_ForOrchestrator_FieldMapping(t *testing.T) {
	group := &ComprehensiveErrorGroup{
		ByFile: map[string][]*ExtractedError{
			"src/component.tsx": {
				{
					File:        "src/component.tsx",
					Line:        42,
					Column:      10,
					Message:     "Property 'foo' does not exist",
					Severity:    "error",
					Category:    CategoryTypeCheck,
					Source:      SourceTypeScript,
					RuleID:      "TS2339",
					Raw:         "raw output line",
					StackTrace:  "at Component (component.tsx:42)",
					Suggestions: []string{"Did you mean 'bar'?"},
					CodeSnippet: &CodeSnippet{
						Lines:     []string{"const x = obj.foo;"},
						StartLine: 42,
						ErrorLine: 1,
						Language:  "typescript",
					},
					WorkflowContext: &WorkflowContext{
						Job:    "build",
						Step:   "compile",
						Action: "tsc",
					},
				},
			},
		},
		NoFile: []*ExtractedError{},
		Total:  1,
		Stats: ErrorStats{
			Total:      1,
			ErrorCount: 1,
			ByCategory: map[ErrorCategory]int{CategoryTypeCheck: 1},
			BySource:   map[string]int{SourceTypeScript: 1},
		},
	}

	result := group.ForOrchestrator()

	if result == nil {
		t.Fatalf("ForOrchestrator() = nil, want non-nil")
	}

	if len(result.Errors) != 1 {
		t.Fatalf("ForOrchestrator() returned %d errors, want 1", len(result.Errors))
	}

	orchErr := result.Errors[0]

	if orchErr.File != "src/component.tsx" {
		t.Errorf("File = %q, want %q", orchErr.File, "src/component.tsx")
	}
	if orchErr.Line != 42 {
		t.Errorf("Line = %d, want %d", orchErr.Line, 42)
	}
	if orchErr.Message != "Property 'foo' does not exist" {
		t.Errorf("Message = %q, want %q", orchErr.Message, "Property 'foo' does not exist")
	}
	if orchErr.Severity != "error" {
		t.Errorf("Severity = %q, want %q", orchErr.Severity, "error")
	}
	if orchErr.Category != CategoryTypeCheck {
		t.Errorf("Category = %q, want %q", orchErr.Category, CategoryTypeCheck)
	}
	if orchErr.Source != SourceTypeScript {
		t.Errorf("Source = %q, want %q", orchErr.Source, SourceTypeScript)
	}
	if orchErr.RuleID != "TS2339" {
		t.Errorf("RuleID = %q, want %q", orchErr.RuleID, "TS2339")
	}
	if orchErr.WorkflowJob != "build" {
		t.Errorf("WorkflowJob = %q, want %q", orchErr.WorkflowJob, "build")
	}
}

func TestComprehensiveErrorGroup_ForAgent(t *testing.T) {
	baseGroup := &ComprehensiveErrorGroup{
		ByFile: map[string][]*ExtractedError{
			"src/utils.ts": {
				{
					File:     "src/utils.ts",
					Line:     10,
					Message:  "no-var rule",
					Severity: "error",
					Category: CategoryLint,
					Source:   SourceESLint,
					RuleID:   "no-var",
				},
			},
			"src/app.ts": {
				{
					File:     "src/app.ts",
					Line:     20,
					Message:  "type error",
					Severity: "error",
					Category: CategoryTypeCheck,
					Source:   SourceTypeScript,
					RuleID:   "TS2304",
				},
				{
					File:     "src/app.ts",
					Line:     25,
					Message:  "unused import",
					Severity: "warning",
					Category: CategoryLint,
					Source:   SourceESLint,
					RuleID:   "no-unused-vars",
				},
			},
			"tests/app.test.ts": {
				{
					File:     "tests/app.test.ts",
					Line:     5,
					Message:  "test failure",
					Severity: "error",
					Category: CategoryTest,
					Source:   SourceGoTest,
				},
			},
		},
		NoFile: []*ExtractedError{
			{
				Message:  "build failed",
				Severity: "error",
				Category: CategoryCompile,
				Source:   SourceGo,
			},
		},
		Total: 5,
	}

	tests := []struct {
		name        string
		group       *ComprehensiveErrorGroup
		filter      func(*ExtractedError) bool
		expectedNil bool
		expectedLen int
	}{
		{
			name:        "nil receiver returns nil",
			group:       nil,
			filter:      FilterByCategory(CategoryLint),
			expectedNil: true,
		},
		{
			name:        "nil filter returns empty slice",
			group:       baseGroup,
			filter:      nil,
			expectedNil: false,
			expectedLen: 0,
		},
		{
			name:        "filter by lint category",
			group:       baseGroup,
			filter:      FilterByCategory(CategoryLint),
			expectedNil: false,
			expectedLen: 2,
		},
		{
			name:        "filter by type-check category",
			group:       baseGroup,
			filter:      FilterByCategory(CategoryTypeCheck),
			expectedNil: false,
			expectedLen: 1,
		},
		{
			name:        "filter by non-existent category",
			group:       baseGroup,
			filter:      FilterByCategory(CategorySecurity),
			expectedNil: false,
			expectedLen: 0,
		},
		{
			name:        "filter by severity error",
			group:       baseGroup,
			filter:      FilterBySeverity("error"),
			expectedNil: false,
			expectedLen: 4,
		},
		{
			name:        "filter by severity warning",
			group:       baseGroup,
			filter:      FilterBySeverity("warning"),
			expectedNil: false,
			expectedLen: 1,
		},
		{
			name:        "filter by file prefix src/",
			group:       baseGroup,
			filter:      FilterByFile("src/"),
			expectedNil: false,
			expectedLen: 3,
		},
		{
			name:        "filter by file prefix tests/",
			group:       baseGroup,
			filter:      FilterByFile("tests/"),
			expectedNil: false,
			expectedLen: 1,
		},
		{
			name:        "filter by exact file match",
			group:       baseGroup,
			filter:      FilterByFile("src/app.ts"),
			expectedNil: false,
			expectedLen: 2,
		},
		{
			name:        "filter by non-matching prefix",
			group:       baseGroup,
			filter:      FilterByFile("lib/"),
			expectedNil: false,
			expectedLen: 0,
		},
		{
			name: "filter all matches",
			group: baseGroup,
			filter: func(_ *ExtractedError) bool {
				return true
			},
			expectedNil: false,
			expectedLen: 5,
		},
		{
			name: "filter none matches",
			group: baseGroup,
			filter: func(_ *ExtractedError) bool {
				return false
			},
			expectedNil: false,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.group.ForAgent(tt.filter)

			if tt.expectedNil {
				if result != nil {
					t.Errorf("ForAgent() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatalf("ForAgent() = nil, want non-nil")
			}

			if len(result) != tt.expectedLen {
				t.Errorf("ForAgent() returned %d errors, want %d", len(result), tt.expectedLen)
			}
		})
	}
}

func TestComprehensiveErrorGroup_ForAgent_ComposedFilters(t *testing.T) {
	group := &ComprehensiveErrorGroup{
		ByFile: map[string][]*ExtractedError{
			"src/utils.ts": {
				{
					File:     "src/utils.ts",
					Line:     10,
					Message:  "lint error",
					Severity: "error",
					Category: CategoryLint,
					Source:   SourceESLint,
				},
				{
					File:     "src/utils.ts",
					Line:     15,
					Message:  "lint warning",
					Severity: "warning",
					Category: CategoryLint,
					Source:   SourceESLint,
				},
			},
			"src/app.ts": {
				{
					File:     "src/app.ts",
					Line:     20,
					Message:  "type error",
					Severity: "error",
					Category: CategoryTypeCheck,
					Source:   SourceTypeScript,
				},
			},
			"tests/app.test.ts": {
				{
					File:     "tests/app.test.ts",
					Line:     5,
					Message:  "lint error in test",
					Severity: "error",
					Category: CategoryLint,
					Source:   SourceESLint,
				},
			},
		},
		NoFile: []*ExtractedError{},
		Total:  4,
	}

	tests := []struct {
		name        string
		filter      func(*ExtractedError) bool
		expectedLen int
	}{
		{
			name: "lint errors only",
			filter: func(err *ExtractedError) bool {
				return FilterByCategory(CategoryLint)(err) && FilterBySeverity("error")(err)
			},
			expectedLen: 2,
		},
		{
			name: "src/ lint errors",
			filter: func(err *ExtractedError) bool {
				return FilterByFile("src/")(err) && FilterByCategory(CategoryLint)(err)
			},
			expectedLen: 2,
		},
		{
			name: "src/ errors (any category)",
			filter: func(err *ExtractedError) bool {
				return FilterByFile("src/")(err) && FilterBySeverity("error")(err)
			},
			expectedLen: 2,
		},
		{
			name: "all three combined - src/ lint errors",
			filter: func(err *ExtractedError) bool {
				return FilterByFile("src/")(err) &&
					FilterByCategory(CategoryLint)(err) &&
					FilterBySeverity("error")(err)
			},
			expectedLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := group.ForAgent(tt.filter)

			if len(result) != tt.expectedLen {
				t.Errorf("ForAgent() returned %d errors, want %d", len(result), tt.expectedLen)
			}
		})
	}
}

func TestFilterByCategory(t *testing.T) {
	tests := []struct {
		name     string
		category ErrorCategory
		error    *ExtractedError
		expected bool
	}{
		{
			name:     "matching lint category",
			category: CategoryLint,
			error:    &ExtractedError{Category: CategoryLint},
			expected: true,
		},
		{
			name:     "matching type-check category",
			category: CategoryTypeCheck,
			error:    &ExtractedError{Category: CategoryTypeCheck},
			expected: true,
		},
		{
			name:     "non-matching category",
			category: CategoryLint,
			error:    &ExtractedError{Category: CategoryTypeCheck},
			expected: false,
		},
		{
			name:     "empty category in error",
			category: CategoryLint,
			error:    &ExtractedError{Category: ""},
			expected: false,
		},
		{
			name:     "empty category filter",
			category: "",
			error:    &ExtractedError{Category: ""},
			expected: true,
		},
		{
			name:     "unknown category",
			category: CategoryUnknown,
			error:    &ExtractedError{Category: CategoryUnknown},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := FilterByCategory(tt.category)
			result := filter(tt.error)

			if result != tt.expected {
				t.Errorf("FilterByCategory(%q)(%+v) = %v, want %v",
					tt.category, tt.error.Category, result, tt.expected)
			}
		})
	}
}

func TestFilterByFile(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		error    *ExtractedError
		expected bool
	}{
		{
			name:     "matching prefix",
			prefix:   "src/",
			error:    &ExtractedError{File: "src/app.ts"},
			expected: true,
		},
		{
			name:     "exact match",
			prefix:   "src/app.ts",
			error:    &ExtractedError{File: "src/app.ts"},
			expected: true,
		},
		{
			name:     "non-matching prefix",
			prefix:   "lib/",
			error:    &ExtractedError{File: "src/app.ts"},
			expected: false,
		},
		{
			name:     "empty prefix matches all",
			prefix:   "",
			error:    &ExtractedError{File: "src/app.ts"},
			expected: true,
		},
		{
			name:     "empty file matches empty prefix",
			prefix:   "",
			error:    &ExtractedError{File: ""},
			expected: true,
		},
		{
			name:     "empty file does not match non-empty prefix",
			prefix:   "src/",
			error:    &ExtractedError{File: ""},
			expected: false,
		},
		{
			name:     "partial match within directory name",
			prefix:   "src",
			error:    &ExtractedError{File: "src-old/app.ts"},
			expected: true,
		},
		{
			name:     "nested directory match",
			prefix:   "src/components/",
			error:    &ExtractedError{File: "src/components/Button.tsx"},
			expected: true,
		},
		{
			name:     "case sensitive - no match",
			prefix:   "SRC/",
			error:    &ExtractedError{File: "src/app.ts"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := FilterByFile(tt.prefix)
			result := filter(tt.error)

			if result != tt.expected {
				t.Errorf("FilterByFile(%q)(%q) = %v, want %v",
					tt.prefix, tt.error.File, result, tt.expected)
			}
		})
	}
}

func TestFilterBySeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity string
		error    *ExtractedError
		expected bool
	}{
		{
			name:     "matching error severity",
			severity: "error",
			error:    &ExtractedError{Severity: "error"},
			expected: true,
		},
		{
			name:     "matching warning severity",
			severity: "warning",
			error:    &ExtractedError{Severity: "warning"},
			expected: true,
		},
		{
			name:     "non-matching severity",
			severity: "error",
			error:    &ExtractedError{Severity: "warning"},
			expected: false,
		},
		{
			name:     "empty severity in error",
			severity: "error",
			error:    &ExtractedError{Severity: ""},
			expected: false,
		},
		{
			name:     "empty severity filter",
			severity: "",
			error:    &ExtractedError{Severity: ""},
			expected: true,
		},
		{
			name:     "invalid severity filter",
			severity: "fatal",
			error:    &ExtractedError{Severity: "error"},
			expected: false,
		},
		{
			name:     "case sensitive - no match",
			severity: "Error",
			error:    &ExtractedError{Severity: "error"},
			expected: false,
		},
		{
			name:     "info severity (if supported)",
			severity: "info",
			error:    &ExtractedError{Severity: "info"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := FilterBySeverity(tt.severity)
			result := filter(tt.error)

			if result != tt.expected {
				t.Errorf("FilterBySeverity(%q)(%q) = %v, want %v",
					tt.severity, tt.error.Severity, result, tt.expected)
			}
		})
	}
}

func TestComprehensiveErrorGroup_flatten(t *testing.T) {
	tests := []struct {
		name        string
		group       *ComprehensiveErrorGroup
		expectedLen int
	}{
		{
			name: "empty group",
			group: &ComprehensiveErrorGroup{
				ByFile: make(map[string][]*ExtractedError),
				NoFile: []*ExtractedError{},
				Total:  0,
			},
			expectedLen: 0,
		},
		{
			name: "only ByFile errors",
			group: &ComprehensiveErrorGroup{
				ByFile: map[string][]*ExtractedError{
					"file1.go": {
						{Message: "error 1"},
						{Message: "error 2"},
					},
					"file2.go": {
						{Message: "error 3"},
					},
				},
				NoFile: []*ExtractedError{},
				Total:  3,
			},
			expectedLen: 3,
		},
		{
			name: "only NoFile errors",
			group: &ComprehensiveErrorGroup{
				ByFile: make(map[string][]*ExtractedError),
				NoFile: []*ExtractedError{
					{Message: "error 1"},
					{Message: "error 2"},
				},
				Total: 2,
			},
			expectedLen: 2,
		},
		{
			name: "mixed ByFile and NoFile",
			group: &ComprehensiveErrorGroup{
				ByFile: map[string][]*ExtractedError{
					"file1.go": {
						{Message: "error 1"},
					},
				},
				NoFile: []*ExtractedError{
					{Message: "error 2"},
				},
				Total: 2,
			},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.group.flatten()

			if len(result) != tt.expectedLen {
				t.Errorf("flatten() returned %d errors, want %d", len(result), tt.expectedLen)
			}
		})
	}
}

func TestForOrchestrator_StatsPreserved(t *testing.T) {
	expectedStats := ErrorStats{
		Total:        5,
		ErrorCount:   3,
		WarningCount: 2,
		ByCategory: map[ErrorCategory]int{
			CategoryLint:      2,
			CategoryTypeCheck: 3,
		},
		BySource: map[string]int{
			SourceESLint:     2,
			SourceTypeScript: 3,
		},
		UniqueFiles: 2,
		UniqueRules: 4,
	}

	group := &ComprehensiveErrorGroup{
		ByFile: map[string][]*ExtractedError{
			"file.ts": {{Message: "test"}},
		},
		NoFile: []*ExtractedError{},
		Total:  1,
		Stats:  expectedStats,
	}

	result := group.ForOrchestrator()

	if result == nil {
		t.Fatalf("ForOrchestrator() = nil, want non-nil")
	}

	if result.Stats.Total != expectedStats.Total {
		t.Errorf("Stats.Total = %d, want %d", result.Stats.Total, expectedStats.Total)
	}
	if result.Stats.ErrorCount != expectedStats.ErrorCount {
		t.Errorf("Stats.ErrorCount = %d, want %d", result.Stats.ErrorCount, expectedStats.ErrorCount)
	}
	if result.Stats.WarningCount != expectedStats.WarningCount {
		t.Errorf("Stats.WarningCount = %d, want %d", result.Stats.WarningCount, expectedStats.WarningCount)
	}
	if result.Stats.UniqueFiles != expectedStats.UniqueFiles {
		t.Errorf("Stats.UniqueFiles = %d, want %d", result.Stats.UniqueFiles, expectedStats.UniqueFiles)
	}
	if result.Stats.UniqueRules != expectedStats.UniqueRules {
		t.Errorf("Stats.UniqueRules = %d, want %d", result.Stats.UniqueRules, expectedStats.UniqueRules)
	}
}

func TestForAgent_ReturnsFullErrorDetails(t *testing.T) {
	codeSnippet := &CodeSnippet{
		Lines:     []string{"const x = 1;", "const y = 2;"},
		StartLine: 10,
		ErrorLine: 1,
		Language:  "typescript",
	}
	workflowContext := &WorkflowContext{
		Job:    "build",
		Step:   "compile",
		Action: "tsc",
	}

	group := &ComprehensiveErrorGroup{
		ByFile: map[string][]*ExtractedError{
			"src/app.ts": {
				{
					File:            "src/app.ts",
					Line:            11,
					Column:          5,
					Message:         "type error",
					Severity:        "error",
					Category:        CategoryTypeCheck,
					Source:          SourceTypeScript,
					RuleID:          "TS2304",
					Raw:             "raw line",
					StackTrace:      "stack trace here",
					Suggestions:     []string{"suggestion 1", "suggestion 2"},
					CodeSnippet:     codeSnippet,
					WorkflowContext: workflowContext,
					LineKnown:       true,
					ColumnKnown:     true,
				},
			},
		},
		NoFile: []*ExtractedError{},
		Total:  1,
	}

	result := group.ForAgent(func(_ *ExtractedError) bool { return true })

	if len(result) != 1 {
		t.Fatalf("ForAgent() returned %d errors, want 1", len(result))
	}

	err := result[0]

	if err.CodeSnippet == nil {
		t.Error("CodeSnippet should be preserved")
	} else if err.CodeSnippet.Language != "typescript" {
		t.Errorf("CodeSnippet.Language = %q, want %q", err.CodeSnippet.Language, "typescript")
	}

	if err.WorkflowContext == nil {
		t.Error("WorkflowContext should be preserved")
	} else if err.WorkflowContext.Job != "build" {
		t.Errorf("WorkflowContext.Job = %q, want %q", err.WorkflowContext.Job, "build")
	}

	if err.StackTrace != "stack trace here" {
		t.Errorf("StackTrace = %q, want %q", err.StackTrace, "stack trace here")
	}

	if len(err.Suggestions) != 2 {
		t.Errorf("Suggestions length = %d, want 2", len(err.Suggestions))
	}

	if err.Raw != "raw line" {
		t.Errorf("Raw = %q, want %q", err.Raw, "raw line")
	}

	if !err.LineKnown {
		t.Error("LineKnown should be true")
	}

	if !err.ColumnKnown {
		t.Error("ColumnKnown should be true")
	}
}
