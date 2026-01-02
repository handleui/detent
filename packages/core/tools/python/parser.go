package python

import (
	"strconv"
	"strings"

	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/tools/parser"
)

// tracebackState holds multi-line state for traceback accumulation.
type tracebackState struct {
	inTraceback bool
	file        string // Last (deepest) file seen
	line        int    // Last (deepest) line number
	function    string // Last (deepest) function name
	stackTrace  strings.Builder
	frameCount  int
	// SyntaxError specific
	isSyntaxError bool
	column        int // Column from caret position
	codeContext   string
}

func (s *tracebackState) reset() {
	s.inTraceback = false
	s.file = ""
	s.line = 0
	s.function = ""
	s.stackTrace.Reset()
	s.frameCount = 0
	s.isSyntaxError = false
	s.column = 0
	s.codeContext = ""
}

// Parser implements parser.ToolParser for Python tracebacks, pytest, mypy, ruff, flake8, and pylint.
//
// Thread Safety: Parser maintains internal state for traceback accumulation
// and is NOT thread-safe. Create a new Parser instance per goroutine for concurrent use.
type Parser struct {
	traceback tracebackState
}

// NewParser creates a new Python parser instance.
func NewParser() *Parser {
	return &Parser{}
}

// ID implements parser.ToolParser.
func (p *Parser) ID() string {
	return "python"
}

// Priority implements parser.ToolParser.
func (p *Parser) Priority() int {
	return 90 // Language-specific, same as Go and TypeScript
}

// CanParse implements parser.ToolParser.
// Uses fast path string checks before regex matching for performance.
func (p *Parser) CanParse(line string, _ *parser.ParseContext) float64 {
	stripped := parser.StripANSI(line)

	// Check if we're in a multi-line state (fast path)
	if p.traceback.inTraceback {
		return 0.9
	}

	// Fast path: skip regex for lines that can't possibly match
	if stripped == "" {
		return 0
	}

	// Fast path for traceback start (must begin with "Traceback")
	if strings.HasPrefix(stripped, "Traceback") && tracebackStartPattern.MatchString(stripped) {
		return 0.95
	}

	// Fast path for pytest (must begin with "FAILED" or "ERROR")
	if strings.HasPrefix(stripped, "FAILED") && pytestFailedPattern.MatchString(stripped) {
		return 0.95
	}
	if strings.HasPrefix(stripped, "ERROR") && pytestErrorPattern.MatchString(stripped) {
		return 0.95
	}

	// Fast path for exception lines (must contain ": " and start with uppercase)
	firstChar := stripped[0]
	if firstChar >= 'A' && firstChar <= 'Z' && strings.Contains(stripped, ": ") {
		// Check SyntaxError first (subset of exception pattern)
		if strings.HasPrefix(stripped, "SyntaxError:") ||
			strings.HasPrefix(stripped, "IndentationError:") ||
			strings.HasPrefix(stripped, "TabError:") {
			if syntaxErrorPattern.MatchString(stripped) {
				return 0.9
			}
		}
		// Check general exception pattern
		if exceptionPattern.MatchString(stripped) {
			return 0.95
		}
	}

	// Fast path for .py files (mypy, ruff, flake8, pylint all require .py)
	if strings.Contains(stripped, ".py:") {
		if mypyPatternAlt.MatchString(stripped) {
			return 0.93
		}
		if ruffFlake8Pattern.MatchString(stripped) {
			return 0.93
		}
		if ruffFlake8NoColPattern.MatchString(stripped) {
			return 0.91
		}
		if pylintPattern.MatchString(stripped) {
			return 0.93
		}
	}

	// Fast path for File lines (traceback continuation, must start with whitespace + "File")
	if strings.HasPrefix(stripped, "  File ") && tracebackFilePattern.MatchString(stripped) {
		return 0.8
	}

	return 0
}

// Parse implements parser.ToolParser.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	stripped := parser.StripANSI(line)

	// Handle traceback start
	if tracebackStartPattern.MatchString(stripped) {
		p.startTraceback(line)
		return nil // Wait for traceback to complete
	}

	// Handle pytest FAILED
	if matches := pytestFailedPattern.FindStringSubmatch(stripped); matches != nil {
		return p.parsePytestFailed(matches, line, ctx)
	}

	// Handle pytest ERROR
	if matches := pytestErrorPattern.FindStringSubmatch(stripped); matches != nil {
		return p.parsePytestError(matches, line, ctx)
	}

	// Handle mypy output
	if matches := mypyPatternAlt.FindStringSubmatch(stripped); matches != nil {
		return p.parseMypy(matches, line, ctx)
	}

	// Handle ruff/flake8 with column
	if matches := ruffFlake8Pattern.FindStringSubmatch(stripped); matches != nil {
		return p.parseRuffFlake8(matches, line, ctx)
	}

	// Handle ruff/flake8 without column
	if matches := ruffFlake8NoColPattern.FindStringSubmatch(stripped); matches != nil {
		return p.parseRuffFlake8NoCol(matches, line, ctx)
	}

	// Handle pylint
	if matches := pylintPattern.FindStringSubmatch(stripped); matches != nil {
		return p.parsePylint(matches, line, ctx)
	}

	// Handle standalone SyntaxError BEFORE general exceptions
	// (since SyntaxError ends with "Error" it would match exceptionPattern)
	if matches := syntaxErrorPattern.FindStringSubmatch(stripped); matches != nil {
		return p.parseStandaloneSyntaxError(matches, line, ctx)
	}

	// Handle standalone exception (outside traceback)
	if matches := exceptionPattern.FindStringSubmatch(stripped); matches != nil {
		return p.parseStandaloneException(matches, line, ctx)
	}

	return nil
}

// startTraceback begins accumulating a traceback.
func (p *Parser) startTraceback(rawLine string) {
	p.traceback.inTraceback = true
	p.traceback.stackTrace.Reset()
	p.traceback.stackTrace.WriteString(rawLine)
	p.traceback.stackTrace.WriteString("\n")
	p.traceback.frameCount = 0
	p.traceback.file = ""
	p.traceback.line = 0
	p.traceback.function = ""
	p.traceback.isSyntaxError = false
	p.traceback.column = 0
	p.traceback.codeContext = ""
}

// parsePytestFailed handles pytest FAILED lines.
func (p *Parser) parsePytestFailed(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	testName := matches[2]
	message := matches[3]

	// Build descriptive message
	fullMessage := TruncateMessage("Test failed: " + testName + " - " + message)

	err := &errors.ExtractedError{
		Message:  fullMessage,
		File:     file,
		Severity: "error",
		Raw:      rawLine,
		Category: errors.CategoryTest,
		Source:   errors.SourcePython,
		RuleID:   testName,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// parsePytestError handles pytest ERROR collection lines.
func (p *Parser) parsePytestError(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	message := matches[2]

	err := &errors.ExtractedError{
		Message:  "Collection error: " + message,
		File:     file,
		Severity: "error",
		Raw:      rawLine,
		Category: errors.CategoryTest,
		Source:   errors.SourcePython,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// parseMypy handles mypy output lines.
func (p *Parser) parseMypy(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	lineNum, _ := strconv.Atoi(matches[2])
	severity := matches[3]
	message := matches[4]

	// Extract rule ID if present (in brackets at end)
	ruleID := ""
	if idx := strings.LastIndex(message, " ["); idx != -1 && strings.HasSuffix(message, "]") {
		ruleID = message[idx+2 : len(message)-1]
		message = strings.TrimSpace(message[:idx])
	}

	// Map mypy severity to our severity
	var mappedSeverity string
	switch severity {
	case "warning", "note":
		mappedSeverity = "warning" // Notes are informational, treat as warnings
	default:
		mappedSeverity = "error"
	}

	message = TruncateMessage(message)

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Severity: mappedSeverity,
		Raw:      rawLine,
		Category: errors.CategoryTypeCheck,
		Source:   errors.SourcePython,
		RuleID:   ruleID,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// parseRuffFlake8 handles ruff/flake8 output with column.
func (p *Parser) parseRuffFlake8(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	lineNum, _ := strconv.Atoi(matches[2])
	col, _ := strconv.Atoi(matches[3])
	code := matches[4]
	message := matches[5]

	severity := GetRuffFlake8Severity(code)
	message = TruncateMessage(message)

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Column:   col,
		Severity: severity,
		Raw:      rawLine,
		Category: errors.CategoryLint,
		Source:   errors.SourcePython,
		RuleID:   code,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// parseRuffFlake8NoCol handles ruff/flake8 output without column.
func (p *Parser) parseRuffFlake8NoCol(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	lineNum, _ := strconv.Atoi(matches[2])
	code := matches[3]
	message := matches[4]

	severity := GetRuffFlake8Severity(code)
	message = TruncateMessage(message)

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Severity: severity,
		Raw:      rawLine,
		Category: errors.CategoryLint,
		Source:   errors.SourcePython,
		RuleID:   code,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// parsePylint handles pylint output.
func (p *Parser) parsePylint(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := matches[1]
	lineNum, _ := strconv.Atoi(matches[2])
	col, _ := strconv.Atoi(matches[3])
	code := matches[4]
	message := matches[5]
	ruleID := matches[6]

	severity := GetPylintSeverity(code)
	message = TruncateMessage(message)

	// Use the named rule ID (e.g., "missing-module-docstring") as rule ID
	// but include the code in the message for context
	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Column:   col,
		Severity: severity,
		Raw:      rawLine,
		Category: errors.CategoryLint,
		Source:   errors.SourcePython,
		RuleID:   ruleID,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// parseStandaloneException handles exception lines outside of tracebacks.
func (p *Parser) parseStandaloneException(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	exceptionType := matches[1]
	message := TruncateMessage(matches[2])

	err := &errors.ExtractedError{
		Message:  exceptionType + ": " + message,
		Severity: "error",
		Raw:      rawLine,
		Category: errors.CategoryRuntime,
		Source:   errors.SourcePython,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// parseStandaloneSyntaxError handles SyntaxError lines outside of tracebacks.
func (p *Parser) parseStandaloneSyntaxError(matches []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	errorType := matches[1]
	message := TruncateMessage(matches[2])

	err := &errors.ExtractedError{
		Message:  errorType + ": " + message,
		Severity: "error",
		Raw:      rawLine,
		Category: errors.CategoryCompile,
		Source:   errors.SourcePython,
	}

	ctx.ApplyWorkflowContext(err)
	return err
}

// IsNoise implements parser.ToolParser.
func (p *Parser) IsNoise(line string) bool {
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
	if !p.traceback.inTraceback {
		return false
	}

	return p.continueTraceback(line)
}

// continueTraceback implements a state machine for Python traceback accumulation.
//
// Python tracebacks follow this structure:
//
//	Traceback (most recent call last):
//	  File "a.py", line 10, in function_a    <- Frame 1 (oldest)
//	    code_line_1()
//	  File "b.py", line 20, in function_b    <- Frame 2
//	    code_line_2()
//	  File "c.py", line 30, in function_c    <- Frame N (deepest/most recent)
//	    code_line_n()
//	ExceptionType: message                    <- End marker
//
// State transitions:
//   - File lines: Extract location, increment frame count (we keep the LAST frame as it's deepest)
//   - Code lines: Accumulate context (4+ space indented)
//   - Caret lines: Extract column for SyntaxError (^ markers)
//   - Chained headers: Continue accumulating (exception chains)
//   - Exception/SyntaxError lines: Signal end, return false
//   - Empty lines: Continue accumulating
//   - Other lines: Signal end, return false
//
// Resource limits prevent memory exhaustion from maliciously large tracebacks.
func (p *Parser) continueTraceback(line string) bool {
	stripped := parser.StripANSI(line)

	// Check resource limits
	if p.traceback.frameCount >= maxTracebackFrames || p.traceback.stackTrace.Len() >= maxTracebackBytes {
		// Stop accumulating but look for exception line to finish
		if exceptionPattern.MatchString(stripped) || syntaxErrorPattern.MatchString(stripped) {
			return false
		}
		return true
	}

	// Handle chained exception headers - continue accumulating
	if chainedExceptionPattern.MatchString(stripped) {
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return true
	}

	// Handle exception line - signals end of traceback
	if matches := exceptionPattern.FindStringSubmatch(stripped); matches != nil {
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return false // End of traceback
	}

	// Handle SyntaxError line - signals end of traceback
	if matches := syntaxErrorPattern.FindStringSubmatch(stripped); matches != nil {
		p.traceback.isSyntaxError = true
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return false // End of traceback
	}

	// Handle File line - extract location (we want the LAST/deepest one)
	if matches := tracebackFilePattern.FindStringSubmatch(stripped); matches != nil {
		p.traceback.file = matches[1]
		lineNum, _ := strconv.Atoi(matches[2])
		p.traceback.line = lineNum
		if len(matches) > 3 && matches[3] != "" {
			p.traceback.function = matches[3]
		}
		p.traceback.frameCount++
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return true
	}

	// Handle SyntaxError-specific File line (without function)
	if matches := syntaxErrorFilePattern.FindStringSubmatch(stripped); matches != nil {
		p.traceback.file = matches[1]
		lineNum, _ := strconv.Atoi(matches[2])
		p.traceback.line = lineNum
		p.traceback.isSyntaxError = true
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return true
	}

	// Handle caret line for SyntaxError column detection
	if syntaxErrorCaretPattern.MatchString(stripped) {
		// Calculate column from caret position with bounds validation
		caretPos := strings.Index(stripped, "^")
		if caretPos >= 0 && caretPos < len(stripped) {
			p.traceback.column = caretPos + 1 // 1-indexed
		}
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return true
	}

	// Handle code line in traceback
	if tracebackCodePattern.MatchString(stripped) {
		if p.traceback.isSyntaxError {
			p.traceback.codeContext = strings.TrimSpace(stripped)
		}
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return true
	}

	// Empty lines continue the traceback
	if strings.TrimSpace(stripped) == "" {
		p.traceback.stackTrace.WriteString(line)
		p.traceback.stackTrace.WriteString("\n")
		return true
	}

	// Any other non-matching line signals end
	return false
}

// FinishMultiLine implements parser.ToolParser.
func (p *Parser) FinishMultiLine(ctx *parser.ParseContext) *errors.ExtractedError {
	if !p.traceback.inTraceback {
		return nil
	}

	return p.finishTraceback(ctx)
}

// finishTraceback creates an error from accumulated traceback data.
func (p *Parser) finishTraceback(ctx *parser.ParseContext) *errors.ExtractedError {
	stackTrace := strings.TrimSuffix(p.traceback.stackTrace.String(), "\n")

	// Extract the exception message from the last line
	lines := strings.Split(stackTrace, "\n")
	message := ""
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if matches := exceptionPattern.FindStringSubmatch(line); matches != nil {
			message = matches[1] + ": " + matches[2]
			break
		}
		if matches := syntaxErrorPattern.FindStringSubmatch(line); matches != nil {
			message = matches[1] + ": " + matches[2]
			break
		}
	}

	// If no exception message found, use a generic one
	if message == "" {
		message = "Python exception"
	}

	message = TruncateMessage(message)

	// Determine category
	category := errors.CategoryRuntime
	if p.traceback.isSyntaxError {
		category = errors.CategoryCompile
	}

	err := &errors.ExtractedError{
		Message:    message,
		File:       p.traceback.file,
		Line:       p.traceback.line,
		Column:     p.traceback.column,
		Severity:   "error",
		Raw:        stackTrace,
		StackTrace: stackTrace,
		Category:   category,
		Source:     errors.SourcePython,
	}

	ctx.ApplyWorkflowContext(err)

	p.Reset()
	return err
}

// Reset implements parser.ToolParser.
func (p *Parser) Reset() {
	p.traceback.reset()
}

// NoisePatterns returns the Python parser's noise detection patterns for registry optimization.
// These patterns are Python-specific and should not match errors from other languages.
func (p *Parser) NoisePatterns() parser.NoisePatterns {
	return parser.NoisePatterns{
		FastPrefixes: []string{
			"collecting",
			"collected ",
			"platform linux",
			"platform darwin",
			"platform win",
			"cachedir:",
			"rootdir:",
			"configfile:",
			"plugins:",
			"ok (",
			"your code has been rated",
			"all checks passed",
		},
		FastContains: []string{
			"test session starts",
			"short test summary",
			"warnings summary",
			"files checked",
			"files scanned",
			" passed in ",
			" passed,",
		},
		Regex: noisePatterns,
	}
}

// Ensure Parser implements parser.ToolParser
var _ parser.ToolParser = (*Parser)(nil)

// Ensure Parser implements parser.NoisePatternProvider
var _ parser.NoisePatternProvider = (*Parser)(nil)
