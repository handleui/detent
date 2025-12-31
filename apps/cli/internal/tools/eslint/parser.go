package eslint

import (
	"strconv"
	"strings"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

const (
	parserID       = "eslint"
	parserPriority = 85 // Below Go/TypeScript (90) but above generic (10)
)

// Parser implements parser.ToolParser for ESLint output.
// ESLint stylish format is multi-line: file path on one line, errors indented below.
// Compact format is single-line with all info on one line.
type Parser struct {
	// Multi-line state for stylish format
	currentFile string // Current file from file path line
	inStylish   bool   // True when we've seen a file path, awaiting errors
}

// NewParser creates a new ESLint parser.
func NewParser() *Parser {
	return &Parser{}
}

// ID returns the unique identifier for this parser.
func (p *Parser) ID() string {
	return parserID
}

// Priority returns the parse order priority.
// ESLint uses 85 (specific but below Go/TypeScript which use file extension patterns).
func (p *Parser) Priority() int {
	return parserPriority
}

// CanParse returns a confidence score for parsing this line.
// Returns 0.90+ for stylish error lines (when in multi-line context),
// 0.85 for file paths, 0.92 for unix format (higher due to distinctive suffix),
// and 0.90 for compact format matches.
func (p *Parser) CanParse(line string, ctx *parser.ParseContext) float64 {
	stripped := StripANSI(line)

	// If in multi-line stylish mode, check for continuation
	if p.inStylish && p.currentFile != "" {
		if stylishErrorPattern.MatchString(stripped) {
			return 0.90
		}
	}

	// Check for unix format first (highest specificity due to [severity/rule] suffix)
	// This MUST be checked before stylish file pattern to avoid false positives
	if unixPattern.MatchString(stripped) {
		return 0.92
	}

	// Check for file path (starts stylish multi-line sequence)
	if stylishFilePattern.MatchString(stripped) {
		return 0.85
	}

	// Check for stylish error line (with indentation)
	if stylishErrorPattern.MatchString(stripped) {
		// Need file context from ParseContext
		if ctx != nil && ctx.LastFile != "" {
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

// Parse extracts an ESLint error from the line.
// Returns nil if the line doesn't match ESLint error format.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	stripped := StripANSI(line)

	// Try unix format first (most specific due to [severity/rule] suffix)
	if match := unixPattern.FindStringSubmatch(stripped); match != nil {
		return p.parseUnixError(match, line, ctx)
	}

	// Check for file path line (starts stylish sequence)
	if match := stylishFilePattern.FindStringSubmatch(stripped); match != nil {
		p.currentFile = match[1]
		p.inStylish = true
		// Update context for other parsers
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

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

	return err
}

// parseCompactError creates an ExtractedError from compact format match.
func (p *Parser) parseCompactError(match []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := match[1]
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

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

	return err
}

// parseUnixError creates an ExtractedError from unix format match.
// Unix format: "file.js:8:11: message [error/rule-id]"
func (p *Parser) parseUnixError(match []string, rawLine string, ctx *parser.ParseContext) *errors.ExtractedError {
	file := match[1]
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

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

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

// IsNoise returns true if the line is ESLint-specific noise.
// Filters out summary lines, counts, and other non-error output.
func (p *Parser) IsNoise(line string) bool {
	stripped := StripANSI(line)
	for _, pattern := range noisePatterns {
		if pattern.MatchString(stripped) {
			return true
		}
	}
	return false
}

// SupportsMultiLine returns true as ESLint stylish format requires multi-line handling.
func (p *Parser) SupportsMultiLine() bool {
	return true
}

// ContinueMultiLine processes a continuation line for stylish format.
// Returns true if the line was consumed as part of the stylish output,
// false if it signals the end of the file's errors.
func (p *Parser) ContinueMultiLine(line string, _ *parser.ParseContext) bool {
	if !p.inStylish {
		return false
	}

	stripped := StripANSI(line)

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

// FinishMultiLine finalizes the current multi-line error context.
// For ESLint, errors are emitted immediately from Parse, so this just cleans up.
func (p *Parser) FinishMultiLine(_ *parser.ParseContext) *errors.ExtractedError {
	p.Reset()
	return nil
}

// Reset clears any accumulated multi-line state.
func (p *Parser) Reset() {
	p.currentFile = ""
	p.inStylish = false
}

// Compile-time interface compliance check
var _ parser.ToolParser = (*Parser)(nil)
