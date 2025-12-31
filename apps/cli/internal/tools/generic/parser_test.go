package generic

import (
	"strings"
	"testing"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

func TestParser_ID(t *testing.T) {
	p := NewParser()
	if got := p.ID(); got != "generic" {
		t.Errorf("ID() = %q, want %q", got, "generic")
	}
}

func TestParser_Priority(t *testing.T) {
	p := NewParser()
	if got := p.Priority(); got != 10 {
		t.Errorf("Priority() = %d, want %d", got, 10)
	}
}

func TestParser_CanParse(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	tests := []struct {
		name      string
		line      string
		wantMatch bool // true if score > 0 (should be flagged)
	}{
		// === ACTUAL ERROR PATTERNS (should match with low confidence) ===
		{"error prefix", "Error: something went wrong with the build", true},
		{"fatal prefix", "Fatal: unable to connect to database", true},
		{"permission denied", "permission denied accessing /etc/passwd", true},
		{"command not found", "command not found: npm", true},
		{"no such file", "no such file or directory", true},
		{"exit code", "exit code 1", true},
		{"segfault", "segmentation fault", true},
		{"killed", "Killed", false}, // Too short (< 10 chars) to be flagged
		{"out of memory", "out of memory", true},

		// === FALSE POSITIVES - MUST RETURN 0 ===
		// Code patterns
		{"comment hash", "# this is an error handler comment", false},
		{"comment slash", "// handle error case here", false},
		{"error handler code", "setupErrorHandler()", false},
		{"if error", "if err != nil { return error }", false},
		{"return error", "return fmt.Errorf('error: %v', err)", false},
		{"catch error", "} catch (error) {", false},
		{"throw error", "throw new Error('message')", false},
		{"error type", "type MyError struct {}", false},

		// Success indicators
		{"success checkmark", "test passed OK", false},
		{"success emoji", "Build completed ✓", false},
		{"zero errors", "0 errors found", false},
		{"no errors", "no errors found", false},
		{"build succeeded", "build succeeded in 5s", false},
		{"all tests passed", "all tests passed", false},
		{"completed successfully", "completed successfully", false},
		{"process success", "Process completed with exit code 0", false},

		// Progress/download messages
		{"downloading", "Downloading https://example.com/file.tar.gz", false},
		{"installing", "Installing dependencies...", false},
		{"fetching", "Fetching latest changes", false},
		{"pulling", "Pulling docker image", false},
		{"using cached", "Using cached layer abc123", false},
		{"cache hit", "Cache hit for key: deps-v1", false},

		// Retry/recovery messages
		{"retrying", "Retrying connection attempt 2/3", false},
		{"will retry", "Connection failed, will retry in 5s", false},
		{"attempt", "Attempt 2 of 3", false},

		// GitHub Actions workflow commands
		{"debug command", "::debug::Setting up environment", false},
		{"warning command", "::warning file=app.js,line=1::Missing semicolon", false},
		{"error command", "::error::Build failed", false},
		{"github annotation", "##[error]Build failed", false},

		// Test framework output
		{"go test run", "=== RUN   TestParser_CanParse", false},
		{"go test pass", "--- PASS: TestParser_CanParse (0.00s)", false},
		{"go test skip", "--- SKIP: TestParser_Skip", false},
		{"go test summary", "PASS github.com/test/pkg 1.234s", false},
		{"go no tests", "?   github.com/test/pkg [no test files]", false},
		{"mocha passing", "5 passing", false},

		// Coverage tools
		{"coverage", "coverage: 85.5% of statements", false},
		{"codecov", "Uploading to Codecov", false},

		// Docker output
		{"docker step", "Step 1/10: FROM golang:1.21", false},
		{"docker buildx", "#5 [2/4] RUN go build", false},
		{"docker digest", "sha256:abc123def456", false},

		// Stack traces (belong to parent error)
		{"js stack trace", "    at processTicksAndRejections (node:internal/process/task_queues:95:5)", false},
		{"python traceback", `  File "/path/to/file.py", line 10`, false},
		{"go goroutine", "goroutine 1 [running]:", false},
		{"heavy indent", "        this is heavily indented", false},

		// Version/info output
		{"version info", "version 1.2.3", false},
		{"node version", "node v18.17.0", false},
		{"using node", "Using node 18.17.0", false},

		// CI platform noise
		{"running step", "Running setup-node", false},
		{"starting job", "Starting job 'build'", false},
		{"finished step", "Finished step 'test'", false},

		// Linter/tool status
		{"running linter", "Running golangci-lint", false},
		{"issues count", "Issues: 0", false},
		{"found issues", "Found 3 issues", false},
		{"level debug", "level=debug msg=something", false},

		// Empty/decorative
		{"empty", "", false},
		{"whitespace", "   ", false},
		{"horizontal rule", "-------------------", false},
		{"box chars", "├──────────────────┤", false},

		// URL/path patterns
		{"url with error", "https://example.com/errors/404", false},
		{"path with error", "/app/lib/errors/handler.go", false},
		{"error module", "import error.ts", false},

		// Short/long lines
		{"too short", "err", false},
		{"short line", "hi there", false},
		{"very long line", strings.Repeat("x", 600), false}, // > 500 chars

		// Generic error keywords without structure (intentionally NOT matched)
		{"vague error", "some error occurred here", false},
		{"vague failed", "the operation failed somewhere", false},
		{"error in context", "there was an error during processing", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := p.CanParse(tt.line, ctx)

			if tt.wantMatch && score == 0 {
				t.Errorf("CanParse(%q) = 0, want > 0 (should be flagged)", tt.line)
			}
			if !tt.wantMatch && score != 0 {
				t.Errorf("CanParse(%q) = %f, want 0 (should NOT be flagged)", tt.line, score)
			}
		})
	}
}

func TestParser_Parse(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{
		WorkflowContext: &errors.WorkflowContext{
			Job:  "test-job",
			Step: "test-step",
		},
	}

	t.Run("parses error line", func(t *testing.T) {
		line := "Error: something went wrong"
		err := p.Parse(line, ctx)

		if err == nil {
			t.Fatal("Parse() returned nil, want error")
		}
		if err.Message != "Error: something went wrong" {
			t.Errorf("Message = %q, want %q", err.Message, "Error: something went wrong")
		}
		if err.Severity != "error" {
			t.Errorf("Severity = %q, want %q", err.Severity, "error")
		}
		if err.Category != errors.CategoryUnknown {
			t.Errorf("Category = %q, want %q", err.Category, errors.CategoryUnknown)
		}
		if err.Source != errors.SourceGeneric {
			t.Errorf("Source = %q, want %q", err.Source, errors.SourceGeneric)
		}
		if !err.UnknownPattern {
			t.Error("UnknownPattern = false, want true")
		}
		if err.Raw != line {
			t.Errorf("Raw = %q, want %q", err.Raw, line)
		}
		if err.WorkflowContext == nil {
			t.Error("WorkflowContext = nil, want non-nil")
		} else if err.WorkflowContext.Job != "test-job" {
			t.Errorf("WorkflowContext.Job = %q, want %q", err.WorkflowContext.Job, "test-job")
		}
	})

	t.Run("returns nil for short lines", func(t *testing.T) {
		err := p.Parse("hi", ctx)
		if err != nil {
			t.Errorf("Parse() = %v, want nil for short line", err)
		}
	})

	t.Run("handles nil context", func(t *testing.T) {
		err := p.Parse("Error: test", nil)
		if err == nil {
			t.Fatal("Parse() = nil, want error")
		}
		if err.WorkflowContext != nil {
			t.Error("WorkflowContext should be nil when context is nil")
		}
	})
}

func TestParser_IsNoise(t *testing.T) {
	p := NewParser()

	// Lines that should be marked as noise
	noiseLines := []string{
		"# this is a comment",
		"// another comment",
		"Downloading packages...",
		"Installing dependencies",
		"Using cached layer",
		"Retrying connection",
		"::debug::message",
		"=== RUN   TestSomething",
		"--- PASS: TestSomething",
		"coverage: 85.5%",
		"Step 1/10: FROM golang",
		"    at someFunction (file.js:10:5)",
		"goroutine 1 [running]:",
		"version 1.2.3",
		"Running tests...",
		"",
		"-------------------",
	}

	for _, line := range noiseLines {
		if !p.IsNoise(line) {
			t.Errorf("IsNoise(%q) = false, want true", line)
		}
	}

	// Lines that should NOT be noise (actual errors)
	notNoiseLines := []string{
		"Error: something went wrong",
		"Fatal: database connection failed",
		"permission denied accessing file",
	}

	for _, line := range notNoiseLines {
		if p.IsNoise(line) {
			t.Errorf("IsNoise(%q) = true, want false", line)
		}
	}
}

func TestParser_MultiLine(t *testing.T) {
	p := NewParser()

	if p.SupportsMultiLine() {
		t.Error("SupportsMultiLine() = true, want false")
	}

	if p.ContinueMultiLine("any", nil) {
		t.Error("ContinueMultiLine() = true, want false")
	}

	if p.FinishMultiLine(nil) != nil {
		t.Error("FinishMultiLine() should return nil")
	}
}

func TestParser_Reset(t *testing.T) {
	p := NewParser()
	p.Reset() // Should not panic
}

func TestParser_ImplementsInterface(t *testing.T) {
	var _ parser.ToolParser = (*Parser)(nil)
}
