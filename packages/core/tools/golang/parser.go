package golang

import (
	"strconv"
	"strings"

	"github.com/handleui/detent/packages/core/errors"
	"github.com/handleui/detent/packages/core/tools/parser"
)

const (
	// maxStackTraceLines limits stack trace accumulation to prevent memory exhaustion
	maxStackTraceLines = 500
	// maxStackTraceBytes limits total stack trace size (500KB)
	maxStackTraceBytes = 512 * 1024
)

// panicState holds multi-line state for panic/stack trace accumulation.
type panicState struct {
	inPanic       bool
	message       string
	file          string
	line          int
	stackTrace    strings.Builder
	stackLines    int
	goroutineSeen bool
}

func (s *panicState) reset() {
	s.inPanic = false
	s.message = ""
	s.file = ""
	s.line = 0
	s.stackTrace.Reset()
	s.stackLines = 0
	s.goroutineSeen = false
}

// testFailureState holds multi-line state for test failure accumulation.
type testFailureState struct {
	inTestFailure  bool
	testName       string
	file           string
	line           int
	message        string
	stackTrace     strings.Builder
	stackLineCount int
}

func (s *testFailureState) reset() {
	s.inTestFailure = false
	s.testName = ""
	s.file = ""
	s.line = 0
	s.message = ""
	s.stackTrace.Reset()
	s.stackLineCount = 0
}

// Parser implements parser.ToolParser for Go compiler, go test, and golangci-lint output.
//
// Thread Safety: Parser maintains internal state for panic and test failure accumulation
// and is NOT thread-safe. Create a new Parser instance per goroutine for concurrent use.
// This parser does not mutate ParseContext fields directly (only reads WorkflowContext
// for cloning into extracted errors).
type Parser struct {
	panic panicState
	test  testFailureState
}

// NewParser creates a new Go parser instance.
func NewParser() *Parser {
	return &Parser{}
}

// ID implements parser.ToolParser.
func (p *Parser) ID() string {
	return "go"
}

// Priority implements parser.ToolParser.
func (p *Parser) Priority() int {
	return 90
}

// CanParse implements parser.ToolParser.
func (p *Parser) CanParse(line string, _ *parser.ParseContext) float64 {
	// Strip ANSI escape codes for pattern matching
	stripped := parser.StripANSI(line)

	// Check if we're in a multi-line state (panic or test failure)
	if p.panic.inPanic || p.test.inTestFailure {
		return 0.9
	}

	// Check for exact pattern matches (high confidence)
	if goErrorPattern.MatchString(stripped) {
		return 0.95
	}

	// Error without column number (still high confidence for Go files)
	if goErrorNoColPattern.MatchString(stripped) {
		return 0.93
	}

	if goTestFailPattern.MatchString(stripped) {
		return 0.95
	}

	if goPanicPattern.MatchString(stripped) {
		return 0.95
	}

	// Go module errors
	if goModuleErrorPattern.MatchString(stripped) {
		return 0.9
	}

	// Lower confidence for stack trace continuation lines
	if goGoroutinePattern.MatchString(stripped) || goStackFramePattern.MatchString(stripped) {
		return 0.8
	}

	return 0
}

// Parse implements parser.ToolParser.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	// Strip ANSI escape codes for pattern matching
	stripped := parser.StripANSI(line)

	// Handle panic start
	if matches := goPanicPattern.FindStringSubmatch(stripped); matches != nil {
		p.startPanic(matches[1], line)
		return nil // Wait for stack trace to complete
	}

	// Handle test failure start
	if matches := goTestFailPattern.FindStringSubmatch(stripped); matches != nil {
		p.startTestFailure(matches[1])
		return nil // Wait for test output to complete
	}

	// Handle standard Go error (compiler, linter) with column
	if matches := goErrorPattern.FindStringSubmatch(stripped); matches != nil {
		// Error safe to ignore: regex captures (\d+) which guarantees numeric string
		lineNum, _ := strconv.Atoi(matches[2])
		col, _ := strconv.Atoi(matches[3])
		return p.parseGoError(matches[1], lineNum, col, matches[4], line, ctx)
	}

	// Handle Go error without column (import cycle, some build errors)
	if matches := goErrorNoColPattern.FindStringSubmatch(stripped); matches != nil {
		// Error safe to ignore: regex captures (\d+) which guarantees numeric string
		lineNum, _ := strconv.Atoi(matches[2])
		return p.parseGoError(matches[1], lineNum, 0, matches[3], line, ctx)
	}

	// Handle Go module errors
	if matches := goModuleErrorPattern.FindStringSubmatch(stripped); matches != nil {
		return p.parseModuleError(matches, line, ctx)
	}

	return nil
}

// parseGoError creates an ExtractedError from a Go error.
// col can be 0 if no column information is available.
func (p *Parser) parseGoError(file string, lineNum, col int, message, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	// Determine source and category based on context and message content
	source := errors.SourceGo
	category := errors.CategoryCompile

	// Check for specific error types (only relevant when no column, but harmless to check always)
	if goImportCyclePattern.MatchString(message) {
		category = errors.CategoryCompile
	} else if goBuildConstraintPattern.MatchString(message) {
		category = errors.CategoryCompile
	}

	// Check for lint tool indicators in context
	if ctx != nil && strings.Contains(strings.ToLower(ctx.Step), "lint") {
		category = errors.CategoryLint
	}

	// Extract rule ID from golangci-lint format
	ruleID := ""
	linterName := ""
	if ruleMatches := golangciLintRulePattern.FindStringSubmatch(message); ruleMatches != nil {
		message = ruleMatches[1]
		ruleID = ruleMatches[2]
		linterName = ruleMatches[2]
		category = errors.CategoryLint
	}

	// Check for static analysis codes (SA4006, G101, ST1000, etc.)
	codePrefix := ""
	if codeMatches := golangciLintCodePattern.FindStringSubmatch(message); codeMatches != nil {
		code := codeMatches[1]
		if ruleID == "" {
			ruleID = code
		} else {
			ruleID = code + "/" + ruleID
		}
		message = codeMatches[2]
		category = errors.CategoryLint

		// Extract code prefix for severity detection (SA, S, ST, QF, G)
		codePrefix = extractCodePrefix(code)
	}

	// Determine severity based on linter name and code prefix
	severity := determineLintSeverity(linterName, codePrefix)

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Column:   col,
		Severity: severity,
		Raw:      rawLine,
		Category: category,
		Source:   source,
		RuleID:   ruleID,
	}

	ctx.ApplyWorkflowContext(err)

	return err
}

// parseModuleError creates an ExtractedError from a Go module error.
func (p *Parser) parseModuleError(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	message := matches[1]

	err := &errors.ExtractedError{
		Message:  message,
		Severity: "error",
		Raw:      rawLine,
		Category: errors.CategoryCompile,
		Source:   errors.SourceGo,
	}

	ctx.ApplyWorkflowContext(err)

	return err
}

// extractCodePrefix extracts the letter prefix from a lint code (e.g., "SA" from "SA4006").
func extractCodePrefix(code string) string {
	for i, r := range code {
		if r >= '0' && r <= '9' {
			return code[:i]
		}
	}
	return code
}

// determineLintSeverity determines the severity based on linter name and code prefix.
func determineLintSeverity(linterName, codePrefix string) string {
	// Check code prefix first (more specific)
	if codePrefix != "" {
		if sev, ok := CodePrefixSeverity[codePrefix]; ok {
			return sev
		}
	}

	// Check linter name
	if linterName != "" {
		if sev, ok := KnownLinters[linterName]; ok {
			return sev
		}
	}

	// Default to error for unknown linters (safer)
	return "error"
}

// startPanic begins accumulating a panic stack trace.
func (p *Parser) startPanic(message, rawLine string) {
	p.panic.inPanic = true
	p.panic.message = message
	p.panic.stackTrace.Reset()
	p.panic.stackTrace.WriteString(rawLine)
	p.panic.stackTrace.WriteString("\n")
	p.panic.stackLines = 1
	p.panic.goroutineSeen = false
	p.panic.file = ""
	p.panic.line = 0
}

// startTestFailure begins accumulating test failure output.
func (p *Parser) startTestFailure(testName string) {
	p.test.inTestFailure = true
	p.test.testName = testName
	p.test.file = ""
	p.test.line = 0
	p.test.message = ""
	p.test.stackTrace.Reset()
	p.test.stackLineCount = 0
}

// IsNoise implements parser.ToolParser.
func (p *Parser) IsNoise(line string) bool {
	// Strip ANSI escape codes for pattern matching
	stripped := parser.StripANSI(line)

	for _, pattern := range noisePatterns {
		if pattern.MatchString(stripped) {
			return true
		}
	}
	return false
}

// SupportsMultiLine implements parser.ToolParser.
func (p *Parser) SupportsMultiLine() bool {
	return true
}

// ContinueMultiLine implements parser.ToolParser.
func (p *Parser) ContinueMultiLine(line string, _ *parser.ParseContext) bool {
	if p.panic.inPanic {
		return p.continuePanic(line)
	}

	if p.test.inTestFailure {
		return p.continueTestFailure(line)
	}

	return false
}

// continuePanic handles panic stack trace continuation.
func (p *Parser) continuePanic(line string) bool {
	// Check resource limits to prevent memory exhaustion
	if p.panic.stackLines >= maxStackTraceLines || p.panic.stackTrace.Len() >= maxStackTraceBytes {
		// Stop accumulating but continue parsing to find end of stack trace
		if p.panic.goroutineSeen && strings.TrimSpace(line) == "" {
			return false
		}
		return true
	}

	// Empty line might signal end of stack trace
	if strings.TrimSpace(line) == "" {
		// Only end if we've seen at least one goroutine
		if p.panic.goroutineSeen {
			return false
		}
		// Otherwise include it and continue
		p.panic.stackTrace.WriteString(line)
		p.panic.stackTrace.WriteString("\n")
		p.panic.stackLines++
		return true
	}

	// Goroutine header
	if goGoroutinePattern.MatchString(line) {
		p.panic.goroutineSeen = true
		p.panic.stackTrace.WriteString(line)
		p.panic.stackTrace.WriteString("\n")
		p.panic.stackLines++
		return true
	}

	// Stack frame (function call or file location)
	if goStackFramePattern.MatchString(line) {
		p.panic.stackTrace.WriteString(line)
		p.panic.stackTrace.WriteString("\n")
		p.panic.stackLines++

		// Extract first file location as the error location
		if p.panic.file == "" {
			if matches := goStackFilePattern.FindStringSubmatch(line); matches != nil {
				p.panic.file = matches[1]
				// Error safe to ignore: regex captures (\d+) which guarantees numeric string
				p.panic.line, _ = strconv.Atoi(matches[2])
			}
		}
		return true
	}

	// If we've seen a goroutine but this line doesn't match stack patterns, end
	if p.panic.goroutineSeen {
		return false
	}

	// Before goroutine, accumulate everything (could be additional panic info)
	p.panic.stackTrace.WriteString(line)
	p.panic.stackTrace.WriteString("\n")
	p.panic.stackLines++
	return true
}

// continueTestFailure handles test failure output continuation.
func (p *Parser) continueTestFailure(line string) bool {
	// Check resource limits to prevent memory exhaustion
	if p.test.stackLineCount >= maxStackTraceLines || p.test.stackTrace.Len() >= maxStackTraceBytes {
		// Stop accumulating but continue to find end of test output
		if strings.TrimSpace(line) == "" || !testOutputPattern.MatchString(line) {
			return false
		}
		return true
	}

	// Check for test output with file:line reference
	if matches := testFileLinePattern.FindStringSubmatch(line); matches != nil {
		// Only capture first file/line as the error location
		if p.test.file == "" {
			p.test.file = matches[1]
			// Error safe to ignore: regex captures (\d+) which guarantees numeric string
			p.test.line, _ = strconv.Atoi(matches[2])
			p.test.message = matches[3]
		}
		p.test.stackTrace.WriteString(line)
		p.test.stackTrace.WriteString("\n")
		p.test.stackLineCount++
		return true
	}

	// Indented continuation lines
	if testOutputPattern.MatchString(line) {
		p.test.stackTrace.WriteString(line)
		p.test.stackTrace.WriteString("\n")
		p.test.stackLineCount++
		return true
	}

	// Empty line continues the test output context
	if strings.TrimSpace(line) == "" {
		return true
	}

	// Non-indented, non-empty line ends test failure
	return false
}

// FinishMultiLine implements parser.ToolParser.
func (p *Parser) FinishMultiLine(ctx *parser.ParseContext) *errors.ExtractedError {
	if p.panic.inPanic {
		return p.finishPanic(ctx)
	}

	if p.test.inTestFailure {
		return p.finishTestFailure(ctx)
	}

	return nil
}

// finishPanic creates an error from accumulated panic data.
func (p *Parser) finishPanic(ctx *parser.ParseContext) *errors.ExtractedError {
	if !p.panic.inPanic {
		return nil
	}

	err := &errors.ExtractedError{
		Message:    "panic: " + p.panic.message,
		File:       p.panic.file,
		Line:       p.panic.line,
		Severity:   "error",
		Raw:        strings.TrimSuffix(p.panic.stackTrace.String(), "\n"),
		StackTrace: strings.TrimSuffix(p.panic.stackTrace.String(), "\n"),
		Category:   errors.CategoryRuntime,
		Source:     errors.SourceGo,
	}

	ctx.ApplyWorkflowContext(err)

	p.Reset()
	return err
}

// finishTestFailure creates an error from accumulated test failure data.
func (p *Parser) finishTestFailure(ctx *parser.ParseContext) *errors.ExtractedError {
	if !p.test.inTestFailure {
		return nil
	}

	message := "FAIL: " + p.test.testName
	if p.test.message != "" {
		message = p.test.message
	}

	stackTrace := strings.TrimSuffix(p.test.stackTrace.String(), "\n")

	err := &errors.ExtractedError{
		Message:    message,
		File:       p.test.file,
		Line:       p.test.line,
		Severity:   "error",
		Raw:        "--- FAIL: " + p.test.testName,
		StackTrace: stackTrace,
		Category:   errors.CategoryTest,
		Source:     errors.SourceGoTest,
	}

	ctx.ApplyWorkflowContext(err)

	p.Reset()
	return err
}

// Reset implements parser.ToolParser.
func (p *Parser) Reset() {
	p.panic.reset()
	p.test.reset()
}

// NoisePatterns returns the Go parser's noise detection patterns for registry optimization.
func (p *Parser) NoisePatterns() parser.NoisePatterns {
	return parser.NoisePatterns{
		FastPrefixes: []string{
			"=== run",   // Go test start
			"=== pause", // Go test pause
			"=== cont",  // Go test continue
			"=== name",  // Go test name (Go 1.20+)
			"--- pass:", // Go test pass
			"--- skip:", // Go test skip
			"pass",      // Go overall pass (exact)
			"ok ",       // Go package pass (ok package 0.123s)
			"? ",        // Go no test files
			"# ",        // Go build package header (# package/path)
			"go: ",      // Go tool messages (go: downloading, go: finding)
			"level=",    // golangci-lint debug output
			"issues:",   // golangci-lint summary
			"coverage:", // Go test coverage
			"running ",  // golangci-lint progress
		},
		Regex: noisePatterns,
	}
}

// Ensure Parser implements parser.ToolParser
var _ parser.ToolParser = (*Parser)(nil)

// Ensure Parser implements parser.NoisePatternProvider
var _ parser.NoisePatternProvider = (*Parser)(nil)
