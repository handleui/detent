package typescript

import (
	"strconv"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

const (
	parserID       = "typescript"
	parserPriority = 90 // High priority: very specific .ts/.tsx file format with parentheses
)

// Parser implements parser.ToolParser for TypeScript compiler (tsc) output.
// TypeScript errors are single-line with a distinctive format:
// file.ts(line,col): error TSxxxx: message
type Parser struct{}

// NewParser creates a new TypeScript parser.
func NewParser() *Parser {
	return &Parser{}
}

// ID returns the unique identifier for this parser.
func (p *Parser) ID() string {
	return parserID
}

// Priority returns the parse order priority.
// TypeScript uses 90 (very specific format with parentheses for line,col).
func (p *Parser) Priority() int {
	return parserPriority
}

// CanParse returns a confidence score for parsing this line.
// Returns 0.95 for exact pattern matches (very high confidence due to unique format).
// Also handles tsc --pretty output by stripping ANSI escape codes before matching.
func (p *Parser) CanParse(line string, _ *parser.ParseContext) float64 {
	// Always strip ANSI codes first to handle --pretty output consistently
	stripped := StripANSI(line)
	if tsErrorPattern.MatchString(stripped) {
		return 0.95
	}
	return 0
}

// Parse extracts a TypeScript error from the line.
// Returns nil if the line doesn't match the TypeScript error format.
// Handles tsc --pretty output by stripping ANSI escape codes before parsing.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	// Always strip ANSI codes first to handle --pretty output consistently
	stripped := StripANSI(line)
	match := tsErrorPattern.FindStringSubmatch(stripped)
	if match == nil {
		return nil
	}

	lineNum, err := strconv.Atoi(match[2])
	if err != nil {
		lineNum = 0
	}

	colNum, err := strconv.Atoi(match[3])
	if err != nil {
		colNum = 0
	}

	extractedErr := &errors.ExtractedError{
		Message:  match[5],       // Group 5: error message
		File:     match[1],       // Group 1: file path
		Line:     lineNum,        // Group 2: line number
		Column:   colNum,         // Group 3: column number
		RuleID:   match[4],       // Group 4: TS error code (may be empty)
		Severity: "error",        // TypeScript compiler errors are always errors
		Category: errors.CategoryTypeCheck,
		Source:   errors.SourceTypeScript,
		Raw:      line,
	}

	// Propagate workflow context if available
	if ctx != nil && ctx.WorkflowContext != nil {
		extractedErr.WorkflowContext = ctx.WorkflowContext.Clone()
	}

	return extractedErr
}

// IsNoise returns true if the line is TypeScript-specific noise.
// TypeScript compiler output includes some informational lines that should be skipped.
// Handles tsc --pretty output by stripping ANSI escape codes before checking.
func (p *Parser) IsNoise(line string) bool {
	// Strip ANSI codes for consistent matching
	stripped := StripANSI(line)
	for _, pattern := range noisePatterns {
		if pattern.MatchString(stripped) {
			return true
		}
	}
	return false
}

// SupportsMultiLine returns false as TypeScript errors are single-line.
func (p *Parser) SupportsMultiLine() bool {
	return false
}

// ContinueMultiLine is not used for TypeScript (single-line errors).
func (p *Parser) ContinueMultiLine(_ string, _ *parser.ParseContext) bool {
	return false
}

// FinishMultiLine is not used for TypeScript (single-line errors).
func (p *Parser) FinishMultiLine(_ *parser.ParseContext) *errors.ExtractedError {
	return nil
}

// Reset clears any accumulated state. No-op for TypeScript (stateless).
func (p *Parser) Reset() {}

// Compile-time interface compliance check
var _ parser.ToolParser = (*Parser)(nil)
