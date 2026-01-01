package extract

import (
	"strings"
	"testing"

	"github.com/handleui/detent/packages/core/ci"
	"github.com/handleui/detent/packages/core/errors"
	"github.com/handleui/detent/packages/core/tools"
	"github.com/handleui/detent/packages/core/tools/parser"
)

// mockContextParser is a simple context parser for testing
type mockContextParser struct {
	skipLine bool
	jobName  string
	stepName string
}

func (m *mockContextParser) ParseLine(line string) (*ci.LineContext, string, bool) {
	if m.skipLine {
		return nil, "", true
	}

	var ctx *ci.LineContext
	if m.jobName != "" || m.stepName != "" {
		ctx = &ci.LineContext{
			Job:  m.jobName,
			Step: m.stepName,
		}
	}

	return ctx, line, false
}

// mockToolParser is a simple parser for testing
type mockToolParser struct {
	id              string
	priority        int
	canParseScore   float64
	parseResult     *errors.ExtractedError
	supportsMulti   bool
	continueResult  bool
	finishResult    *errors.ExtractedError
	isNoise         bool
	parseFunc       func(line string, ctx *parser.ParseContext) *errors.ExtractedError
}

func (m *mockToolParser) ID() string {
	return m.id
}

func (m *mockToolParser) Priority() int {
	return m.priority
}

func (m *mockToolParser) CanParse(line string, ctx *parser.ParseContext) float64 {
	return m.canParseScore
}

func (m *mockToolParser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	if m.parseFunc != nil {
		return m.parseFunc(line, ctx)
	}
	return m.parseResult
}

func (m *mockToolParser) IsNoise(line string) bool {
	return m.isNoise
}

func (m *mockToolParser) SupportsMultiLine() bool {
	return m.supportsMulti
}

func (m *mockToolParser) ContinueMultiLine(line string, ctx *parser.ParseContext) bool {
	return m.continueResult
}

func (m *mockToolParser) FinishMultiLine(ctx *parser.ParseContext) *errors.ExtractedError {
	return m.finishResult
}

func (m *mockToolParser) Reset() {}

func TestNewExtractor(t *testing.T) {
	registry := tools.NewRegistry()
	ext := NewExtractor(registry)

	if ext == nil {
		t.Fatal("NewExtractor() returned nil")
	}

	if ext.registry != registry {
		t.Error("NewExtractor() did not set registry correctly")
	}

	if ext.currentWorkflowCtx != nil {
		t.Error("NewExtractor() should initialize with nil workflow context")
	}
}

func TestExtractor_Extract_EmptyOutput(t *testing.T) {
	registry := tools.NewRegistry()
	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	result := ext.Extract("", ctxParser)

	if len(result) != 0 {
		t.Errorf("Extract() on empty output returned %d errors, want 0", len(result))
	}
}

func TestExtractor_Extract_SkipLongLines(t *testing.T) {
	registry := tools.NewRegistry()
	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	// Create a line longer than maxLineLength
	longLine := strings.Repeat("x", maxLineLength+100)

	result := ext.Extract(longLine, ctxParser)

	if len(result) != 0 {
		t.Errorf("Extract() on extremely long line returned %d errors, want 0", len(result))
	}
}

func TestExtractor_Extract_SkipLineFromParser(t *testing.T) {
	registry := tools.NewRegistry()
	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{skipLine: true}

	result := ext.Extract("some error line", ctxParser)

	if len(result) != 0 {
		t.Errorf("Extract() with skip=true returned %d errors, want 0", len(result))
	}
}

func TestExtractor_Extract_SingleError(t *testing.T) {
	registry := tools.NewRegistry()
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseResult: &errors.ExtractedError{
			Message: "test error",
			File:    "test.go",
			Line:    10,
		},
	}
	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	result := ext.Extract("test.go:10: test error", ctxParser)

	if len(result) != 1 {
		t.Fatalf("Extract() returned %d errors, want 1", len(result))
	}

	if result[0].Message != "test error" {
		t.Errorf("Extract() error message = %q, want %q", result[0].Message, "test error")
	}
	if result[0].File != "test.go" {
		t.Errorf("Extract() error file = %q, want %q", result[0].File, "test.go")
	}
	if result[0].Line != 10 {
		t.Errorf("Extract() error line = %d, want 10", result[0].Line)
	}
}

func TestExtractor_Extract_Deduplication(t *testing.T) {
	registry := tools.NewRegistry()
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseResult: &errors.ExtractedError{
			Message: "duplicate error",
			File:    "test.go",
			Line:    10,
		},
	}
	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	// Same error appears twice
	output := "test.go:10: duplicate error\ntest.go:10: duplicate error"
	result := ext.Extract(output, ctxParser)

	if len(result) != 1 {
		t.Errorf("Extract() with duplicate errors returned %d errors, want 1", len(result))
	}
}

func TestExtractor_Extract_DifferentErrors(t *testing.T) {
	registry := tools.NewRegistry()

	result1 := &errors.ExtractedError{
		Message: "error 1",
		File:    "test.go",
		Line:    10,
	}
	result2 := &errors.ExtractedError{
		Message: "error 2",
		File:    "test.go",
		Line:    20,
	}

	callCount := 0
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseFunc: func(line string, ctx *parser.ParseContext) *errors.ExtractedError {
			callCount++
			if strings.Contains(line, "error 1") {
				return result1
			}
			if strings.Contains(line, "error 2") {
				return result2
			}
			return nil
		},
	}

	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	output := "test.go:10: error 1\ntest.go:20: error 2"
	result := ext.Extract(output, ctxParser)

	if len(result) != 2 {
		t.Errorf("Extract() returned %d errors, want 2", len(result))
	}
}

func TestExtractor_Extract_WorkflowContext(t *testing.T) {
	registry := tools.NewRegistry()
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseResult: &errors.ExtractedError{
			Message: "test error",
		},
	}
	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{
		jobName:  "test-job",
		stepName: "test-step",
	}

	result := ext.Extract("error line", ctxParser)

	if len(result) != 1 {
		t.Fatalf("Extract() returned %d errors, want 1", len(result))
	}

	if result[0].WorkflowContext == nil {
		t.Fatal("Extract() did not set workflow context")
	}

	if result[0].WorkflowContext.Job != "test-job" {
		t.Errorf("WorkflowContext.Job = %q, want %q", result[0].WorkflowContext.Job, "test-job")
	}
}

func TestExtractor_Extract_NoiseFiltering(t *testing.T) {
	registry := tools.NewRegistry()

	// Create a parser that returns an error but should be filtered as noise
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseResult: &errors.ExtractedError{
			Message: "noise",
		},
	}
	registry.Register(mockParser)

	// Create mock noise patterns
	noiseParser := &mockToolParser{
		id:       "noise",
		priority: 1,
		isNoise:  true,
	}
	registry.Register(noiseParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	// The registry.IsNoise would need to be mocked or the noise checker implemented
	// For now, this tests the basic flow
	result := ext.Extract("some line", ctxParser)

	// Result will depend on noise checker implementation
	if result == nil {
		t.Error("Extract() returned nil, want non-nil slice")
	}
}

func TestExtractor_Extract_MaxDeduplicationSize(t *testing.T) {
	registry := tools.NewRegistry()

	callCount := 0
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseFunc: func(line string, ctx *parser.ParseContext) *errors.ExtractedError {
			callCount++
			return &errors.ExtractedError{
				Message: "error",
				File:    "test.go",
				Line:    callCount,
			}
		},
	}

	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	// Create more than maxDeduplicationSize unique errors
	lines := make([]string, maxDeduplicationSize+100)
	for i := range lines {
		lines[i] = "error line"
	}
	output := strings.Join(lines, "\n")

	result := ext.Extract(output, ctxParser)

	// All errors should be returned even beyond maxDeduplicationSize
	if len(result) != maxDeduplicationSize+100 {
		t.Errorf("Extract() returned %d errors, want %d", len(result), maxDeduplicationSize+100)
	}
}

func TestExtractor_Extract_MultiLine(t *testing.T) {
	registry := tools.NewRegistry()

	mockParser := &mockToolParser{
		id:             "test",
		priority:       50,
		canParseScore:  0.9,
		supportsMulti:  true,
		continueResult: true,
		finishResult: &errors.ExtractedError{
			Message:    "multi-line error",
			StackTrace: "line1\nline2\nline3",
		},
	}
	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	// First line starts multi-line, next 2 continue, then a different line finishes
	output := "panic: something\nat line1\nat line2\nnormal line"
	result := ext.Extract(output, ctxParser)

	// Should have the finished multi-line error
	if len(result) < 1 {
		t.Fatalf("Extract() returned %d errors, want at least 1", len(result))
	}
}

func TestExtractor_Extract_MultiLineAtEnd(t *testing.T) {
	registry := tools.NewRegistry()

	mockParser := &mockToolParser{
		id:             "test",
		priority:       50,
		canParseScore:  0.9,
		supportsMulti:  true,
		continueResult: true,
		finishResult: &errors.ExtractedError{
			Message: "multi-line error at end",
		},
	}
	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	// Multi-line that continues to the end of output
	output := "start\ncontinue\ncontinue"
	result := ext.Extract(output, ctxParser)

	// Should finalize the multi-line error at the end
	if len(result) < 1 {
		t.Fatalf("Extract() returned %d errors, want at least 1", len(result))
	}
}

func TestExtractor_Reset(t *testing.T) {
	registry := tools.NewRegistry()
	ext := NewExtractor(registry)

	// Set workflow context
	ext.currentWorkflowCtx = &errors.WorkflowContext{
		Job:  "test-job",
		Step: "test-step",
	}

	ext.Reset()

	if ext.currentWorkflowCtx != nil {
		t.Error("Reset() did not clear workflow context")
	}
}

func TestSanitizePatternForSentry_Length(t *testing.T) {
	longPattern := strings.Repeat("x", maxUnknownPatternLineLength+100)

	result := SanitizePatternForTelemetry(longPattern)

	if len(result) > maxUnknownPatternLineLength+10 {
		t.Errorf("SanitizePatternForTelemetry() result length = %d, want <= %d", len(result), maxUnknownPatternLineLength+10)
	}

	if !strings.HasSuffix(result, "...") {
		t.Error("SanitizePatternForTelemetry() should add '...' to truncated pattern")
	}
}

func TestSanitizePatternForSentry_APIKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFree bool // true if the secret should be redacted
	}{
		{
			name:     "api key",
			input:    "api_key=sk_live_abcd1234efgh5678",
			wantFree: true,
		},
		{
			name:     "github token",
			input:    "token: ghp_abcdefghijklmnopqrstuvwxyz123456",
			wantFree: true,
		},
		{
			name:     "jwt token",
			input:    "Authorization: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			wantFree: true,
		},
		{
			name:     "email address",
			input:    "user@example.com failed",
			wantFree: true,
		},
		{
			name:     "home directory unix",
			input:    "/home/username/project/file.go:10",
			wantFree: true,
		},
		{
			name:     "home directory mac",
			input:    "/Users/username/project/file.go:10",
			wantFree: true,
		},
		{
			name:     "safe error message",
			input:    "error: undefined variable foo",
			wantFree: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizePatternForTelemetry(tt.input)

			if tt.wantFree {
				// Result should not contain the original sensitive data
				if result == tt.input {
					t.Errorf("SanitizePatternForTelemetry() did not redact sensitive data in %q", tt.input)
				}
				// Should contain a redaction marker
				if !strings.Contains(result, "REDACTED") && !strings.Contains(result, "[") {
					t.Errorf("SanitizePatternForTelemetry() = %q, should contain redaction marker", result)
				}
			}
		})
	}
}

func TestSanitizePatternForSentry_FilePaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "go file path",
			input: "internal/extract/extractor.go:10:5: error message",
			want:  "[path].go:10:5: error message",
		},
		{
			name:  "typescript file path",
			input: "src/components/Button.tsx:20:10: type error",
			want:  "[path].tsx:20:10: type error",
		},
		{
			name:  "multiple file paths",
			input: "file1.go:10 and file2.ts:20",
			want:  "[path].go:10 and [path].ts:20",
		},
		{
			name:  "absolute path",
			input: "/absolute/path/to/file.go:10: error",
			want:  "[path].go:10: error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizePatternForTelemetry(tt.input)

			if result != tt.want {
				t.Errorf("SanitizePatternForTelemetry() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestSanitizePatternForSentry_SpecificTokens(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		shouldMatch    string
		shouldNotMatch string
	}{
		{
			name:           "github pat",
			input:          "github_pat_11AAAAAA0aBcDeFgHiJkLmNoPq",
			shouldMatch:    "[GITHUB_PAT]",
			shouldNotMatch: "github_pat_11AAAAAA0aBcDeFgHiJkLmNoPq",
		},
		{
			name:           "github oauth - long enough",
			input:          "gho_abcdefghijklmnopqrstuvwxyz1234567890",
			shouldMatch:    "[GITHUB_OAUTH_TOKEN]",
			shouldNotMatch: "gho_abcdefghijklmnopqrstuvwxyz1234567890",
		},
		{
			name:           "gitlab pat",
			input:          "glpat-abcdefghijklmnopqrst",
			shouldMatch:    "[GITLAB_PAT]",
			shouldNotMatch: "glpat-abcdefghijklmnopqrst",
		},
		{
			name:           "aws access key",
			input:          "AKIAIOSFODNN7EXAMPLE",
			shouldMatch:    "[AWS_ACCESS_KEY]",
			shouldNotMatch: "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:           "npm token - long enough",
			input:          "npm_abcdefghijklmnopqrstuvwxyz1234567890",
			shouldMatch:    "[NPM_TOKEN]",
			shouldNotMatch: "npm_abcdefghijklmnopqrstuvwxyz1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizePatternForTelemetry(tt.input)

			if !strings.Contains(result, tt.shouldMatch) {
				t.Errorf("SanitizePatternForTelemetry() = %q, should contain %q", result, tt.shouldMatch)
			}

			if strings.Contains(result, tt.shouldNotMatch) {
				t.Errorf("SanitizePatternForTelemetry() = %q, should not contain original token", result)
			}
		})
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   int
	}{
		{
			name:   "found at start",
			s:      "hello world",
			substr: "hello",
			want:   0,
		},
		{
			name:   "found at end",
			s:      "hello world",
			substr: "world",
			want:   6,
		},
		{
			name:   "found in middle",
			s:      "hello world",
			substr: "o w",
			want:   4,
		},
		{
			name:   "not found",
			s:      "hello world",
			substr: "foo",
			want:   -1,
		},
		{
			name:   "empty substr",
			s:      "hello",
			substr: "",
			want:   0,
		},
		{
			name:   "empty string",
			s:      "",
			substr: "hello",
			want:   -1,
		},
		{
			name:   "both empty",
			s:      "",
			substr: "",
			want:   0,
		},
		{
			name:   "substr longer than s",
			s:      "hi",
			substr: "hello",
			want:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexOf(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("indexOf(%q, %q) = %d, want %d", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestReportUnknownPatterns_Empty(t *testing.T) {
	// Should not panic with empty slice
	ReportUnknownPatterns([]*errors.ExtractedError{})
}

func TestReportUnknownPatterns_NoUnknown(t *testing.T) {
	errs := []*errors.ExtractedError{
		{Message: "error 1", UnknownPattern: false},
		{Message: "error 2", UnknownPattern: false},
	}

	// Should not panic
	ReportUnknownPatterns(errs)
}

func TestReportUnknownPatterns_WithUnknown(t *testing.T) {
	errs := []*errors.ExtractedError{
		{Message: "error 1", UnknownPattern: true, Raw: "raw error 1"},
		{Message: "error 2", UnknownPattern: false},
		{Message: "error 3", UnknownPattern: true, Raw: "raw error 3"},
	}

	// Should not panic and should handle sanitization
	ReportUnknownPatterns(errs)
}

func TestReportUnknownPatterns_TruncatesLongRaw(t *testing.T) {
	longRaw := strings.Repeat("x", maxUnknownPatternLineLength+100)
	errs := []*errors.ExtractedError{
		{Message: "error", UnknownPattern: true, Raw: longRaw},
	}

	// Should not panic
	ReportUnknownPatterns(errs)
}

func TestReportUnknownPatterns_LimitCount(t *testing.T) {
	// Create more unknown patterns than maxUnknownPatternsToReport
	errs := make([]*errors.ExtractedError, maxUnknownPatternsToReport+10)
	for i := range errs {
		errs[i] = &errors.ExtractedError{
			Message:        "error",
			UnknownPattern: true,
			Raw:            "raw error",
		}
	}

	// Should not panic and should limit reporting
	ReportUnknownPatterns(errs)
}

func TestSanitizePatternForSentry_ConnectionStrings(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "mongodb connection",
			input: "mongodb://user:pass@host:27017/db",
		},
		{
			name:  "postgres connection",
			input: "postgres://user:pass@localhost/db",
		},
		{
			name:  "mysql connection",
			input: "mysql://user:pass@localhost:3306/db",
		},
		{
			name:  "redis connection",
			input: "redis://user:pass@localhost:6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizePatternForTelemetry(tt.input)

			// Should not contain the password
			if strings.Contains(result, "user:pass") {
				t.Errorf("SanitizePatternForTelemetry() = %q, should redact credentials", result)
			}

			// Should contain CONNECTION_STRING marker
			if !strings.Contains(result, "[CONNECTION_STRING]") {
				t.Errorf("SanitizePatternForTelemetry() = %q, should contain [CONNECTION_STRING]", result)
			}
		})
	}
}

func TestSanitizePatternForSentry_URLWithCredentials(t *testing.T) {
	input := "https://username:password@example.com/path"
	result := SanitizePatternForTelemetry(input)

	// Should not contain the original credentials
	if strings.Contains(result, "username:password") {
		t.Errorf("SanitizePatternForTelemetry() = %q, should redact URL credentials", result)
	}

	// The pattern should replace credentials with [CREDENTIALS]
	// Note: email pattern might also match "password@example.com"
	// So we just verify the credentials aren't in plaintext
	if result == input {
		t.Errorf("SanitizePatternForTelemetry() did not modify input with credentials")
	}
}

func TestSanitizePatternForSentry_IPAddress(t *testing.T) {
	input := "connection failed to 192.168.1.100"
	result := SanitizePatternForTelemetry(input)

	if strings.Contains(result, "192.168.1.100") {
		t.Errorf("SanitizePatternForTelemetry() = %q, should redact IP address", result)
	}

	if !strings.Contains(result, "[IP_ADDR]") {
		t.Errorf("SanitizePatternForTelemetry() = %q, should contain [IP_ADDR]", result)
	}
}

func TestSanitizePatternForSentry_HexStrings(t *testing.T) {
	// Long hex strings (likely secrets) should be redacted
	input := "error: hash mismatch abcdef1234567890abcdef1234567890abcdef12"
	result := SanitizePatternForTelemetry(input)

	if strings.Contains(result, "abcdef1234567890abcdef1234567890abcdef12") {
		t.Errorf("SanitizePatternForTelemetry() = %q, should redact long hex strings", result)
	}
}

func TestSanitizePatternForSentry_Base64(t *testing.T) {
	// Long base64 strings should be redacted
	input := "token: YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODkwYWJjZGVm"
	result := SanitizePatternForTelemetry(input)

	if strings.Contains(result, "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODkwYWJjZGVm") {
		t.Errorf("SanitizePatternForTelemetry() = %q, should redact long base64 strings", result)
	}
}

func TestExtractor_Extract_WorkflowContextClone(t *testing.T) {
	registry := tools.NewRegistry()

	err1 := &errors.ExtractedError{Message: "error 1"}
	err2 := &errors.ExtractedError{Message: "error 2"}

	callCount := 0
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseFunc: func(line string, ctx *parser.ParseContext) *errors.ExtractedError {
			callCount++
			if callCount == 1 {
				return err1
			}
			return err2
		},
	}

	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{
		jobName: "test-job",
	}

	output := "error 1\nerror 2"
	result := ext.Extract(output, ctxParser)

	if len(result) != 2 {
		t.Fatalf("Extract() returned %d errors, want 2", len(result))
	}

	// Verify contexts are cloned (different pointers but same values)
	if result[0].WorkflowContext == result[1].WorkflowContext {
		t.Error("Extract() should clone workflow context for each error")
	}

	if result[0].WorkflowContext.Job != result[1].WorkflowContext.Job {
		t.Error("Extract() cloned contexts should have same values")
	}
}

func TestExtractor_Extract_ParseContextUpdated(t *testing.T) {
	registry := tools.NewRegistry()

	var capturedCtx *parser.ParseContext
	mockParser := &mockToolParser{
		id:            "test",
		priority:      50,
		canParseScore: 0.9,
		parseFunc: func(line string, ctx *parser.ParseContext) *errors.ExtractedError {
			capturedCtx = ctx
			return &errors.ExtractedError{Message: "test"}
		},
	}

	registry.Register(mockParser)

	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{
		jobName:  "test-job",
		stepName: "test-step",
	}

	ext.Extract("error line", ctxParser)

	if capturedCtx == nil {
		t.Fatal("Parse() was not called")
	}

	if capturedCtx.Job != "test-job" {
		t.Errorf("ParseContext.Job = %q, want %q", capturedCtx.Job, "test-job")
	}

	if capturedCtx.Step != "test-step" {
		t.Errorf("ParseContext.Step = %q, want %q", capturedCtx.Step, "test-step")
	}

	if capturedCtx.WorkflowContext == nil {
		t.Error("ParseContext.WorkflowContext should be set")
	}
}

func BenchmarkSanitizePatternForSentry(b *testing.B) {
	pattern := "error in /Users/john/project/src/main.go:10:5: api_key=sk_live_abcdef1234567890 failed with token ghp_abc123def456"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SanitizePatternForTelemetry(pattern)
	}
}

func BenchmarkExtractor_Extract(b *testing.B) {
	registry := tools.DefaultRegistry()
	ext := NewExtractor(registry)
	ctxParser := &mockContextParser{}

	// Realistic CI output with mix of errors and normal output
	output := `[CI/test] Running tests...
[CI/test] main.go:10:5: undefined: foo
[CI/test] Building project...
[CI/test] internal/extract/extractor.go:50:10: missing return
[CI/test] Tests completed
[CI/test] === FAIL: TestSomething
[CI/test]     Expected: true
[CI/test]     Got: false
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ext.Extract(output, ctxParser)
		ext.Reset()
	}
}
