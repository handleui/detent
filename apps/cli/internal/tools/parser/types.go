package parser

import (
	"github.com/detent/cli/internal/errors"
)

// ToolParser defines the interface for tool-specific error parsers.
// Each tool (Go, ESLint, TypeScript, etc.) implements this interface
// to handle its specific output format and error patterns.
type ToolParser interface {
	// ID returns the unique identifier for this parser (e.g., "go", "eslint", "typescript")
	ID() string

	// Priority returns the parse order priority.
	// Higher values are tried first (more specific parsers should have higher priority).
	// Recommended ranges:
	//   90-100: Very specific parsers (exact format match)
	//   70-89:  Specific parsers (language-specific format)
	//   50-69:  General parsers (common patterns)
	//   0-49:   Fallback parsers (last resort)
	Priority() int

	// CanParse returns a confidence score (0.0-1.0) indicating how likely
	// this parser can handle the given line. Higher scores indicate more
	// confidence. Returns 0 if the line doesn't match this parser's format.
	CanParse(line string, ctx *ParseContext) float64

	// Parse extracts an error from the line.
	// Returns nil if the line doesn't contain a parseable error.
	// The ParseContext provides additional context for stateful parsing.
	Parse(line string, ctx *ParseContext) *errors.ExtractedError

	// IsNoise returns true if the line is tool-specific noise that should be skipped.
	// Examples: progress indicators, timing info, decorative output.
	IsNoise(line string) bool

	// SupportsMultiLine returns true if this parser handles multi-line errors
	// (e.g., Python tracebacks, Go panics, Rust error messages).
	SupportsMultiLine() bool

	// ContinueMultiLine processes a continuation line for multi-line errors.
	// Returns true if the line was consumed as part of the multi-line error,
	// false if it signals the end of the multi-line sequence.
	// Only called when SupportsMultiLine() returns true and an error is in progress.
	ContinueMultiLine(line string, ctx *ParseContext) bool

	// FinishMultiLine finalizes the current multi-line error and returns it.
	// Called when ContinueMultiLine returns false or when input ends.
	FinishMultiLine(ctx *ParseContext) *errors.ExtractedError

	// Reset clears any accumulated multi-line state.
	// Called between parsing runs or when switching context.
	Reset()
}

// ParseContext provides shared context for tool parsers during extraction.
// It maintains state across lines and provides contextual information.
type ParseContext struct {
	// Job is the current workflow job name (e.g., "[CLI] Test")
	Job string

	// Step is the current step name if detected (e.g., "Run golangci-lint")
	Step string

	// Tool is the detected tool ID for this step, if known from step parsing
	Tool string

	// LastFile tracks the most recently seen file path for multi-line formats
	// where file paths appear on separate lines (e.g., ESLint output)
	LastFile string

	// BasePath is the workspace root for converting absolute paths to relative
	BasePath string

	// WorkflowContext provides the full workflow context for error attribution
	WorkflowContext *errors.WorkflowContext
}

// Clone creates a shallow copy of ParseContext for isolated modifications
func (c *ParseContext) Clone() *ParseContext {
	if c == nil {
		return nil
	}
	clone := *c
	if c.WorkflowContext != nil {
		clone.WorkflowContext = c.WorkflowContext.Clone()
	}
	return &clone
}
