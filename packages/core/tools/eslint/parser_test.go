package eslint

import (
	"testing"

	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/tools/parser"
)

func TestParser_ID(t *testing.T) {
	p := NewParser()
	if got := p.ID(); got != "eslint" {
		t.Errorf("ID() = %q, want %q", got, "eslint")
	}
}

func TestParser_Priority(t *testing.T) {
	p := NewParser()
	if got := p.Priority(); got != 85 {
		t.Errorf("Priority() = %d, want %d", got, 85)
	}
}

func TestParser_CanParse(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		ctx      *parser.ParseContext
		minScore float64
		maxScore float64
	}{
		{
			name:     "stylish file path .js",
			line:     "/path/to/file.js",
			minScore: 0.8,
			maxScore: 0.9,
		},
		{
			name:     "stylish file path .tsx",
			line:     "src/components/Button.tsx",
			minScore: 0.8,
			maxScore: 0.9,
		},
		{
			name:     "stylish file path .astro",
			line:     "src/pages/index.astro",
			minScore: 0.8,
			maxScore: 0.9,
		},
		{
			name:     "stylish error line with context",
			line:     "  8:11  error  Value other than \"bar\" assigned  no-var",
			ctx:      &parser.ParseContext{LastFile: "/path/to/file.js"},
			minScore: 0.85,
			maxScore: 1.0,
		},
		{
			name:     "stylish warning line",
			line:     "  10:5  warning  Unexpected console statement  no-console",
			ctx:      &parser.ParseContext{LastFile: "/path/to/file.js"},
			minScore: 0.85,
			maxScore: 1.0,
		},
		{
			name:     "compact format",
			line:     "/path/to/file.js: line 8, col 11, Error - Message (rule-id)",
			minScore: 0.85,
			maxScore: 1.0,
		},
		{
			name:     "unix format",
			line:     "/path/to/file.js:8:11: Unexpected var [error/no-var]",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "unix format warning",
			line:     "src/app.tsx:15:3: Console statement found [warning/no-console]",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "Go error (should not match)",
			line:     "main.go:25:10: undefined: foo",
			minScore: 0,
			maxScore: 0.1,
		},
		{
			name:     "TypeScript error (should not match)",
			line:     "src/app.ts(10,5): error TS2322: Type error",
			minScore: 0,
			maxScore: 0.1,
		},
		{
			name:     "JS file with Go-style error but no bracket suffix (should not match unix)",
			line:     "src/app.js:10:5: some error message",
			minScore: 0,
			maxScore: 0.1,
		},
		{
			name:     "empty line",
			line:     "",
			minScore: 0,
			maxScore: 0.1,
		},
		{
			name:     "plain text",
			line:     "Building project...",
			minScore: 0,
			maxScore: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.Reset() // Reset between tests
			score := p.CanParse(tt.line, tt.ctx)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("CanParse(%q) = %v, want between %.2f and %.2f",
					tt.line, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestParser_Parse_StylishFormat(t *testing.T) {
	p := NewParser()

	// First, set the file path
	ctx := &parser.ParseContext{}
	err := p.Parse("/path/to/file.js", ctx)
	if err != nil {
		t.Error("expected nil for file path line")
	}

	// Verify file was captured
	if p.currentFile != "/path/to/file.js" {
		t.Errorf("currentFile = %q, want %q", p.currentFile, "/path/to/file.js")
	}

	tests := []struct {
		name     string
		line     string
		wantFile string
		wantLine int
		wantCol  int
		wantMsg  string
		wantRule string
		wantSev  string
	}{
		{
			name:     "error with simple rule",
			line:     "  8:11  error  Value other than \"bar\" assigned  no-var",
			wantFile: "/path/to/file.js",
			wantLine: 8,
			wantCol:  11,
			wantMsg:  "Value other than \"bar\" assigned",
			wantRule: "no-var",
			wantSev:  "error",
		},
		{
			name:     "warning with scoped rule",
			line:     "  10:5  warning  Unexpected console statement  react/no-unsafe",
			wantFile: "/path/to/file.js",
			wantLine: 10,
			wantCol:  5,
			wantMsg:  "Unexpected console statement",
			wantRule: "react/no-unsafe",
			wantSev:  "warning",
		},
		{
			name:     "error with namespaced rule",
			line:     "  15:3  error  Unused variable 'foo'  @typescript-eslint/no-unused-vars",
			wantFile: "/path/to/file.js",
			wantLine: 15,
			wantCol:  3,
			wantMsg:  "Unused variable 'foo'",
			wantRule: "@typescript-eslint/no-unused-vars",
			wantSev:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Parse(tt.line, ctx)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.File != tt.wantFile {
				t.Errorf("File = %q, want %q", err.File, tt.wantFile)
			}
			if err.Line != tt.wantLine {
				t.Errorf("Line = %d, want %d", err.Line, tt.wantLine)
			}
			if err.Column != tt.wantCol {
				t.Errorf("Column = %d, want %d", err.Column, tt.wantCol)
			}
			if err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", err.Message, tt.wantMsg)
			}
			if err.RuleID != tt.wantRule {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRule)
			}
			if err.Severity != tt.wantSev {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSev)
			}
			if err.Source != errors.SourceESLint {
				t.Errorf("Source = %q, want %q", err.Source, errors.SourceESLint)
			}
			if err.Category != errors.CategoryLint {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryLint)
			}
		})
	}
}

func TestParser_Parse_CompactFormat(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		wantFile string
		wantLine int
		wantCol  int
		wantMsg  string
		wantRule string
		wantSev  string
	}{
		{
			name:     "compact with rule",
			line:     "/path/to/file.js: line 8, col 11, Error - Unexpected var (no-var)",
			wantFile: "/path/to/file.js",
			wantLine: 8,
			wantCol:  11,
			wantMsg:  "Unexpected var",
			wantRule: "no-var",
			wantSev:  "error",
		},
		{
			name:     "compact warning",
			line:     "src/app.tsx: line 10, col 5, Warning - Console statement (no-console)",
			wantFile: "src/app.tsx",
			wantLine: 10,
			wantCol:  5,
			wantMsg:  "Console statement",
			wantRule: "no-console",
			wantSev:  "warning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.Reset()
			err := p.Parse(tt.line, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.File != tt.wantFile {
				t.Errorf("File = %q, want %q", err.File, tt.wantFile)
			}
			if err.Line != tt.wantLine {
				t.Errorf("Line = %d, want %d", err.Line, tt.wantLine)
			}
			if err.Column != tt.wantCol {
				t.Errorf("Column = %d, want %d", err.Column, tt.wantCol)
			}
			if err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", err.Message, tt.wantMsg)
			}
			if err.RuleID != tt.wantRule {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRule)
			}
			if err.Severity != tt.wantSev {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSev)
			}
		})
	}
}

func TestParser_Parse_UnixFormat(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		wantFile string
		wantLine int
		wantCol  int
		wantMsg  string
		wantRule string
		wantSev  string
	}{
		{
			name:     "unix error with simple rule",
			line:     "/path/to/file.js:8:11: Unexpected var [error/no-var]",
			wantFile: "/path/to/file.js",
			wantLine: 8,
			wantCol:  11,
			wantMsg:  "Unexpected var",
			wantRule: "no-var",
			wantSev:  "error",
		},
		{
			name:     "unix warning with scoped rule",
			line:     "src/app.tsx:15:3: Missing key prop [warning/react/jsx-key]",
			wantFile: "src/app.tsx",
			wantLine: 15,
			wantCol:  3,
			wantMsg:  "Missing key prop",
			wantRule: "react/jsx-key",
			wantSev:  "warning",
		},
		{
			name:     "unix error with namespaced rule",
			line:     "src/utils.ts:22:1: Unused variable 'foo' [error/@typescript-eslint/no-unused-vars]",
			wantFile: "src/utils.ts",
			wantLine: 22,
			wantCol:  1,
			wantMsg:  "Unused variable 'foo'",
			wantRule: "@typescript-eslint/no-unused-vars",
			wantSev:  "error",
		},
		{
			name:     "unix astro file",
			line:     "src/pages/index.astro:10:5: Some lint error [error/some-rule]",
			wantFile: "src/pages/index.astro",
			wantLine: 10,
			wantCol:  5,
			wantMsg:  "Some lint error",
			wantRule: "some-rule",
			wantSev:  "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.Reset()
			err := p.Parse(tt.line, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.File != tt.wantFile {
				t.Errorf("File = %q, want %q", err.File, tt.wantFile)
			}
			if err.Line != tt.wantLine {
				t.Errorf("Line = %d, want %d", err.Line, tt.wantLine)
			}
			if err.Column != tt.wantCol {
				t.Errorf("Column = %d, want %d", err.Column, tt.wantCol)
			}
			if err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", err.Message, tt.wantMsg)
			}
			if err.RuleID != tt.wantRule {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRule)
			}
			if err.Severity != tt.wantSev {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSev)
			}
			if err.Source != errors.SourceESLint {
				t.Errorf("Source = %q, want %q", err.Source, errors.SourceESLint)
			}
			if err.Category != errors.CategoryLint {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryLint)
			}
		})
	}
}

func TestParser_IsNoise(t *testing.T) {
	p := NewParser()

	noiseLines := []string{
		"✖ 2 problems (1 error, 1 warning)",
		"X 5 problems (5 errors, 0 warnings)",
		"",
		"   ",
		"1 error",
		"3 warnings",
		"1 error and 0 warnings potentially fixable with the `--fix` option.",
		"✓ All files pass linting",
		"All files pass linting",
	}

	for _, line := range noiseLines {
		if !p.IsNoise(line) {
			t.Errorf("expected %q to be noise", line)
		}
	}

	nonNoiseLines := []string{
		"  8:11  error  Message  rule-id",
		"/path/to/file.js",
		"/path/to/file.js: line 8, col 11, Error - Message (rule)",
	}

	for _, line := range nonNoiseLines {
		if p.IsNoise(line) {
			t.Errorf("expected %q to NOT be noise", line)
		}
	}
}

func TestParser_SupportsMultiLine(t *testing.T) {
	p := NewParser()
	if !p.SupportsMultiLine() {
		t.Error("expected SupportsMultiLine to return true")
	}
}

func TestParser_MultiLineStylishFlow(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Simulate stylish output flow
	// 1. File path
	err := p.Parse("/path/to/file.js", ctx)
	if err != nil {
		t.Error("file path should return nil")
	}
	if !p.inStylish {
		t.Error("expected inStylish to be true after file path")
	}

	// 2. First error
	err = p.Parse("  8:11  error  First error  rule-a", ctx)
	if err == nil {
		t.Error("expected error for first line")
	}

	// 3. Continue with second error
	continued := p.ContinueMultiLine("  10:5  warning  Second warning  rule-b", ctx)
	if !continued {
		t.Error("expected ContinueMultiLine to return true for error line")
	}

	// 4. Empty line ends sequence
	continued = p.ContinueMultiLine("", ctx)
	if continued {
		t.Error("expected ContinueMultiLine to return false for empty line")
	}
}

func TestParser_Reset(t *testing.T) {
	p := NewParser()

	// Set up state
	p.currentFile = "/path/to/file.js"
	p.inStylish = true

	p.Reset()

	if p.currentFile != "" {
		t.Error("currentFile should be empty after Reset")
	}
	if p.inStylish {
		t.Error("inStylish should be false after Reset")
	}
}

func TestParser_WorkflowContext(t *testing.T) {
	p := NewParser()

	ctx := &parser.ParseContext{
		Job:  "lint-job",
		Step: "Run ESLint",
		WorkflowContext: &errors.WorkflowContext{
			Job:  "lint-job",
			Step: "Run ESLint",
		},
	}

	// Set file first
	p.Parse("src/app.js", ctx)

	// Parse error
	err := p.Parse("  10:5  error  Some error  no-var", ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.WorkflowContext == nil {
		t.Fatal("expected WorkflowContext to be set")
	}

	if err.WorkflowContext.Job != "lint-job" {
		t.Errorf("WorkflowContext.Job = %q, want %q", err.WorkflowContext.Job, "lint-job")
	}
}

func TestParser_ExtractRuleID(t *testing.T) {
	tests := []struct {
		input    string
		wantMsg  string
		wantRule string
	}{
		{
			input:    "Unexpected var  no-var",
			wantMsg:  "Unexpected var",
			wantRule: "no-var",
		},
		{
			input:    "Use const instead  react/jsx-key",
			wantMsg:  "Use const instead",
			wantRule: "react/jsx-key",
		},
		{
			input:    "Unused variable 'x'  @typescript-eslint/no-unused-vars",
			wantMsg:  "Unused variable 'x'",
			wantRule: "@typescript-eslint/no-unused-vars",
		},
		{
			input:    "Message without rule ending in period.",
			wantMsg:  "Message without rule ending in period.",
			wantRule: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			msg, rule := extractRuleID(tt.input)
			if msg != tt.wantMsg {
				t.Errorf("message = %q, want %q", msg, tt.wantMsg)
			}
			if rule != tt.wantRule {
				t.Errorf("rule = %q, want %q", rule, tt.wantRule)
			}
		})
	}
}

func TestParser_ANSIStripping(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Set file with ANSI codes
	p.Parse("\x1b[36m/path/to/file.js\x1b[0m", ctx)

	// Verify file was captured without ANSI codes
	if p.currentFile != "/path/to/file.js" {
		t.Errorf("currentFile = %q, want %q", p.currentFile, "/path/to/file.js")
	}

	// Parse error with ANSI codes
	line := "\x1b[31m  8:11  error  Message  no-var\x1b[0m"
	err := p.Parse(line, ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Line != 8 {
		t.Errorf("Line = %d, want 8", err.Line)
	}
	if err.RuleID != "no-var" {
		t.Errorf("RuleID = %q, want %q", err.RuleID, "no-var")
	}
}

func TestParser_RealWorldSamples(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Simulate real ESLint stylish output
	lines := []string{
		"/Users/dev/project/src/components/Button.tsx",
		"  12:5   error    'useState' is defined but never used  @typescript-eslint/no-unused-vars",
		"  15:10  warning  Unexpected console statement  no-console",
		"  22:3   error    React Hook \"useEffect\" is called conditionally  react-hooks/rules-of-hooks",
		"",
		"/Users/dev/project/src/utils/helpers.ts",
		"  8:1  error  Prefer const  prefer-const",
	}

	expectedErrors := []struct {
		file     string
		line     int
		ruleID   string
		severity string
	}{
		{"/Users/dev/project/src/components/Button.tsx", 12, "@typescript-eslint/no-unused-vars", "error"},
		{"/Users/dev/project/src/components/Button.tsx", 15, "no-console", "warning"},
		{"/Users/dev/project/src/components/Button.tsx", 22, "react-hooks/rules-of-hooks", "error"},
		{"/Users/dev/project/src/utils/helpers.ts", 8, "prefer-const", "error"},
	}

	errorIdx := 0
	for _, line := range lines {
		err := p.Parse(line, ctx)
		if err != nil {
			if errorIdx >= len(expectedErrors) {
				t.Fatalf("unexpected error at index %d", errorIdx)
			}
			expected := expectedErrors[errorIdx]
			if err.File != expected.file {
				t.Errorf("error %d: File = %q, want %q", errorIdx, err.File, expected.file)
			}
			if err.Line != expected.line {
				t.Errorf("error %d: Line = %d, want %d", errorIdx, err.Line, expected.line)
			}
			if err.RuleID != expected.ruleID {
				t.Errorf("error %d: RuleID = %q, want %q", errorIdx, err.RuleID, expected.ruleID)
			}
			if err.Severity != expected.severity {
				t.Errorf("error %d: Severity = %q, want %q", errorIdx, err.Severity, expected.severity)
			}
			errorIdx++
		}
	}

	if errorIdx != len(expectedErrors) {
		t.Errorf("found %d errors, want %d", errorIdx, len(expectedErrors))
	}
}

func TestParser_FileExtensions(t *testing.T) {
	p := NewParser()

	validFiles := []string{
		"file.js",
		"file.jsx",
		"file.ts",
		"file.tsx",
		"file.mjs",
		"file.cjs",
		"file.mts",
		"file.cts",
		"file.vue",
		"file.svelte",
		"file.astro",
		"/path/to/deep/file.tsx",
		"src/pages/index.astro",
	}

	for _, file := range validFiles {
		p.Reset()
		if score := p.CanParse(file, nil); score < 0.8 {
			t.Errorf("expected high score for %q, got %v", file, score)
		}
	}

	invalidFiles := []string{
		"file.go",
		"file.py",
		"file.rs",
		"file.java",
		"file.txt",
		"file.json",
	}

	for _, file := range invalidFiles {
		p.Reset()
		if score := p.CanParse(file, nil); score > 0.1 {
			t.Errorf("expected low score for %q, got %v", file, score)
		}
	}
}

func TestParser_PatternClashPrevention(t *testing.T) {
	p := NewParser()

	// These should NOT be matched by ESLint parser to avoid clashing with Go/TS parsers
	clashTests := []struct {
		name string
		line string
		desc string
	}{
		{
			name: "Go error format",
			line: "main.go:25:10: undefined: foo",
			desc: "Go compiler error should not match ESLint",
		},
		{
			name: "TypeScript error format",
			line: "src/app.ts(10,5): error TS2322: Type mismatch",
			desc: "TypeScript error with parentheses should not match ESLint",
		},
		{
			name: "JS file with Go-style colon format (no bracket suffix)",
			line: "src/app.js:10:5: some error without bracket suffix",
			desc: "JS file with colon format but no [severity/rule] should not match unix format",
		},
		{
			name: "File path with colons (timestamp)",
			line: "/path/to/file.js:10:30:45",
			desc: "File path with multiple colons should not match",
		},
	}

	for _, tt := range clashTests {
		t.Run(tt.name, func(t *testing.T) {
			p.Reset()
			score := p.CanParse(tt.line, nil)
			if score > 0.1 {
				t.Errorf("%s: got score %v, want < 0.1", tt.desc, score)
			}
		})
	}
}

func TestParser_ContinueMultiLine_Summary(t *testing.T) {
	p := NewParser()
	p.inStylish = true
	p.currentFile = "/path/to/file.js"

	// Summary line should end multi-line sequence and reset
	continued := p.ContinueMultiLine("✖ 2 problems (1 error, 1 warning)", nil)
	if continued {
		t.Error("expected ContinueMultiLine to return false for summary line")
	}
	if p.inStylish {
		t.Error("expected inStylish to be false after summary")
	}
	if p.currentFile != "" {
		t.Error("expected currentFile to be empty after summary")
	}
}

func TestParser_ContinueMultiLine_NewFile(t *testing.T) {
	p := NewParser()
	p.inStylish = true
	p.currentFile = "/path/to/file.js"

	// New file path should end current file's sequence
	continued := p.ContinueMultiLine("/path/to/other.js", nil)
	if continued {
		t.Error("expected ContinueMultiLine to return false for new file path")
	}
}

func TestParser_FinishMultiLine(t *testing.T) {
	p := NewParser()
	p.inStylish = true
	p.currentFile = "/path/to/file.js"

	result := p.FinishMultiLine(nil)
	if result != nil {
		t.Error("expected FinishMultiLine to return nil")
	}
	if p.inStylish {
		t.Error("expected inStylish to be false after FinishMultiLine")
	}
	if p.currentFile != "" {
		t.Error("expected currentFile to be empty after FinishMultiLine")
	}
}

func TestParser_ContextLastFileUpdate(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Parse file path should update context
	p.Parse("/path/to/file.js", ctx)

	if ctx.LastFile != "/path/to/file.js" {
		t.Errorf("ctx.LastFile = %q, want %q", ctx.LastFile, "/path/to/file.js")
	}
}

func TestParser_StylishErrorWithoutFileContext(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{LastFile: "/context/file.js"}

	// Parse error line without setting file first - should use context
	err := p.Parse("  8:11  error  Some error  no-var", ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.File != "/context/file.js" {
		t.Errorf("File = %q, want %q", err.File, "/context/file.js")
	}
}
