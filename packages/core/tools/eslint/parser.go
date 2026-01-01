package eslint

import (
	"strconv"
	"strings"

	"github.com/handleui/detent/packages/core/errors"
	"github.com/handleui/detent/packages/core/tools/parser"
)

const (
	parserID       = "eslint"
	parserPriority = 85 // Below Go/TypeScript (90) but above generic (10)
)

// Parser implements parser.ToolParser for ESLint output.
// ESLint stylish format is multi-line: file path on one line, errors indented below.
// Compact format is single-line with all info on one line.
//
// Thread Safety: Parser maintains internal state for multi-line parsing and is NOT
// thread-safe. Create a new Parser instance per goroutine for concurrent use.
// Additionally, Parse() mutates ctx.LastFile when processing file path lines in
// stylish format - ensure each goroutine uses its own ParseContext via Clone().
type Parser struct {
	// Multi-line state for stylish format
	currentFile string // Current file from file path line
	inStylish   bool   // True when we've seen a file path, awaiting errors
}

// NewParser creates a new ESLint parser.
func NewParser() *Parser {
	return &Parser{}
}

// ID implements parser.ToolParser.
func (p *Parser) ID() string {
	return parserID
}

// Priority implements parser.ToolParser.
func (p *Parser) Priority() int {
	return parserPriority
}

// CanParse implements parser.ToolParser.
func (p *Parser) CanParse(line string, ctx *parser.ParseContext) float64 {
	stripped := parser.StripANSI(line)

	// Check for unix format first (highest specificity due to [severity/rule] suffix)
	// This MUST be checked before stylish file pattern to avoid false positives
	if unixPattern.MatchString(stripped) {
		return 0.92
	}

	// Check for file path (starts stylish multi-line sequence)
	if stylishFilePattern.MatchString(stripped) {
		return 0.85
	}

	// Check for stylish error line (with indentation) - single check handles both cases
	if stylishErrorPattern.MatchString(stripped) {
		// High confidence if we have file context from parser state or ParseContext
		if (p.inStylish && p.currentFile != "") || (ctx != nil && ctx.LastFile != "") {
			return 0.90
		}
		return 0.70 // Lower confidence without file context
	}

	// Check for compact format
	if compactPattern.MatchString(stripped) {
		return 0.90
	}

	return 0
}

// Parse implements parser.ToolParser.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	stripped := parser.StripANSI(line)

	// Try unix format first (most specific due to [severity/rule] suffix)
	if match := unixPattern.FindStringSubmatch(stripped); match != nil {
		return p.parseUnixError(match, line, ctx)
	}

	// Check for file path line (starts stylish sequence)
	if match := stylishFilePattern.FindStringSubmatch(stripped); match != nil {
		p.currentFile = match[1]
		p.inStylish = true
		// Update context for cross-parser file context sharing.
		// Note: This mutates ctx - for concurrent use, each goroutine needs its own ctx.
		if ctx != nil {
			ctx.LastFile = p.currentFile
		}
		return nil // File path line doesn't produce an error itself
	}

	// Try stylish format (indented line with line:col error message rule)
	if match := stylishErrorPattern.FindStringSubmatch(stripped); match != nil {
		return p.parseStylishError(match, line, ctx)
	}

	// Try compact format
	if match := compactPattern.FindStringSubmatch(stripped); match != nil {
		return p.parseCompactError(match, line, ctx)
	}

	return nil
}

// parseStylishError creates an ExtractedError from stylish format match.
func (p *Parser) parseStylishError(match []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	// Errors safe to ignore: regex captures (\d+) which guarantees numeric strings
	lineNum, _ := strconv.Atoi(match[1])
	colNum, _ := strconv.Atoi(match[2])
	severity := strings.ToLower(match[3])
	messageAndRule := strings.TrimSpace(match[4])

	// Extract rule ID from end of message
	message, ruleID := extractRuleID(messageAndRule)

	// Determine file from parser state or context
	file := p.currentFile
	if file == "" && ctx != nil {
		file = ctx.LastFile
	}

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Column:   colNum,
		RuleID:   ruleID,
		Severity: severity,
		Category: errors.CategoryLint,
		Source:   errors.SourceESLint,
		Raw:      rawLine,
	}

	ctx.ApplyWorkflowContext(err)

	return err
}

// parseCompactError creates an ExtractedError from compact format match.
func (p *Parser) parseCompactError(match []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := match[1]
	// Errors safe to ignore: regex captures (\d+) which guarantees numeric strings
	lineNum, _ := strconv.Atoi(match[2])
	colNum, _ := strconv.Atoi(match[3])
	severity := strings.ToLower(match[4])
	message := strings.TrimSpace(match[5])
	ruleID := ""
	if len(match) > 6 {
		ruleID = match[6]
	}

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Column:   colNum,
		RuleID:   ruleID,
		Severity: severity,
		Category: errors.CategoryLint,
		Source:   errors.SourceESLint,
		Raw:      rawLine,
	}

	ctx.ApplyWorkflowContext(err)

	return err
}

// parseUnixError creates an ExtractedError from unix format match.
// Unix format: "file.js:8:11: message [error/rule-id]"
func (p *Parser) parseUnixError(match []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := match[1]
	// Errors safe to ignore: regex captures (\d+) which guarantees numeric strings
	lineNum, _ := strconv.Atoi(match[2])
	colNum, _ := strconv.Atoi(match[3])
	message := strings.TrimSpace(match[4])
	severity := strings.ToLower(match[5])
	ruleID := match[6]

	err := &errors.ExtractedError{
		Message:  message,
		File:     file,
		Line:     lineNum,
		Column:   colNum,
		RuleID:   ruleID,
		Severity: severity,
		Category: errors.CategoryLint,
		Source:   errors.SourceESLint,
		Raw:      rawLine,
	}

	ctx.ApplyWorkflowContext(err)

	return err
}

// extractRuleID separates the rule ID from the message in stylish format.
// ESLint stylish format puts rule ID at the end: "Message text  rule-id"
func extractRuleID(messageAndRule string) (message, ruleID string) {
	match := ruleIDPattern.FindStringSubmatch(messageAndRule)
	if match != nil {
		return strings.TrimSpace(match[1]), match[2]
	}
	// No rule ID found, return entire string as message
	return messageAndRule, ""
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
	if !p.inStylish {
		return false
	}

	stripped := parser.StripANSI(line)

	// Empty line signals potential end of file's errors
	if strings.TrimSpace(stripped) == "" {
		return false
	}

	// New file path means new file's errors starting
	if stylishFilePattern.MatchString(stripped) {
		return false // Let Parse handle the new file
	}

	// Summary line signals end of all errors
	if summaryPattern.MatchString(stripped) {
		p.Reset()
		return false
	}

	// Indented error line continues current file
	if stylishErrorPattern.MatchString(stripped) {
		return true
	}

	// Non-matching line ends stylish sequence
	return false
}

// FinishMultiLine implements parser.ToolParser.
func (p *Parser) FinishMultiLine(_ *parser.ParseContext) *errors.ExtractedError {
	p.Reset()
	return nil
}

// Reset implements parser.ToolParser.
func (p *Parser) Reset() {
	p.currentFile = ""
	p.inStylish = false
}

// NoisePatterns returns the ESLint parser's noise detection patterns for registry optimization.
func (p *Parser) NoisePatterns() parser.NoisePatterns {
	return parser.NoisePatterns{
		FastPrefixes: []string{
			"all files pass", // ESLint success message
		},
		FastContains: []string{
			"potentially fixable", // ESLint fixable hints
			"--fix option",        // ESLint fix suggestion
		},
		Regex: noisePatterns,
	}
}

// Compile-time interface compliance check
var _ parser.ToolParser = (*Parser)(nil)

// Ensure Parser implements parser.NoisePatternProvider
var _ parser.NoisePatternProvider = (*Parser)(nil)
