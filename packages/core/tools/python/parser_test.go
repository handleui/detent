package python

import (
	"strings"
	"testing"

	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/tools/parser"
)

func TestParser_ID(t *testing.T) {
	p := NewParser()
	if p.ID() != "python" {
		t.Errorf("expected ID 'python', got %q", p.ID())
	}
}

func TestParser_Priority(t *testing.T) {
	p := NewParser()
	if p.Priority() != 90 {
		t.Errorf("expected priority 90, got %d", p.Priority())
	}
}

func TestParser_SupportsMultiLine(t *testing.T) {
	p := NewParser()
	if !p.SupportsMultiLine() {
		t.Error("expected SupportsMultiLine() to return true")
	}
}

func TestParser_CanParse(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		minScore float64
		maxScore float64
	}{
		{
			name:     "traceback start",
			line:     "Traceback (most recent call last):",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "exception line",
			line:     "ValueError: invalid value",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "pytest FAILED",
			line:     "FAILED tests/test_foo.py::test_bar - AssertionError: assert 1 == 2",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "pytest ERROR",
			line:     "ERROR tests/test_foo.py - ModuleNotFoundError: No module named 'foo'",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "mypy error",
			line:     "app/main.py:42: error: Argument 1 has incompatible type",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "ruff/flake8",
			line:     "app/main.py:42:10: E501 Line too long",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "pylint",
			line:     "app/main.py:42:0: C0114: Missing module docstring (missing-module-docstring)",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "SyntaxError",
			line:     "SyntaxError: invalid syntax",
			minScore: 0.8,
			maxScore: 1.0,
		},
		{
			name:     "unrelated line",
			line:     "hello world",
			minScore: 0.0,
			maxScore: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			score := p.CanParse(tt.line, nil)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("CanParse(%q) = %v, want [%v, %v]", tt.line, score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestParser_Parse_PytestFailed(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantFile     string
		wantMessage  string
		wantCategory errors.ErrorCategory
		wantRuleID   string
	}{
		{
			name:         "simple test failure",
			line:         "FAILED tests/test_foo.py::test_bar - AssertionError: assert 1 == 2",
			wantFile:     "tests/test_foo.py",
			wantMessage:  "Test failed: test_bar - AssertionError: assert 1 == 2",
			wantCategory: errors.CategoryTest,
			wantRuleID:   "test_bar",
		},
		{
			name:         "class method test failure",
			line:         "FAILED tests/test_foo.py::TestClass::test_method - ValueError: bad value",
			wantFile:     "tests/test_foo.py",
			wantMessage:  "Test failed: TestClass::test_method - ValueError: bad value",
			wantCategory: errors.CategoryTest,
			wantRuleID:   "TestClass::test_method",
		},
		{
			name:         "parametrized test failure",
			line:         "FAILED tests/test_foo.py::test_add[1-2-3] - AssertionError",
			wantFile:     "tests/test_foo.py",
			wantMessage:  "Test failed: test_add[1-2-3] - AssertionError",
			wantCategory: errors.CategoryTest,
			wantRuleID:   "test_add[1-2-3]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			ctx := &parser.ParseContext{}
			err := p.Parse(tt.line, ctx)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.File != tt.wantFile {
				t.Errorf("File = %q, want %q", err.File, tt.wantFile)
			}
			if err.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", err.Message, tt.wantMessage)
			}
			if err.Category != tt.wantCategory {
				t.Errorf("Category = %q, want %q", err.Category, tt.wantCategory)
			}
			if err.RuleID != tt.wantRuleID {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRuleID)
			}
			if err.Source != errors.SourcePython {
				t.Errorf("Source = %q, want %q", err.Source, errors.SourcePython)
			}
			if err.Severity != "error" {
				t.Errorf("Severity = %q, want %q", err.Severity, "error")
			}
		})
	}
}

func TestParser_Parse_PytestError(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}
	line := "ERROR tests/test_foo.py - ModuleNotFoundError: No module named 'foo'"

	err := p.Parse(line, ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.File != "tests/test_foo.py" {
		t.Errorf("File = %q, want %q", err.File, "tests/test_foo.py")
	}
	if !strings.Contains(err.Message, "Collection error:") {
		t.Errorf("Message should contain 'Collection error:', got %q", err.Message)
	}
	if err.Category != errors.CategoryTest {
		t.Errorf("Category = %q, want %q", err.Category, errors.CategoryTest)
	}
}

func TestParser_Parse_Mypy(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantFile     string
		wantLine     int
		wantSeverity string
		wantRuleID   string
		wantCategory errors.ErrorCategory
	}{
		{
			name:         "mypy error with rule ID",
			line:         `app/main.py:42: error: Argument 1 to "foo" has incompatible type "str"; expected "int" [arg-type]`,
			wantFile:     "app/main.py",
			wantLine:     42,
			wantSeverity: "error",
			wantRuleID:   "arg-type",
			wantCategory: errors.CategoryTypeCheck,
		},
		{
			name:         "mypy warning",
			line:         "app/main.py:50: warning: Unused variable [unused-variable]",
			wantFile:     "app/main.py",
			wantLine:     50,
			wantSeverity: "warning",
			wantRuleID:   "unused-variable",
			wantCategory: errors.CategoryTypeCheck,
		},
		{
			name:         "mypy note",
			line:         `app/main.py:10: note: Revealed type is "builtins.str"`,
			wantFile:     "app/main.py",
			wantLine:     10,
			wantSeverity: "warning", // Notes treated as warnings
			wantRuleID:   "",
			wantCategory: errors.CategoryTypeCheck,
		},
		{
			name:         "mypy error without rule ID",
			line:         "app/main.py:25: error: Cannot find implementation",
			wantFile:     "app/main.py",
			wantLine:     25,
			wantSeverity: "error",
			wantRuleID:   "",
			wantCategory: errors.CategoryTypeCheck,
		},
		{
			name:         "pyi stub file",
			line:         "stubs/mylib.pyi:10: error: Missing type annotation",
			wantFile:     "stubs/mylib.pyi",
			wantLine:     10,
			wantSeverity: "error",
			wantRuleID:   "",
			wantCategory: errors.CategoryTypeCheck,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			ctx := &parser.ParseContext{}
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
			if err.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSeverity)
			}
			if err.RuleID != tt.wantRuleID {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRuleID)
			}
			if err.Category != tt.wantCategory {
				t.Errorf("Category = %q, want %q", err.Category, tt.wantCategory)
			}
		})
	}
}

func TestParser_Parse_RuffFlake8(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantFile     string
		wantLine     int
		wantColumn   int
		wantRuleID   string
		wantSeverity string
	}{
		{
			name:         "E501 line too long",
			line:         "app/main.py:42:10: E501 Line too long (120 > 88)",
			wantFile:     "app/main.py",
			wantLine:     42,
			wantColumn:   10,
			wantRuleID:   "E501",
			wantSeverity: "warning", // E is style warning
		},
		{
			name:         "F401 unused import",
			line:         "app/main.py:1:1: F401 'os' imported but unused",
			wantFile:     "app/main.py",
			wantLine:     1,
			wantColumn:   1,
			wantRuleID:   "F401",
			wantSeverity: "error", // F4xx are import errors
		},
		{
			name:         "E999 syntax error",
			line:         "app/main.py:5:1: E999 SyntaxError: invalid syntax",
			wantFile:     "app/main.py",
			wantLine:     5,
			wantColumn:   1,
			wantRuleID:   "E999",
			wantSeverity: "error", // E9xx are runtime/syntax errors
		},
		{
			name:         "W503 line break before binary operator",
			line:         "app/main.py:10:5: W503 line break before binary operator",
			wantFile:     "app/main.py",
			wantLine:     10,
			wantColumn:   5,
			wantRuleID:   "W503",
			wantSeverity: "warning",
		},
		{
			name:         "F821 undefined name",
			line:         "app/main.py:20:5: F821 undefined name 'foo'",
			wantFile:     "app/main.py",
			wantLine:     20,
			wantColumn:   5,
			wantRuleID:   "F821",
			wantSeverity: "error", // F8xx are undefined name errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			ctx := &parser.ParseContext{}
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
			if err.Column != tt.wantColumn {
				t.Errorf("Column = %d, want %d", err.Column, tt.wantColumn)
			}
			if err.RuleID != tt.wantRuleID {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRuleID)
			}
			if err.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSeverity)
			}
			if err.Category != errors.CategoryLint {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryLint)
			}
		})
	}
}

func TestParser_Parse_RuffFlake8NoColumn(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}
	line := "app/main.py:42: E501 Line too long"

	err := p.Parse(line, ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.File != "app/main.py" {
		t.Errorf("File = %q, want %q", err.File, "app/main.py")
	}
	if err.Line != 42 {
		t.Errorf("Line = %d, want %d", err.Line, 42)
	}
	if err.Column != 0 {
		t.Errorf("Column = %d, want %d", err.Column, 0)
	}
	if err.RuleID != "E501" {
		t.Errorf("RuleID = %q, want %q", err.RuleID, "E501")
	}
}

func TestParser_Parse_Pylint(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantFile     string
		wantLine     int
		wantColumn   int
		wantRuleID   string
		wantSeverity string
	}{
		{
			name:         "C0114 missing docstring",
			line:         "app/main.py:1:0: C0114: Missing module docstring (missing-module-docstring)",
			wantFile:     "app/main.py",
			wantLine:     1,
			wantColumn:   0,
			wantRuleID:   "missing-module-docstring",
			wantSeverity: "warning",
		},
		{
			name:         "E1101 no-member",
			line:         "app/main.py:42:4: E1101: Instance of 'Foo' has no 'bar' member (no-member)",
			wantFile:     "app/main.py",
			wantLine:     42,
			wantColumn:   4,
			wantRuleID:   "no-member",
			wantSeverity: "error", // E = Error
		},
		{
			name:         "W0612 unused variable",
			line:         "app/main.py:10:4: W0612: Unused variable 'x' (unused-variable)",
			wantFile:     "app/main.py",
			wantLine:     10,
			wantColumn:   4,
			wantRuleID:   "unused-variable",
			wantSeverity: "warning", // W = Warning
		},
		{
			name:         "R0913 too many arguments",
			line:         "app/main.py:5:0: R0913: Too many arguments (6/5) (too-many-arguments)",
			wantFile:     "app/main.py",
			wantLine:     5,
			wantColumn:   0,
			wantRuleID:   "too-many-arguments",
			wantSeverity: "warning", // R = Refactor
		},
		{
			name:         "F0001 fatal error",
			line:         "app/main.py:0:0: F0001: error (fatal)",
			wantFile:     "app/main.py",
			wantLine:     0,
			wantColumn:   0,
			wantRuleID:   "fatal",
			wantSeverity: "error", // F = Fatal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			ctx := &parser.ParseContext{}
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
			if err.Column != tt.wantColumn {
				t.Errorf("Column = %d, want %d", err.Column, tt.wantColumn)
			}
			if err.RuleID != tt.wantRuleID {
				t.Errorf("RuleID = %q, want %q", err.RuleID, tt.wantRuleID)
			}
			if err.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", err.Severity, tt.wantSeverity)
			}
			if err.Category != errors.CategoryLint {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryLint)
			}
		})
	}
}

func TestParser_Traceback_Simple(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	lines := []string{
		"Traceback (most recent call last):",
		`  File "/path/to/main.py", line 10, in main`,
		"    result = divide(10, 0)",
		"ZeroDivisionError: division by zero",
	}

	// First line starts traceback
	err := p.Parse(lines[0], ctx)
	if err != nil {
		t.Errorf("Parse first line should return nil, got %v", err)
	}

	// Continue with middle lines
	for _, line := range lines[1:3] {
		if !p.ContinueMultiLine(line, ctx) {
			t.Errorf("ContinueMultiLine(%q) should return true", line)
		}
	}

	// Last line should signal end
	if p.ContinueMultiLine(lines[3], ctx) {
		t.Error("ContinueMultiLine for exception line should return false")
	}

	// Finish and get error
	err = p.FinishMultiLine(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.File != "/path/to/main.py" {
		t.Errorf("File = %q, want %q", err.File, "/path/to/main.py")
	}
	if err.Line != 10 {
		t.Errorf("Line = %d, want %d", err.Line, 10)
	}
	if !strings.Contains(err.Message, "ZeroDivisionError") {
		t.Errorf("Message should contain 'ZeroDivisionError', got %q", err.Message)
	}
	if err.Category != errors.CategoryRuntime {
		t.Errorf("Category = %q, want %q", err.Category, errors.CategoryRuntime)
	}
	if err.StackTrace == "" {
		t.Error("StackTrace should not be empty")
	}
}

func TestParser_Traceback_Nested(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Nested traceback - we want the DEEPEST frame (last File line)
	lines := []string{
		"Traceback (most recent call last):",
		`  File "/app/main.py", line 10, in main`,
		"    result = process(data)",
		`  File "/app/process.py", line 25, in process`,
		"    return transform(data)",
		`  File "/app/transform.py", line 42, in transform`,
		"    raise ValueError('bad data')",
		"ValueError: bad data",
	}

	p.Parse(lines[0], ctx)
	for _, line := range lines[1:7] {
		p.ContinueMultiLine(line, ctx)
	}
	p.ContinueMultiLine(lines[7], ctx)

	err := p.FinishMultiLine(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should extract the DEEPEST frame
	if err.File != "/app/transform.py" {
		t.Errorf("File = %q, want %q (deepest frame)", err.File, "/app/transform.py")
	}
	if err.Line != 42 {
		t.Errorf("Line = %d, want %d", err.Line, 42)
	}
}

func TestParser_Traceback_Chained(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	lines := []string{
		"Traceback (most recent call last):",
		`  File "/app/main.py", line 10, in main`,
		"    result = outer()",
		"KeyError: 'missing'",
		"",
		"During handling of the above exception, another exception occurred:",
		"",
		"Traceback (most recent call last):",
		`  File "/app/main.py", line 15, in main`,
		"    handle_error()",
		`  File "/app/handler.py", line 5, in handle_error`,
		"    raise RuntimeError('failed')",
		"RuntimeError: failed",
	}

	p.Parse(lines[0], ctx)
	for i := 1; i < len(lines)-1; i++ {
		p.ContinueMultiLine(lines[i], ctx)
	}
	p.ContinueMultiLine(lines[len(lines)-1], ctx)

	err := p.FinishMultiLine(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should have the last exception message
	if !strings.Contains(err.Message, "RuntimeError") {
		t.Errorf("Message should contain 'RuntimeError', got %q", err.Message)
	}
}

func TestParser_SyntaxError(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// SyntaxError has a special format with caret
	lines := []string{
		`  File "script.py", line 5`,
		`    while True print('Hello')`,
		`               ^`,
		`SyntaxError: invalid syntax`,
	}

	// The first file line should be detected - we need to start in traceback mode
	// In reality, SyntaxErrors often come without "Traceback" header
	// Let's test the standalone case
	err := p.Parse(lines[3], ctx)
	if err == nil {
		t.Fatal("expected error for standalone SyntaxError, got nil")
	}

	if !strings.Contains(err.Message, "SyntaxError") {
		t.Errorf("Message should contain 'SyntaxError', got %q", err.Message)
	}
	if err.Category != errors.CategoryCompile {
		t.Errorf("Category = %q, want %q", err.Category, errors.CategoryCompile)
	}
}

func TestParser_IsNoise(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		isNoise bool
	}{
		{"empty line", "", true},
		{"whitespace", "   ", true},
		{"pytest passed", "12 passed in 0.5s", true},
		{"pytest skipped", "5 skipped", true},
		{"pytest session starts", "test session starts", true},
		{"platform info", "platform linux -- Python 3.9.0", true},
		{"cachedir", "cachedir: .pytest_cache", true},
		{"rootdir", "rootdir: /app", true},
		{"collecting", "collecting ...", true},
		{"collected", "collected 10 items", true},
		{"coverage output", "Coverage report:", true},
		{"mypy success", "Success: no issues found", true},
		{"pylint rating", "Your code has been rated at 10/10", true},
		{"all checks passed", "All checks passed!", true},
		{"progress dots", ".....", true},
		{"unittest OK", "OK", true},

		// These should NOT be noise
		{"actual error", "ERROR tests/test.py - ImportError", false},
		{"traceback start", "Traceback (most recent call last):", false},
		{"file line", `  File "/app/main.py", line 10, in main`, false},
		{"exception", "ValueError: bad value", false},
		{"mypy error", "app/main.py:10: error: Type mismatch", false},
		{"flake8 error", "app/main.py:10:1: E501 Line too long", false},
		{"go test fail", "--- FAIL: TestFoo (0.00s)", false},
		{"rust error", "error[E0308]: mismatched types", false},
		{"ts error", "src/app.ts(5,10): error TS2322: Type 'string'", false},
	}

	p := NewParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.IsNoise(tt.line)
			if got != tt.isNoise {
				t.Errorf("IsNoise(%q) = %v, want %v", tt.line, got, tt.isNoise)
			}
		})
	}
}

func TestParser_Reset(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Start a traceback
	p.Parse("Traceback (most recent call last):", ctx)
	p.ContinueMultiLine(`  File "/app/main.py", line 10, in main`, ctx)

	// Parser should be in traceback mode
	if !p.traceback.inTraceback {
		t.Error("parser should be in traceback mode")
	}

	// Reset
	p.Reset()

	// Should no longer be in traceback mode
	if p.traceback.inTraceback {
		t.Error("parser should not be in traceback mode after reset")
	}
	if p.traceback.file != "" {
		t.Error("traceback state should be cleared after reset")
	}
}

func TestParser_NoisePatterns(t *testing.T) {
	p := NewParser()
	patterns := p.NoisePatterns()

	if len(patterns.FastPrefixes) == 0 {
		t.Error("expected non-empty FastPrefixes")
	}
	if len(patterns.FastContains) == 0 {
		t.Error("expected non-empty FastContains")
	}
	if len(patterns.Regex) == 0 {
		t.Error("expected non-empty Regex patterns")
	}
}

func TestParser_WorkflowContext(t *testing.T) {
	p := NewParser()
	wc := &errors.WorkflowContext{
		Job:  "test-job",
		Step: "Run pytest",
	}
	ctx := &parser.ParseContext{
		WorkflowContext: wc,
	}

	line := "FAILED tests/test_foo.py::test_bar - AssertionError"
	err := p.Parse(line, ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.WorkflowContext == nil {
		t.Fatal("WorkflowContext should be set")
	}
	if err.WorkflowContext.Job != "test-job" {
		t.Errorf("WorkflowContext.Job = %q, want %q", err.WorkflowContext.Job, "test-job")
	}
}

func TestParser_LongMessage(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Create a very long error message
	longMsg := strings.Repeat("x", 3000)
	line := "FAILED tests/test.py::test_foo - " + longMsg

	err := p.Parse(line, ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Message should be truncated
	if len(err.Message) > maxMessageLength+50 { // Allow some overhead for prefix
		t.Errorf("Message length = %d, should be truncated to ~%d", len(err.Message), maxMessageLength)
	}
}

func TestParser_UnicodeSupport(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Test with unicode in file path and message
	line := `FAILED tests/测试_foo.py::test_unicode - AssertionError: 期望值不匹配`

	err := p.Parse(line, ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.File != "tests/测试_foo.py" {
		t.Errorf("File = %q, want %q", err.File, "tests/测试_foo.py")
	}
	if !strings.Contains(err.Message, "期望值不匹配") {
		t.Errorf("Message should contain unicode content, got %q", err.Message)
	}
}

func TestParser_ANSIStripping(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Line with ANSI color codes
	line := "\x1b[31mFAILED\x1b[0m tests/test_foo.py::test_bar - \x1b[1mAssertionError\x1b[0m"

	err := p.Parse(line, ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.File != "tests/test_foo.py" {
		t.Errorf("File = %q, want %q", err.File, "tests/test_foo.py")
	}
}

func TestGetRuffFlake8Severity(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"E501", "warning"},
		{"E999", "error"},
		{"F401", "error"},
		{"F821", "error"},
		{"W503", "warning"},
		{"C901", "warning"},
		{"N801", "warning"},
		{"XXX", "error"}, // Unknown defaults to error
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := GetRuffFlake8Severity(tt.code)
			if got != tt.expected {
				t.Errorf("GetRuffFlake8Severity(%q) = %q, want %q", tt.code, got, tt.expected)
			}
		})
	}
}

func TestGetPylintSeverity(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"C0114", "warning"},
		{"R0913", "warning"},
		{"W0612", "warning"},
		{"E1101", "error"},
		{"F0001", "error"},
		{"X0000", "error"}, // Unknown defaults to error
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := GetPylintSeverity(tt.code)
			if got != tt.expected {
				t.Errorf("GetPylintSeverity(%q) = %q, want %q", tt.code, got, tt.expected)
			}
		})
	}
}

func TestParser_StandaloneException(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	tests := []struct {
		name    string
		line    string
		wantMsg string
	}{
		{
			name:    "ValueError",
			line:    "ValueError: invalid literal for int()",
			wantMsg: "ValueError: invalid literal for int()",
		},
		{
			name:    "TypeError",
			line:    "TypeError: 'NoneType' object is not iterable",
			wantMsg: "TypeError: 'NoneType' object is not iterable",
		},
		{
			name:    "ModuleNotFoundError",
			line:    "ModuleNotFoundError: No module named 'foo'",
			wantMsg: "ModuleNotFoundError: No module named 'foo'",
		},
		{
			name:    "AttributeError",
			line:    "AttributeError: 'list' object has no attribute 'foo'",
			wantMsg: "AttributeError: 'list' object has no attribute 'foo'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.Reset()
			err := p.Parse(tt.line, ctx)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if err.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", err.Message, tt.wantMsg)
			}
			if err.Category != errors.CategoryRuntime {
				t.Errorf("Category = %q, want %q", err.Category, errors.CategoryRuntime)
			}
		})
	}
}

func TestParser_InterfaceCompliance(t *testing.T) {
	// Verify that Parser implements ToolParser interface
	var _ parser.ToolParser = (*Parser)(nil)
	var _ parser.NoisePatternProvider = (*Parser)(nil)
}
