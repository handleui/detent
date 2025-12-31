package golang

import (
	"strconv"
	"strings"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

const (
	// maxStackTraceLines limits stack trace accumulation to prevent memory exhaustion
	maxStackTraceLines = 500
	// maxStackTraceBytes limits total stack trace size (500KB)
	maxStackTraceBytes = 512 * 1024
)

// Parser implements parser.ToolParser for Go compiler, go test, and golangci-lint output.
type Parser struct {
	// Multi-line state for panic/stack trace accumulation
	inPanic         bool
	panicMessage    string
	panicFile       string
	panicLine       int
	stackTrace      strings.Builder
	stackTraceLines int // Track line count for limiting
	goroutineSeen   bool

	// Multi-line state for test failure accumulation
	inTestFailure      bool
	testName           string
	testFile           string
	testLine           int
	testMessage        string
	testStackTrace     strings.Builder
	testStackLineCount int // Track line count for limiting
}

// NewParser creates a new Go parser instance.
func NewParser() *Parser {
	return &Parser{}
}

// ID returns the unique identifier for this parser.
func (p *Parser) ID() string {
	return "go"
}

// Priority returns the parse order priority.
// Go errors have a very specific format (file.go:line:col: message).
func (p *Parser) Priority() int {
	return 90
}

// CanParse returns a confidence score for whether this parser can handle the line.
func (p *Parser) CanParse(line string, _ *parser.ParseContext) float64 {
	// Check if we're in a multi-line state (panic or test failure)
	if p.inPanic || p.inTestFailure {
		return 0.9
	}

	// Check for exact pattern matches (high confidence)
	if goErrorPattern.MatchString(line) {
		return 0.95
	}

	// Error without column number (still high confidence for Go files)
	if goErrorNoColPattern.MatchString(line) {
		return 0.93
	}

	if goTestFailPattern.MatchString(line) {
		return 0.95
	}

	if goPanicPattern.MatchString(line) {
		return 0.95
	}

	// Go module errors
	if goModuleErrorPattern.MatchString(line) {
		return 0.9
	}

	// Lower confidence for stack trace continuation lines
	if goGoroutinePattern.MatchString(line) || goStackFramePattern.MatchString(line) {
		return 0.8
	}

	return 0
}

// Parse extracts an error from the line.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	// Handle panic start
	if matches := goPanicPattern.FindStringSubmatch(line); matches != nil {
		p.startPanic(matches[1], line)
		return nil // Wait for stack trace to complete
	}

	// Handle test failure start
	if matches := goTestFailPattern.FindStringSubmatch(line); matches != nil {
		p.startTestFailure(matches[1])
		return nil // Wait for test output to complete
	}

	// Handle standard Go error (compiler, linter) with column
	if matches := goErrorPattern.FindStringSubmatch(line); matches != nil {
		return p.parseGoError(matches, line, ctx)
	}

	// Handle Go error without column (import cycle, some build errors)
	if matches := goErrorNoColPattern.FindStringSubmatch(line); matches != nil {
		return p.parseGoErrorNoCol(matches, line, ctx)
	}

	// Handle Go module errors
	if matches := goModuleErrorPattern.FindStringSubmatch(line); matches != nil {
		return p.parseModuleError(matches, line, ctx)
	}

	return nil
}

// parseGoError creates an ExtractedError from a Go error pattern match.
func (p *Parser) parseGoError(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	lineNum, _ := strconv.Atoi(matches[2])
	col, _ := strconv.Atoi(matches[3])
	message := matches[4]

	// Determine source and category based on context and message content
	source := errors.SourceGo
	category := errors.CategoryCompile

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

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

	return err
}

// parseGoErrorNoCol creates an ExtractedError from a Go error without column number.
func (p *Parser) parseGoErrorNoCol(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	lineNum, _ := strconv.Atoi(matches[2])
	message := matches[3]

	source := errors.SourceGo
	category := errors.CategoryCompile

	// Check for specific error types
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

	// Check for static analysis codes
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
		codePrefix = extractCodePrefix(code)
	}

	severity := determineLintSeverity(linterName, codePrefix)

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Column:   0, // No column info
		Severity: severity,
		Raw:      rawLine,
		Category: category,
		Source:   source,
		RuleID:   ruleID,
	}

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

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

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

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
	p.inPanic = true
	p.panicMessage = message
	p.stackTrace.Reset()
	p.stackTrace.WriteString(rawLine)
	p.stackTrace.WriteString("\n")
	p.stackTraceLines = 1
	p.goroutineSeen = false
	p.panicFile = ""
	p.panicLine = 0
}

// startTestFailure begins accumulating test failure output.
func (p *Parser) startTestFailure(testName string) {
	p.inTestFailure = true
	p.testName = testName
	p.testFile = ""
	p.testLine = 0
	p.testMessage = ""
	p.testStackTrace.Reset()
	p.testStackLineCount = 0
}

// IsNoise returns true if the line is Go-specific noise.
func (p *Parser) IsNoise(line string) bool {
	for _, pattern := range noisePatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// SupportsMultiLine returns true as Go supports panic stack traces and test output.
func (p *Parser) SupportsMultiLine() bool {
	return true
}

// ContinueMultiLine processes a continuation line for multi-line errors.
func (p *Parser) ContinueMultiLine(line string, _ *parser.ParseContext) bool {
	if p.inPanic {
		return p.continuePanic(line)
	}

	if p.inTestFailure {
		return p.continueTestFailure(line)
	}

	return false
}

// continuePanic handles panic stack trace continuation.
func (p *Parser) continuePanic(line string) bool {
	// Check resource limits to prevent memory exhaustion
	if p.stackTraceLines >= maxStackTraceLines || p.stackTrace.Len() >= maxStackTraceBytes {
		// Stop accumulating but continue parsing to find end of stack trace
		if p.goroutineSeen && strings.TrimSpace(line) == "" {
			return false
		}
		return true
	}

	// Empty line might signal end of stack trace
	if strings.TrimSpace(line) == "" {
		// Only end if we've seen at least one goroutine
		if p.goroutineSeen {
			return false
		}
		// Otherwise include it and continue
		p.stackTrace.WriteString(line)
		p.stackTrace.WriteString("\n")
		p.stackTraceLines++
		return true
	}

	// Goroutine header
	if goGoroutinePattern.MatchString(line) {
		p.goroutineSeen = true
		p.stackTrace.WriteString(line)
		p.stackTrace.WriteString("\n")
		p.stackTraceLines++
		return true
	}

	// Stack frame (function call or file location)
	if goStackFramePattern.MatchString(line) {
		p.stackTrace.WriteString(line)
		p.stackTrace.WriteString("\n")
		p.stackTraceLines++

		// Extract first file location as the error location
		if p.panicFile == "" {
			if matches := goStackFilePattern.FindStringSubmatch(line); matches != nil {
				p.panicFile = matches[1]
				p.panicLine, _ = strconv.Atoi(matches[2])
			}
		}
		return true
	}

	// If we've seen a goroutine but this line doesn't match stack patterns, end
	if p.goroutineSeen {
		return false
	}

	// Before goroutine, accumulate everything (could be additional panic info)
	p.stackTrace.WriteString(line)
	p.stackTrace.WriteString("\n")
	p.stackTraceLines++
	return true
}

// continueTestFailure handles test failure output continuation.
func (p *Parser) continueTestFailure(line string) bool {
	// Check resource limits to prevent memory exhaustion
	if p.testStackLineCount >= maxStackTraceLines || p.testStackTrace.Len() >= maxStackTraceBytes {
		// Stop accumulating but continue to find end of test output
		if strings.TrimSpace(line) == "" || !testOutputPattern.MatchString(line) {
			return false
		}
		return true
	}

	// Check for test output with file:line reference
	if matches := testFileLinePattern.FindStringSubmatch(line); matches != nil {
		// Only capture first file/line as the error location
		if p.testFile == "" {
			p.testFile = matches[1]
			p.testLine, _ = strconv.Atoi(matches[2])
			p.testMessage = matches[3]
		}
		p.testStackTrace.WriteString(line)
		p.testStackTrace.WriteString("\n")
		p.testStackLineCount++
		return true
	}

	// Indented continuation lines
	if testOutputPattern.MatchString(line) {
		p.testStackTrace.WriteString(line)
		p.testStackTrace.WriteString("\n")
		p.testStackLineCount++
		return true
	}

	// Empty line continues the test output context
	if strings.TrimSpace(line) == "" {
		return true
	}

	// Non-indented, non-empty line ends test failure
	return false
}

// FinishMultiLine finalizes the current multi-line error.
func (p *Parser) FinishMultiLine(ctx *parser.ParseContext) *errors.ExtractedError {
	if p.inPanic {
		return p.finishPanic(ctx)
	}

	if p.inTestFailure {
		return p.finishTestFailure(ctx)
	}

	return nil
}

// finishPanic creates an error from accumulated panic data.
func (p *Parser) finishPanic(ctx *parser.ParseContext) *errors.ExtractedError {
	if !p.inPanic {
		return nil
	}

	err := &errors.ExtractedError{
		Message:    "panic: " + p.panicMessage,
		File:       p.panicFile,
		Line:       p.panicLine,
		Severity:   "error",
		Raw:        strings.TrimSuffix(p.stackTrace.String(), "\n"),
		StackTrace: strings.TrimSuffix(p.stackTrace.String(), "\n"),
		Category:   errors.CategoryRuntime,
		Source:     errors.SourceGo,
	}

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

	p.Reset()
	return err
}

// finishTestFailure creates an error from accumulated test failure data.
func (p *Parser) finishTestFailure(ctx *parser.ParseContext) *errors.ExtractedError {
	if !p.inTestFailure {
		return nil
	}

	message := "FAIL: " + p.testName
	if p.testMessage != "" {
		message = p.testMessage
	}

	stackTrace := strings.TrimSuffix(p.testStackTrace.String(), "\n")

	err := &errors.ExtractedError{
		Message:    message,
		File:       p.testFile,
		Line:       p.testLine,
		Severity:   "error",
		Raw:        "--- FAIL: " + p.testName,
		StackTrace: stackTrace,
		Category:   errors.CategoryTest,
		Source:     errors.SourceGoTest,
	}

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

	p.Reset()
	return err
}

// Reset clears any accumulated multi-line state.
func (p *Parser) Reset() {
	p.inPanic = false
	p.panicMessage = ""
	p.panicFile = ""
	p.panicLine = 0
	p.stackTrace.Reset()
	p.stackTraceLines = 0
	p.goroutineSeen = false

	p.inTestFailure = false
	p.testName = ""
	p.testFile = ""
	p.testLine = 0
	p.testMessage = ""
	p.testStackTrace.Reset()
	p.testStackLineCount = 0
}

// Ensure Parser implements parser.ToolParser
var _ parser.ToolParser = (*Parser)(nil)
