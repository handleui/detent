package golang

import (
	"testing"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

func TestParser_ID(t *testing.T) {
	p := NewParser()
	if p.ID() != "go" {
		t.Errorf("expected ID 'go', got %q", p.ID())
	}
}

func TestParser_Priority(t *testing.T) {
	p := NewParser()
	if p.Priority() != 90 {
		t.Errorf("expected priority 90, got %d", p.Priority())
	}
}

func TestParser_CanParse(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		minScore float64
		maxScore float64
	}{
		{
			name:     "go compiler error",
			line:     "main.go:10:5: undefined: foo",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "golangci-lint error",
			line:     "internal/tui/jobtracker.go:119:9: ineffectual assignment to err (ineffassign)",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "test failure",
			line:     "--- FAIL: TestSomething (0.00s)",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "panic",
			line:     "panic: runtime error: invalid memory address or nil pointer dereference",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "goroutine header",
			line:     "goroutine 1 [running]:",
			minScore: 0.7,
			maxScore: 0.9,
		},
		{
			name:     "unrelated line",
			line:     "Building project...",
			minScore: 0,
			maxScore: 0.1,
		},
		{
			name:     "empty line",
			line:     "",
			minScore: 0,
			maxScore: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := p.CanParse(tt.line, nil)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("expected score between %.2f and %.2f, got %.2f", tt.minScore, tt.maxScore, score)
			}
		})
	}
}

func TestParser_Parse_GoCompilerError(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		wantFile string
		wantLine int
		wantCol  int
		wantMsg  string
	}{
		{
			name:     "simple undefined error",
			line:     "main.go:10:5: undefined: foo",
			wantFile: "main.go",
			wantLine: 10,
			wantCol:  5,
			wantMsg:  "undefined: foo",
		},
		{
			name:     "type mismatch error",
			line:     "internal/pkg/file.go:25:10: cannot use x (type int) as type string",
			wantFile: "internal/pkg/file.go",
			wantLine: 25,
			wantCol:  10,
			wantMsg:  "cannot use x (type int) as type string",
		},
		{
			name:     "error with extra whitespace",
			line:     "main.go:1:1:   some error with spaces   ",
			wantFile: "main.go",
			wantLine: 1,
			wantCol:  1,
			wantMsg:  "some error with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if err.Source != errors.SourceGo {
				t.Errorf("Source = %q, want %q", err.Source, errors.SourceGo)
			}
			if err.Category != errors.CategoryCompile {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryCompile)
			}
			if err.Severity != "error" {
				t.Errorf("Severity = %q, want %q", err.Severity, "error")
			}
		})
	}
}

func TestParser_Parse_GolangciLintError(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name       string
		line       string
		wantFile   string
		wantLine   int
		wantCol    int
		wantMsg    string
		wantRuleID string
	}{
		{
			name:       "ineffassign",
			line:       "internal/tui/jobtracker.go:119:9: ineffectual assignment to err (ineffassign)",
			wantFile:   "internal/tui/jobtracker.go",
			wantLine:   119,
			wantCol:    9,
			wantMsg:    "ineffectual assignment to err",
			wantRuleID: "ineffassign",
		},
		{
			name:       "staticcheck with code",
			line:       "internal/errors/extractor.go:45:2: SA4006: this value of `lastFile` is never used (staticcheck)",
			wantFile:   "internal/errors/extractor.go",
			wantLine:   45,
			wantCol:    2,
			wantMsg:    "this value of `lastFile` is never used",
			wantRuleID: "SA4006/staticcheck",
		},
		{
			name:       "govet",
			line:       "internal/config/config.go:50:12: printf: fmt.Sprintf format %s reads arg #1, but call has 0 args (govet)",
			wantFile:   "internal/config/config.go",
			wantLine:   50,
			wantCol:    12,
			wantMsg:    "printf: fmt.Sprintf format %s reads arg #1, but call has 0 args",
			wantRuleID: "govet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if err.RuleID != tt.wantRuleID {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRuleID)
			}
			if err.Category != errors.CategoryLint {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryLint)
			}
		})
	}
}

func TestParser_Parse_TestFailure(t *testing.T) {
	p := NewParser()

	// Start test failure
	err := p.Parse("--- FAIL: TestSomething (0.00s)", nil)
	if err != nil {
		t.Error("expected nil during test failure accumulation")
	}

	if !p.test.inTestFailure {
		t.Error("expected inTestFailure to be true")
	}

	// Continue with test output
	continued := p.ContinueMultiLine("    file_test.go:25: expected 1, got 2", nil)
	if !continued {
		t.Error("expected ContinueMultiLine to return true for test output")
	}

	// Finish with non-indented line
	continued = p.ContinueMultiLine("FAIL", nil)
	if continued {
		t.Error("expected ContinueMultiLine to return false for non-continuation")
	}

	// Finalize
	result := p.FinishMultiLine(nil)
	if result == nil {
		t.Fatal("expected error result from FinishMultiLine")
	}

	if result.File != "file_test.go" {
		t.Errorf("File = %q, want %q", result.File, "file_test.go")
	}
	if result.Line != 25 {
		t.Errorf("Line = %d, want %d", result.Line, 25)
	}
	if result.Message != "expected 1, got 2" {
		t.Errorf("Message = %q, want %q", result.Message, "expected 1, got 2")
	}
	if result.Category != errors.CategoryTest {
		t.Errorf("Category = %q, want %q", result.Category, errors.CategoryTest)
	}
	if result.Source != errors.SourceGoTest {
		t.Errorf("Source = %q, want %q", result.Source, errors.SourceGoTest)
	}
}

func TestParser_Parse_Panic(t *testing.T) {
	p := NewParser()

	lines := []string{
		"panic: runtime error: invalid memory address or nil pointer dereference",
		"goroutine 1 [running]:",
		"main.foo(0x0)",
		"        /path/to/file.go:10 +0x25",
		"main.main()",
		"        /path/to/main.go:5 +0x20",
	}

	// Start panic
	err := p.Parse(lines[0], nil)
	if err != nil {
		t.Error("expected nil during panic accumulation")
	}

	if !p.panic.inPanic {
		t.Error("expected inPanic to be true")
	}

	// Continue with stack trace
	for _, line := range lines[1:] {
		continued := p.ContinueMultiLine(line, nil)
		if !continued {
			t.Errorf("expected ContinueMultiLine to return true for %q", line)
		}
	}

	// End with empty line
	continued := p.ContinueMultiLine("", nil)
	if continued {
		t.Error("expected ContinueMultiLine to return false for empty line after goroutine")
	}

	// Finalize
	result := p.FinishMultiLine(nil)
	if result == nil {
		t.Fatal("expected error result from FinishMultiLine")
	}

	if result.Message != "panic: runtime error: invalid memory address or nil pointer dereference" {
		t.Errorf("Message = %q", result.Message)
	}
	if result.File != "/path/to/file.go" {
		t.Errorf("File = %q, want %q", result.File, "/path/to/file.go")
	}
	if result.Line != 10 {
		t.Errorf("Line = %d, want %d", result.Line, 10)
	}
	if result.Category != errors.CategoryRuntime {
		t.Errorf("Category = %q, want %q", result.Category, errors.CategoryRuntime)
	}
	if result.Source != errors.SourceGo {
		t.Errorf("Source = %q, want %q", result.Source, errors.SourceGo)
	}
	if result.StackTrace == "" {
		t.Error("expected StackTrace to be populated")
	}
}

func TestParser_IsNoise(t *testing.T) {
	p := NewParser()

	noiseLines := []string{
		"=== RUN   TestSomething",
		"=== PAUSE TestSomething",
		"=== CONT  TestSomething",
		"--- PASS: TestSomething (0.00s)",
		"PASS",
		"ok      github.com/example/pkg  0.123s",
		"?       github.com/example/nopkg       [no test files]",
		"FAIL    github.com/example/pkg 0.456s",
	}

	for _, line := range noiseLines {
		if !p.IsNoise(line) {
			t.Errorf("expected %q to be noise", line)
		}
	}

	nonNoiseLines := []string{
		"main.go:10:5: undefined: foo",
		"--- FAIL: TestSomething (0.00s)",
		"panic: something bad",
		"Building...",
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

func TestParser_Reset(t *testing.T) {
	p := NewParser()

	// Set up some state
	p.panic.inPanic = true
	p.panic.message = "test"
	p.test.inTestFailure = true
	p.test.testName = "TestFoo"

	p.Reset()

	if p.panic.inPanic {
		t.Error("expected inPanic to be false after Reset")
	}
	if p.panic.message != "" {
		t.Error("expected panicMessage to be empty after Reset")
	}
	if p.test.inTestFailure {
		t.Error("expected inTestFailure to be false after Reset")
	}
	if p.test.testName != "" {
		t.Error("expected testName to be empty after Reset")
	}
}

func TestParser_WorkflowContext(t *testing.T) {
	p := NewParser()

	ctx := &parser.ParseContext{
		Job:  "test-job",
		Step: "Run tests",
		WorkflowContext: &errors.WorkflowContext{
			Job:  "test-job",
			Step: "Run tests",
		},
	}

	err := p.Parse("main.go:10:5: undefined: foo", ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.WorkflowContext == nil {
		t.Fatal("expected WorkflowContext to be set")
	}

	if err.WorkflowContext.Job != "test-job" {
		t.Errorf("WorkflowContext.Job = %q, want %q", err.WorkflowContext.Job, "test-job")
	}
}

func TestParser_LintCategoryFromContext(t *testing.T) {
	p := NewParser()

	ctx := &parser.ParseContext{
		Step: "Run golangci-lint",
	}

	// Even without rule pattern, should detect lint from step name
	err := p.Parse("main.go:10:5: some generic message", ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Category != errors.CategoryLint {
		t.Errorf("Category = %q, want %q", err.Category, errors.CategoryLint)
	}
}

func TestParser_IntegrationScenario(t *testing.T) {
	// Simulate a real scenario with mixed output
	p := NewParser()

	// Compiler error
	err := p.Parse("cmd/main.go:15:3: undefined: Config", nil)
	if err == nil || err.File != "cmd/main.go" {
		t.Error("failed to parse compiler error")
	}

	// Lint error
	err = p.Parse("internal/handler.go:42:7: exported function Handler should have comment or be unexported (golint)", nil)
	if err == nil || err.RuleID != "golint" {
		t.Error("failed to parse lint error with rule")
	}

	// Reset for test failure scenario
	p.Reset()

	// Test failure
	p.Parse("--- FAIL: TestHandler (0.01s)", nil)
	p.ContinueMultiLine("    handler_test.go:55: handler returned wrong status code: got 500 want 200", nil)
	p.ContinueMultiLine("", nil)
	p.ContinueMultiLine("FAIL", nil)

	result := p.FinishMultiLine(nil)
	if result == nil || result.Source != errors.SourceGoTest {
		t.Error("failed to parse test failure")
	}
}

func TestParser_StaticCheckCode(t *testing.T) {
	p := NewParser()

	// Test SA codes without rule suffix
	line := "internal/errors/extractor.go:45:2: SA4006: this value of `lastFile` is never used"
	err := p.Parse(line, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should extract the SA code
	if err.RuleID != "SA4006" {
		t.Errorf("RuleID = %q, want %q", err.RuleID, "SA4006")
	}
	if err.Message != "this value of `lastFile` is never used" {
		t.Errorf("Message = %q", err.Message)
	}
}

func TestParser_ResetClearsAllState(t *testing.T) {
	p := NewParser()

	// Simulate panic accumulation
	p.panic.inPanic = true
	p.panic.message = "test panic"
	p.panic.file = "/path/to/file.go"
	p.panic.line = 42
	p.panic.stackTrace.WriteString("goroutine 1 [running]:")
	p.panic.goroutineSeen = true

	// Simulate test failure accumulation
	p.test.inTestFailure = true
	p.test.testName = "TestFoo"
	p.test.file = "foo_test.go"
	p.test.line = 10
	p.test.message = "assertion failed"
	p.test.stackTrace.WriteString("    foo_test.go:10: assertion failed")

	p.Reset()

	// Verify panic state is cleared
	if p.panic.inPanic {
		t.Error("inPanic should be false after Reset")
	}
	if p.panic.message != "" {
		t.Error("panicMessage should be empty after Reset")
	}
	if p.panic.file != "" {
		t.Error("panicFile should be empty after Reset")
	}
	if p.panic.line != 0 {
		t.Error("panicLine should be 0 after Reset")
	}
	if p.panic.stackTrace.Len() != 0 {
		t.Error("stackTrace should be empty after Reset")
	}
	if p.panic.goroutineSeen {
		t.Error("goroutineSeen should be false after Reset")
	}

	// Verify test failure state is cleared
	if p.test.inTestFailure {
		t.Error("inTestFailure should be false after Reset")
	}
	if p.test.testName != "" {
		t.Error("testName should be empty after Reset")
	}
	if p.test.file != "" {
		t.Error("testFile should be empty after Reset")
	}
	if p.test.line != 0 {
		t.Error("testLine should be 0 after Reset")
	}
	if p.test.message != "" {
		t.Error("testMessage should be empty after Reset")
	}
	if p.test.stackTrace.Len() != 0 {
		t.Error("testStackTrace should be empty after Reset")
	}
}

func TestParser_CanParseInMultiLineState(t *testing.T) {
	p := NewParser()

	// When not in multi-line state, unrelated lines return 0
	score := p.CanParse("unrelated line", nil)
	if score != 0 {
		t.Errorf("expected 0 for unrelated line, got %f", score)
	}

	// When in panic state, should return high confidence
	p.panic.inPanic = true
	score = p.CanParse("unrelated line", nil)
	if score < 0.8 {
		t.Errorf("expected high score in panic state, got %f", score)
	}
	p.Reset()

	// When in test failure state, should return high confidence
	p.test.inTestFailure = true
	score = p.CanParse("unrelated line", nil)
	if score < 0.8 {
		t.Errorf("expected high score in test failure state, got %f", score)
	}
}

func TestParser_PanicWithCreatedBy(t *testing.T) {
	p := NewParser()

	lines := []string{
		"panic: send on closed channel",
		"",
		"goroutine 34 [running]:",
		"main.sendData(0x0)",
		"        /path/to/sender.go:25 +0x80",
		"created by main.startWorker",
		"        /path/to/worker.go:10 +0x40",
	}

	// Start panic
	err := p.Parse(lines[0], nil)
	if err != nil {
		t.Error("expected nil during panic accumulation")
	}

	// Continue with all lines
	for _, line := range lines[1:] {
		p.ContinueMultiLine(line, nil)
	}

	// End with empty line after goroutine
	continued := p.ContinueMultiLine("", nil)
	if continued {
		t.Error("expected ContinueMultiLine to return false for empty line after goroutine")
	}

	result := p.FinishMultiLine(nil)
	if result == nil {
		t.Fatal("expected error result from FinishMultiLine")
	}

	if result.Message != "panic: send on closed channel" {
		t.Errorf("Message = %q", result.Message)
	}

	// Check that stack trace contains "created by"
	if result.StackTrace == "" {
		t.Error("expected StackTrace to be populated")
	}
}

func TestParser_TestFailureWithMultipleOutputLines(t *testing.T) {
	p := NewParser()

	// Start test failure
	p.Parse("--- FAIL: TestComplex (0.05s)", nil)

	// Continue with multiple test output lines
	lines := []string{
		"    complex_test.go:10: Step 1 failed",
		"        Expected: 100",
		"        Got: 200",
		"    complex_test.go:15: Step 2 also failed",
	}

	for _, line := range lines {
		continued := p.ContinueMultiLine(line, nil)
		if !continued {
			t.Errorf("expected ContinueMultiLine to return true for %q", line)
		}
	}

	// End with non-indented line
	p.ContinueMultiLine("FAIL", nil)

	result := p.FinishMultiLine(nil)
	if result == nil {
		t.Fatal("expected error result from FinishMultiLine")
	}

	// First file/line should be captured
	if result.File != "complex_test.go" {
		t.Errorf("File = %q, want %q", result.File, "complex_test.go")
	}
	if result.Line != 10 {
		t.Errorf("Line = %d, want %d", result.Line, 10)
	}
	if result.Message != "Step 1 failed" {
		t.Errorf("Message = %q, want %q", result.Message, "Step 1 failed")
	}

	// Stack trace should contain all lines
	if result.StackTrace == "" {
		t.Error("expected StackTrace to be populated")
	}
}

func TestParser_FinishMultiLineWithoutState(t *testing.T) {
	p := NewParser()

	// Calling FinishMultiLine without starting multi-line should return nil
	result := p.FinishMultiLine(nil)
	if result != nil {
		t.Error("expected nil when not in multi-line state")
	}
}

func TestParser_ContinueMultiLineWithoutState(t *testing.T) {
	p := NewParser()

	// Calling ContinueMultiLine without starting multi-line should return false
	continued := p.ContinueMultiLine("some line", nil)
	if continued {
		t.Error("expected false when not in multi-line state")
	}
}

func TestParser_AbsolutePaths(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		wantFile string
	}{
		{
			name:     "Unix absolute path",
			line:     "/home/user/project/main.go:10:5: undefined: foo",
			wantFile: "/home/user/project/main.go",
		},
		{
			name:     "deeply nested path",
			line:     "/home/user/go/src/github.com/org/repo/internal/pkg/main.go:10:5: undefined: foo",
			wantFile: "/home/user/go/src/github.com/org/repo/internal/pkg/main.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Parse(tt.line, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.File != tt.wantFile {
				t.Errorf("File = %q, want %q", err.File, tt.wantFile)
			}
		})
	}
}

func TestParser_TestFailureWithWorkflowContext(t *testing.T) {
	p := NewParser()

	ctx := &parser.ParseContext{
		Job:  "test-job",
		Step: "Run go test",
		WorkflowContext: &errors.WorkflowContext{
			Job:  "test-job",
			Step: "Run go test",
		},
	}

	// Start test failure
	p.Parse("--- FAIL: TestWithContext (0.00s)", nil)
	p.ContinueMultiLine("    context_test.go:25: assertion failed", nil)
	p.ContinueMultiLine("FAIL", nil)

	result := p.FinishMultiLine(ctx)
	if result == nil {
		t.Fatal("expected error result")
	}

	if result.WorkflowContext == nil {
		t.Fatal("expected WorkflowContext to be set")
	}

	if result.WorkflowContext.Job != "test-job" {
		t.Errorf("WorkflowContext.Job = %q, want %q", result.WorkflowContext.Job, "test-job")
	}
}

func TestParser_PanicWithWorkflowContext(t *testing.T) {
	p := NewParser()

	ctx := &parser.ParseContext{
		Job:  "test-job",
		Step: "Run tests",
		WorkflowContext: &errors.WorkflowContext{
			Job:  "test-job",
			Step: "Run tests",
		},
	}

	// Start panic
	p.Parse("panic: nil pointer", nil)
	p.ContinueMultiLine("goroutine 1 [running]:", nil)
	p.ContinueMultiLine("main.foo()", nil)
	p.ContinueMultiLine("        /path/file.go:10 +0x20", nil)
	p.ContinueMultiLine("", nil)

	result := p.FinishMultiLine(ctx)
	if result == nil {
		t.Fatal("expected error result")
	}

	if result.WorkflowContext == nil {
		t.Fatal("expected WorkflowContext to be set")
	}

	if result.WorkflowContext.Job != "test-job" {
		t.Errorf("WorkflowContext.Job = %q, want %q", result.WorkflowContext.Job, "test-job")
	}
}

func TestParser_SeverityDetection(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name         string
		line         string
		wantSeverity string
		wantRuleID   string
	}{
		// Error-level linters (bugs, security)
		{
			name:         "staticcheck SA code (error)",
			line:         "main.go:10:5: SA4006: this value is never used (staticcheck)",
			wantSeverity: "error",
			wantRuleID:   "SA4006/staticcheck",
		},
		{
			name:         "gosec G code (error)",
			line:         "main.go:10:5: G101: Potential hardcoded credentials (gosec)",
			wantSeverity: "error",
			wantRuleID:   "G101/gosec",
		},
		{
			name:         "errcheck (error)",
			line:         "main.go:10:5: Error return value is not checked (errcheck)",
			wantSeverity: "error",
			wantRuleID:   "errcheck",
		},
		{
			name:         "govet (error)",
			line:         "main.go:10:5: printf: Sprintf format has no args (govet)",
			wantSeverity: "error",
			wantRuleID:   "govet",
		},
		{
			name:         "ineffassign (error)",
			line:         "main.go:10:5: ineffectual assignment to err (ineffassign)",
			wantSeverity: "error",
			wantRuleID:   "ineffassign",
		},
		{
			name:         "bodyclose (error)",
			line:         "main.go:10:5: response body must be closed (bodyclose)",
			wantSeverity: "error",
			wantRuleID:   "bodyclose",
		},

		// Warning-level linters (style, complexity)
		{
			name:         "stylecheck ST code (warning)",
			line:         "main.go:10:5: ST1000: package comment should be of the form (stylecheck)",
			wantSeverity: "warning",
			wantRuleID:   "ST1000/stylecheck",
		},
		{
			name:         "simple S code (warning)",
			line:         "main.go:10:5: S1000: should use single case select (staticcheck)",
			wantSeverity: "warning",
			wantRuleID:   "S1000/staticcheck",
		},
		{
			name:         "quickfix QF code (warning)",
			line:         "main.go:10:5: QF1001: apply De Morgan's law (staticcheck)",
			wantSeverity: "warning",
			wantRuleID:   "QF1001/staticcheck",
		},
		{
			name:         "gocritic (warning)",
			line:         "main.go:10:5: appendAssign: append result not assigned (gocritic)",
			wantSeverity: "warning",
			wantRuleID:   "gocritic",
		},
		{
			name:         "gocyclo (warning)",
			line:         "main.go:10:5: cyclomatic complexity 15 of function foo (gocyclo)",
			wantSeverity: "warning",
			wantRuleID:   "gocyclo",
		},
		{
			name:         "misspell (warning)",
			line:         "main.go:10:5: \"recieve\" is a misspelling of \"receive\" (misspell)", //nolint:misspell // intentional test case
			wantSeverity: "warning",
			wantRuleID:   "misspell",
		},
		{
			name:         "golint (warning)",
			line:         "main.go:10:5: exported function Foo should have comment (golint)",
			wantSeverity: "warning",
			wantRuleID:   "golint",
		},
		{
			name:         "revive (warning)",
			line:         "main.go:10:5: exported: exported function Foo should have comment (revive)",
			wantSeverity: "warning",
			wantRuleID:   "revive",
		},
		{
			name:         "gofmt (warning)",
			line:         "main.go:10:5: File is not `gofmt`-ed (gofmt)",
			wantSeverity: "warning",
			wantRuleID:   "gofmt",
		},

		// Static check codes without linter suffix
		{
			name:         "SA code only (error)",
			line:         "main.go:10:5: SA4006: value never used",
			wantSeverity: "error",
			wantRuleID:   "SA4006",
		},
		{
			name:         "S code only (warning)",
			line:         "main.go:10:5: S1000: simplify select",
			wantSeverity: "warning",
			wantRuleID:   "S1000",
		},
		{
			name:         "G code only (error)",
			line:         "main.go:10:5: G101: hardcoded credentials",
			wantSeverity: "error",
			wantRuleID:   "G101",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Parse(tt.line, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSeverity)
			}
			if err.RuleID != tt.wantRuleID {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRuleID)
			}
			if err.Category != errors.CategoryLint {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryLint)
			}
		})
	}
}

func TestParser_ExtractCodePrefix(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"SA4006", "SA"},
		{"S1000", "S"},
		{"ST1000", "ST"},
		{"QF1001", "QF"},
		{"G101", "G"},
		{"ABC", "ABC"},
		{"", ""},
		{"123", ""},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := extractCodePrefix(tt.code)
			if got != tt.want {
				t.Errorf("extractCodePrefix(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestParser_DetermineLintSeverity(t *testing.T) {
	tests := []struct {
		linterName string
		codePrefix string
		want       string
	}{
		// Code prefix takes priority
		{"staticcheck", "SA", "error"},
		{"staticcheck", "S", "warning"},
		{"staticcheck", "ST", "warning"},
		{"staticcheck", "QF", "warning"},
		{"gosec", "G", "error"},

		// Linter name when no code prefix
		{"gosec", "", "error"},
		{"errcheck", "", "error"},
		{"govet", "", "error"},
		{"gocritic", "", "warning"},
		{"misspell", "", "warning"},
		{"golint", "", "warning"},

		// Unknown linter defaults to error
		{"unknown", "", "error"},
		{"", "", "error"},
	}

	for _, tt := range tests {
		name := tt.linterName
		if tt.codePrefix != "" {
			name = tt.codePrefix + "/" + tt.linterName
		}
		t.Run(name, func(t *testing.T) {
			got := determineLintSeverity(tt.linterName, tt.codePrefix)
			if got != tt.want {
				t.Errorf("determineLintSeverity(%q, %q) = %q, want %q",
					tt.linterName, tt.codePrefix, got, tt.want)
			}
		})
	}
}

func TestParser_IsNoise_Extended(t *testing.T) {
	p := NewParser()

	// Extended noise patterns
	noiseLines := []string{
		"=== RUN   TestSomething",
		"=== PAUSE TestSomething",
		"=== CONT  TestSomething",
		"=== NAME  TestSomething",
		"--- PASS: TestSomething (0.00s)",
		"--- SKIP: TestSomething (0.00s)",
		"PASS",
		"ok      github.com/example/pkg  0.123s",
		"?       github.com/example/nopkg       [no test files]",
		"FAIL    github.com/example/pkg 0.456s",
		"# github.com/example/pkg",
		"go: downloading github.com/some/pkg v1.0.0",
		"go: finding module for package github.com/some/pkg",
		"level=info msg=\"starting analysis\"",
		"Running linters...",
		"Issues: 5",
		"coverage: 85.0% of statements",
		"    --- PASS: TestSomething/subtest (0.00s)",
	}

	for _, line := range noiseLines {
		if !p.IsNoise(line) {
			t.Errorf("expected %q to be noise", line)
		}
	}

	// Non-noise lines
	nonNoiseLines := []string{
		"main.go:10:5: undefined: foo",
		"--- FAIL: TestSomething (0.00s)",
		"panic: something bad",
		"Building...",
		"internal/pkg.go:25:10: SA4006: value unused (staticcheck)",
	}

	for _, line := range nonNoiseLines {
		if p.IsNoise(line) {
			t.Errorf("expected %q to NOT be noise", line)
		}
	}
}

func TestParser_AllKnownLinters(t *testing.T) {
	// Verify that all known linters have valid severity values
	for linter, severity := range KnownLinters {
		if severity != "error" && severity != "warning" {
			t.Errorf("linter %q has invalid severity %q (must be 'error' or 'warning')",
				linter, severity)
		}
	}
}

func TestParser_AllCodePrefixes(t *testing.T) {
	// Verify that all code prefixes have valid severity values
	for prefix, severity := range CodePrefixSeverity {
		if severity != "error" && severity != "warning" {
			t.Errorf("code prefix %q has invalid severity %q (must be 'error' or 'warning')",
				prefix, severity)
		}
	}
}

func TestParser_ErrorWithoutColumn(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		wantFile string
		wantLine int
		wantMsg  string
	}{
		{
			name:     "import cycle error",
			line:     "main.go:10: import cycle not allowed",
			wantFile: "main.go",
			wantLine: 10,
			wantMsg:  "import cycle not allowed",
		},
		{
			name:     "simple error without column",
			line:     "internal/pkg/file.go:25: some error message",
			wantFile: "internal/pkg/file.go",
			wantLine: 25,
			wantMsg:  "some error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if err.Column != 0 {
				t.Errorf("Column = %d, want 0 (no column)", err.Column)
			}
			if err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", err.Message, tt.wantMsg)
			}
			if err.Source != errors.SourceGo {
				t.Errorf("Source = %q, want %q", err.Source, errors.SourceGo)
			}
		})
	}
}

func TestParser_ModuleErrors(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name    string
		line    string
		wantMsg string
	}{
		{
			name:    "module version error",
			line:    "go: example.com/pkg@v1.0.0: invalid version",
			wantMsg: "example.com/pkg@v1.0.0: invalid version",
		},
		{
			name:    "module not found",
			line:    "go: module example.com/pkg: not a module",
			wantMsg: "module example.com/pkg: not a module",
		},
		{
			name:    "go.mod version error",
			line:    "go.mod:3: invalid go version '1.22foo'",
			wantMsg: "invalid go version '1.22foo'",
		},
		{
			name:    "go downloading message",
			line:    "go: downloading github.com/some/pkg v1.0.0",
			wantMsg: "downloading github.com/some/pkg v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Parse(tt.line, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", err.Message, tt.wantMsg)
			}
			if err.Source != errors.SourceGo {
				t.Errorf("Source = %q, want %q", err.Source, errors.SourceGo)
			}
			if err.Severity != "error" {
				t.Errorf("Severity = %q, want %q", err.Severity, "error")
			}
		})
	}
}

func TestParser_CanParse_NewPatterns(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		line     string
		minScore float64
		maxScore float64
	}{
		{
			name:     "error without column",
			line:     "main.go:10: import cycle not allowed",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "module error",
			line:     "go: example.com/pkg@v1.0.0: invalid version",
			minScore: 0.85,
			maxScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := p.CanParse(tt.line, nil)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("expected score between %.2f and %.2f, got %.2f", tt.minScore, tt.maxScore, score)
			}
		})
	}
}

func TestParser_SubtestFailure(t *testing.T) {
	p := NewParser()

	// Start test failure with subtest name
	err := p.Parse("--- FAIL: TestSomething/subtest_name (0.00s)", nil)
	if err != nil {
		t.Error("expected nil during test failure accumulation")
	}

	if !p.test.inTestFailure {
		t.Error("expected inTestFailure to be true")
	}

	if p.test.testName != "TestSomething/subtest_name" {
		t.Errorf("testName = %q, want %q", p.test.testName, "TestSomething/subtest_name")
	}

	// Continue with test output
	continued := p.ContinueMultiLine("    file_test.go:25: expected 1, got 2", nil)
	if !continued {
		t.Error("expected ContinueMultiLine to return true for test output")
	}

	// End with non-indented line
	continued = p.ContinueMultiLine("FAIL", nil)
	if continued {
		t.Error("expected ContinueMultiLine to return false for non-continuation")
	}

	// Finalize
	result := p.FinishMultiLine(nil)
	if result == nil {
		t.Fatal("expected error result from FinishMultiLine")
	}

	if result.File != "file_test.go" {
		t.Errorf("File = %q, want %q", result.File, "file_test.go")
	}
	if result.Category != errors.CategoryTest {
		t.Errorf("Category = %q, want %q", result.Category, errors.CategoryTest)
	}
}

func TestParser_BuildConstraintPatterns(t *testing.T) {
	// Test that build constraint patterns match correctly
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{"build constraints exclude", "build constraints exclude all Go files", true},
		{"build constraint exclude singular", "build constraint exclude all Go files", true},
		{"no buildable go files", "no buildable go files in /path/to/pkg", true},
		{"no go source files", "no go source files in /path/to/pkg", true},
		{"no go files", "no go files in /path/to/pkg", true},
		{"unrelated", "some other error message", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := goBuildConstraintPattern.MatchString(tt.input)
			if matched != tt.match {
				t.Errorf("goBuildConstraintPattern.MatchString(%q) = %v, want %v",
					tt.input, matched, tt.match)
			}
		})
	}
}

func TestParser_ImportCyclePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{"import cycle", "import cycle not allowed", true},
		{"package import cycle", "package import cycle not allowed", true},
		{"unrelated", "undefined: foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := goImportCyclePattern.MatchString(tt.input)
			if matched != tt.match {
				t.Errorf("goImportCyclePattern.MatchString(%q) = %v, want %v",
					tt.input, matched, tt.match)
			}
		})
	}
}

func TestParser_AdditionalLinters(t *testing.T) {
	p := NewParser()

	// Test some of the newly added linters
	tests := []struct {
		name         string
		line         string
		wantSeverity string
		wantRuleID   string
	}{
		{
			name:         "unused linter (error)",
			line:         "main.go:10:5: foo declared but not used (unused)",
			wantSeverity: "error",
			wantRuleID:   "unused",
		},
		{
			name:         "gosimple linter (warning)",
			line:         "main.go:10:5: should use simple statement (gosimple)",
			wantSeverity: "warning",
			wantRuleID:   "gosimple",
		},
		{
			name:         "testifylint linter (warning)",
			line:         "main.go:10:5: use require.Equal (testifylint)",
			wantSeverity: "warning",
			wantRuleID:   "testifylint",
		},
		{
			name:         "copyloopvar linter (error)",
			line:         "main.go:10:5: loop variable captured by func literal (copyloopvar)",
			wantSeverity: "error",
			wantRuleID:   "copyloopvar",
		},
		{
			name:         "fatcontext linter (error)",
			line:         "main.go:10:5: context should not be stored in struct (fatcontext)",
			wantSeverity: "error",
			wantRuleID:   "fatcontext",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Parse(tt.line, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSeverity)
			}
			if err.RuleID != tt.wantRuleID {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRuleID)
			}
		})
	}
}
