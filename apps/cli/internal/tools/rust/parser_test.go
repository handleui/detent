package rust

import (
	"strings"
	"testing"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

func TestParser_ID(t *testing.T) {
	p := NewParser()
	if p.ID() != "rust" {
		t.Errorf("ID() = %q, want %q", p.ID(), "rust")
	}
}

func TestParser_Priority(t *testing.T) {
	p := NewParser()
	if p.Priority() != 85 {
		t.Errorf("Priority() = %d, want %d", p.Priority(), 85)
	}
}

func TestParser_SupportsMultiLine(t *testing.T) {
	p := NewParser()
	if !p.SupportsMultiLine() {
		t.Error("SupportsMultiLine() = false, want true")
	}
}

func TestParser_CanParse(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantHigh bool // true if confidence should be >= 0.85
	}{
		{
			name:     "error with code",
			line:     "error[E0308]: mismatched types",
			wantHigh: true,
		},
		{
			name:     "warning with code",
			line:     "warning[W0501]: unused variable",
			wantHigh: true,
		},
		{
			name:     "error without code",
			line:     "error: cannot find type `Foo` in this scope",
			wantHigh: true,
		},
		{
			name:     "warning without code",
			line:     "warning: unused variable `x`",
			wantHigh: true,
		},
		{
			name:     "location arrow",
			line:     "  --> src/main.rs:4:7",
			wantHigh: true,
		},
		{
			name:     "test failure",
			line:     "test tests::test_foo ... FAILED",
			wantHigh: true,
		},
		{
			name:     "random line",
			line:     "Hello world",
			wantHigh: false,
		},
		{
			name:     "cargo compiling",
			line:     "   Compiling myproject v0.1.0",
			wantHigh: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			score := p.CanParse(tt.line, nil)
			if tt.wantHigh && score < 0.85 {
				t.Errorf("CanParse(%q) = %v, want >= 0.85", tt.line, score)
			}
			if !tt.wantHigh && score >= 0.85 {
				t.Errorf("CanParse(%q) = %v, want < 0.85", tt.line, score)
			}
		})
	}
}

func TestParser_ParseErrorWithCode(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	lines := []string{
		"error[E0308]: mismatched types",
		"  --> src/main.rs:4:5",
		"   |",
		" 4 |     let x: i32 = \"hello\";",
		"   |                  ^^^^^^^ expected `i32`, found `&str`",
		"",
	}

	// Parse header
	err := p.Parse(lines[0], ctx)
	if err != nil {
		t.Fatalf("Parse header returned error prematurely: %+v", err)
	}

	// Parse location
	err = p.Parse(lines[1], ctx)
	if err != nil {
		t.Fatalf("Parse location returned error prematurely: %+v", err)
	}

	// Continue with remaining lines
	for i := 2; i < len(lines)-1; i++ {
		if !p.ContinueMultiLine(lines[i], ctx) {
			break
		}
	}

	// Empty line should end the error
	if p.ContinueMultiLine(lines[len(lines)-1], ctx) {
		t.Error("Expected empty line to end multi-line error")
	}

	// Finalize
	result := p.FinishMultiLine(ctx)
	if result == nil {
		t.Fatal("FinishMultiLine returned nil")
	}

	// Verify extracted error
	if result.Message != "mismatched types" {
		t.Errorf("Message = %q, want %q", result.Message, "mismatched types")
	}
	if result.File != "src/main.rs" {
		t.Errorf("File = %q, want %q", result.File, "src/main.rs")
	}
	if result.Line != 4 {
		t.Errorf("Line = %d, want %d", result.Line, 4)
	}
	if result.Column != 5 {
		t.Errorf("Column = %d, want %d", result.Column, 5)
	}
	if result.RuleID != "E0308" {
		t.Errorf("RuleID = %q, want %q", result.RuleID, "E0308")
	}
	if result.Severity != "error" {
		t.Errorf("Severity = %q, want %q", result.Severity, "error")
	}
	if result.Category != errors.CategoryCompile {
		t.Errorf("Category = %q, want %q", result.Category, errors.CategoryCompile)
	}
	if result.Source != errors.SourceRust {
		t.Errorf("Source = %q, want %q", result.Source, errors.SourceRust)
	}
}

func TestParser_ParseClippyWarning(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	lines := []string{
		"warning: redundant clone",
		"  --> src/lib.rs:10:5",
		"   |",
		"10 |     let s = string.clone();",
		"   |             ^^^^^^^^^^^^^^ help: remove this",
		"   |",
		"   = note: `#[warn(clippy::redundant_clone)]` on by default",
		"",
	}

	// Parse header
	p.Parse(lines[0], ctx)

	// Parse location
	p.Parse(lines[1], ctx)

	// Continue with remaining lines
	for i := 2; i < len(lines); i++ {
		if !p.ContinueMultiLine(lines[i], ctx) {
			// Parse note line if continuation ended
			if i < len(lines)-1 {
				p.Parse(lines[i], ctx)
			}
		}
	}

	// Finalize
	result := p.FinishMultiLine(ctx)
	if result == nil {
		t.Fatal("FinishMultiLine returned nil")
	}

	// Verify extracted error
	if result.Message != "redundant clone" {
		t.Errorf("Message = %q, want %q", result.Message, "redundant clone")
	}
	if result.File != "src/lib.rs" {
		t.Errorf("File = %q, want %q", result.File, "src/lib.rs")
	}
	if result.Line != 10 {
		t.Errorf("Line = %d, want %d", result.Line, 10)
	}
	if !strings.Contains(result.RuleID, "clippy::redundant_clone") {
		t.Errorf("RuleID = %q, should contain %q", result.RuleID, "clippy::redundant_clone")
	}
	if result.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", result.Severity, "warning")
	}
	if result.Category != errors.CategoryLint {
		t.Errorf("Category = %q, want %q", result.Category, errors.CategoryLint)
	}
}

func TestParser_CriticalClippyLintElevation(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	lines := []string{
		"warning: used `unwrap()` on a `Result` value",
		"  --> src/main.rs:15:5",
		"   |",
		"15 |     foo.unwrap();",
		"   |     ^^^^^^^^^^^^",
		"   |",
		"   = note: `#[warn(clippy::unwrap_used)]` on by default",
		"",
	}

	// Parse header
	p.Parse(lines[0], ctx)
	p.Parse(lines[1], ctx)

	// Continue with remaining lines
	for i := 2; i < len(lines); i++ {
		if !p.ContinueMultiLine(lines[i], ctx) {
			if i < len(lines)-1 {
				p.Parse(lines[i], ctx)
			}
		}
	}

	result := p.FinishMultiLine(ctx)
	if result == nil {
		t.Fatal("FinishMultiLine returned nil")
	}

	// Critical Clippy lint should be elevated to error
	if result.Severity != "error" {
		t.Errorf("Severity = %q, want %q (unwrap_used should be elevated)", result.Severity, "error")
	}
}

func TestParser_TestFailure(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	line := "test tests::test_addition ... FAILED"
	result := p.Parse(line, ctx)

	if result == nil {
		t.Fatal("Parse returned nil for test failure")
	}

	if result.Message != "test failed: tests::test_addition" {
		t.Errorf("Message = %q, want %q", result.Message, "test failed: tests::test_addition")
	}
	if result.Severity != "error" {
		t.Errorf("Severity = %q, want %q", result.Severity, "error")
	}
	if result.Category != errors.CategoryTest {
		t.Errorf("Category = %q, want %q", result.Category, errors.CategoryTest)
	}
	if result.Source != errors.SourceRust {
		t.Errorf("Source = %q, want %q", result.Source, errors.SourceRust)
	}
}

func TestParser_IsNoise(t *testing.T) {
	p := NewParser()

	noiseLines := []string{
		"   Compiling myproject v0.1.0 (/path/to/project)",
		"   Downloading crate_name v1.2.3",
		"   Downloaded crate_name v1.2.3",
		"    Finished dev [unoptimized + debuginfo] target(s) in 10.50s",
		"     Running `target/debug/myproject`",
		"running 5 tests",
		"test tests::test_foo ... ok",
		"test result: ok. 5 passed; 0 failed; 0 ignored",
		"For more information about this error",
		"aborting due to previous error",
		"error: could not compile `myproject`",
	}

	for _, line := range noiseLines {
		if !p.IsNoise(line) {
			t.Errorf("IsNoise(%q) = false, want true", line)
		}
	}

	// Non-noise lines
	validLines := []string{
		"error[E0308]: mismatched types",
		"  --> src/main.rs:4:5",
		"warning: unused variable",
	}

	for _, line := range validLines {
		if p.IsNoise(line) {
			t.Errorf("IsNoise(%q) = true, want false", line)
		}
	}
}

func TestParser_Reset(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Start parsing an error
	p.Parse("error[E0308]: mismatched types", ctx)
	p.Parse("  --> src/main.rs:4:5", ctx)

	// Reset
	p.Reset()

	// Verify state is cleared
	if p.inError {
		t.Error("inError should be false after Reset")
	}
	if p.errorFile != "" {
		t.Errorf("errorFile should be empty after Reset, got %q", p.errorFile)
	}

	// FinishMultiLine should return nil after reset
	if result := p.FinishMultiLine(ctx); result != nil {
		t.Error("FinishMultiLine should return nil after Reset")
	}
}

func TestParser_MultipleErrors(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// First error
	lines1 := []string{
		"error[E0308]: mismatched types",
		"  --> src/main.rs:4:5",
		"   |",
		" 4 |     let x: i32 = \"hello\";",
		"   |                  ^^^^^^^ expected `i32`, found `&str`",
	}

	for _, line := range lines1 {
		p.Parse(line, ctx)
	}

	// Second error should finalize the first and start accumulating
	result1 := p.Parse("error[E0425]: cannot find value `foo` in this scope", ctx)

	if result1 == nil {
		t.Fatal("Second error header should finalize first error")
	}

	if result1.Message != "mismatched types" {
		t.Errorf("First error message = %q, want %q", result1.Message, "mismatched types")
	}
	if result1.RuleID != "E0308" {
		t.Errorf("First error RuleID = %q, want %q", result1.RuleID, "E0308")
	}

	// Parse location for second error
	p.Parse("  --> src/main.rs:10:5", ctx)

	// Finalize second error
	result2 := p.FinishMultiLine(ctx)
	if result2 == nil {
		t.Fatal("FinishMultiLine returned nil for second error")
	}

	if result2.Message != "cannot find value `foo` in this scope" {
		t.Errorf("Second error message = %q, want %q", result2.Message, "cannot find value `foo` in this scope")
	}
	if result2.RuleID != "E0425" {
		t.Errorf("Second error RuleID = %q, want %q", result2.RuleID, "E0425")
	}
}

func TestParser_WarningWithoutCode(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	lines := []string{
		"warning: unused variable: `x`",
		"  --> src/main.rs:3:9",
		"   |",
		" 3 |     let x = 5;",
		"   |         ^ help: if this is intentional, prefix with an underscore: `_x`",
		"",
	}

	p.Parse(lines[0], ctx)
	p.Parse(lines[1], ctx)

	for i := 2; i < len(lines)-1; i++ {
		p.ContinueMultiLine(lines[i], ctx)
	}
	p.ContinueMultiLine(lines[len(lines)-1], ctx) // Empty line ends

	result := p.FinishMultiLine(ctx)
	if result == nil {
		t.Fatal("FinishMultiLine returned nil")
	}

	if result.Message != "unused variable: `x`" {
		t.Errorf("Message = %q, want %q", result.Message, "unused variable: `x`")
	}
	if result.RuleID != "" {
		t.Errorf("RuleID = %q, want empty (no error code)", result.RuleID)
	}
	if result.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", result.Severity, "warning")
	}
}

// Verify interface compliance
var _ parser.ToolParser = (*Parser)(nil)
