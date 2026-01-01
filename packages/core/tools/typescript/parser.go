package typescript

import (
	"strconv"

	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/tools/parser"
)

const (
	parserID       = "typescript"
	parserPriority = 90 // High priority: very specific .ts/.tsx file format with parentheses
)

// Parser implements parser.ToolParser for TypeScript compiler (tsc) output.
// TypeScript errors are single-line with a distinctive format:
// file.ts(line,col): error TSxxxx: message
//
// Thread Safety: Parser is stateless and thread-safe. Multiple goroutines can safely
// share a single Parser instance. This parser does not mutate ParseContext fields
// (only reads WorkflowContext for cloning into extracted errors).
type Parser struct{}

// NewParser creates a new TypeScript parser.
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
func (p *Parser) CanParse(line string, _ *parser.ParseContext) float64 {
	// Always strip ANSI codes first to handle --pretty output consistently
	stripped := parser.StripANSI(line)
	if tsErrorPattern.MatchString(stripped) {
		return 0.95
	}
	return 0
}

// Parse implements parser.ToolParser.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	// Always strip ANSI codes first to handle --pretty output consistently
	stripped := parser.StripANSI(line)
	match := tsErrorPattern.FindStringSubmatch(stripped)
	if match == nil {
		return nil
	}

	// Regex captures (\d+) which guarantees numeric strings, but we handle
	// errors defensively by falling back to 0 for robustness
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

	ctx.ApplyWorkflowContext(extractedErr)

	return extractedErr
}

// IsNoise implements parser.ToolParser.
func (p *Parser) IsNoise(line string) bool {
	// Strip ANSI codes for consistent matching
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
	return false
}

// ContinueMultiLine implements parser.ToolParser.
func (p *Parser) ContinueMultiLine(_ string, _ *parser.ParseContext) bool {
	return false
}

// FinishMultiLine implements parser.ToolParser.
func (p *Parser) FinishMultiLine(_ *parser.ParseContext) *errors.ExtractedError {
	return nil
}

// Reset implements parser.ToolParser.
func (p *Parser) Reset() {}

// NoisePatterns returns the TypeScript parser's noise detection patterns for registry optimization.
func (p *Parser) NoisePatterns() parser.NoisePatterns {
	return parser.NoisePatterns{
		FastPrefixes: []string{
			"starting compilation",      // tsc watch mode
			"file change detected",      // tsc watch mode
			"watching for file changes", // tsc watch mode
			"found ",                    // TypeScript error summary (Found N errors)
			"version ",                  // TypeScript version output
			"message ts",                // tsc informational messages
			"projects in this build",    // tsc build mode project list
			"building project",          // tsc build mode building
			"updating output",           // tsc build mode timestamps
			"skipping build",            // tsc build mode skip
			"project '",                 // tsc build mode project status
		},
		Regex: noisePatterns,
	}
}

// Compile-time interface compliance check
var _ parser.ToolParser = (*Parser)(nil)

// Ensure Parser implements parser.NoisePatternProvider
var _ parser.NoisePatternProvider = (*Parser)(nil)
