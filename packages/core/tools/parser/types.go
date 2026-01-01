package parser

import (
	"regexp"

	"github.com/handleui/detent/packages/core/errors"
)

// NoisePatternProvider is an optional interface that parsers can implement
// to expose their noise patterns for registry-level optimization.
// The registry collects patterns from all registered parsers to build
// an optimized noise checker that avoids checking each parser individually.
type NoisePatternProvider interface {
	// NoisePatterns returns the parser's noise detection patterns.
	// The returned struct contains fast prefix/contains checks and regex patterns.
	// This is optional - parsers not implementing this interface will have their
	// IsNoise() method called directly by the registry.
	NoisePatterns() NoisePatterns
}

// NoisePatterns contains categorized noise detection patterns for optimization.
// Fast string checks are performed before expensive regex matching.
type NoisePatterns struct {
	// FastPrefixes are common prefixes that indicate noise (checked first, case-insensitive).
	// These should be lowercase for case-insensitive matching.
	FastPrefixes []string

	// FastContains are common substrings that indicate noise (case-insensitive).
	// These should be lowercase for case-insensitive matching.
	FastContains []string

	// Regex patterns for noise detection (checked last, most expensive).
	Regex []*regexp.Regexp
}

// ToolParser defines the interface for tool-specific error parsers.
// Each tool (Go, ESLint, TypeScript, etc.) implements this interface
// to handle its specific output format and error patterns.
//
// Thread Safety: ToolParser implementations typically maintain internal state
// for multi-line error parsing. Each parser instance should be used by only
// one goroutine at a time. For concurrent parsing, create separate parser
// instances (via the tool's NewParser function) and separate ParseContext
// instances (via Clone).
//
// The Registry uses sync.RWMutex for its own thread-safety but does not
// protect parser instances. When using parsers concurrently:
//  1. Create a new parser instance per goroutine, OR
//  2. Ensure external synchronization around parser method calls
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
//
// Thread Safety: ParseContext is NOT thread-safe. Each parsing goroutine must
// use its own ParseContext instance. Use Clone() to create an isolated copy
// when spawning concurrent parsing operations. The Extractor creates a fresh
// ParseContext per Extract() call, so sequential calls are safe.
//
// Some parsers (e.g., ESLint) may mutate fields like LastFile to communicate
// file context for multi-line error formats. This is intentional behavior for
// single-threaded parsing but requires isolation for concurrent use.
type ParseContext struct {
	// Job is the current workflow job name (e.g., "[CLI] Test")
	Job string

	// Step is the current step name if detected (e.g., "Run golangci-lint")
	Step string

	// Tool is the detected tool ID for this step, if known from step parsing
	Tool string

	// LastFile tracks the most recently seen file path for multi-line formats
	// where file paths appear on separate lines (e.g., ESLint output).
	// Note: This field may be mutated by parsers during extraction.
	LastFile string

	// BasePath is the workspace root for converting absolute paths to relative
	BasePath string

	// WorkflowContext provides the full workflow context for error attribution
	WorkflowContext *errors.WorkflowContext
}

// Clone creates a deep copy of ParseContext for isolated modifications.
// Use this when spawning concurrent parsing operations to prevent data races.
// The clone includes a deep copy of WorkflowContext if present.
//
// Example for concurrent parsing:
//
//	baseCtx := &ParseContext{Job: "build", Step: "lint"}
//	go func() {
//	    ctx := baseCtx.Clone() // Each goroutine gets its own copy
//	    parser.Parse(line, ctx)
//	}()
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

// NewParseContext creates a new ParseContext with the given workflow context.
// This is the preferred way to create a context for a new parsing session.
func NewParseContext(wc *errors.WorkflowContext) *ParseContext {
	return &ParseContext{
		WorkflowContext: wc,
	}
}

// ApplyWorkflowContext safely assigns the workflow context to an ExtractedError.
// This is nil-safe: if ctx is nil, ctx.WorkflowContext is nil, or err is nil, it does nothing.
// This helper eliminates the repeated nil-check pattern across all parsers.
func (c *ParseContext) ApplyWorkflowContext(err *errors.ExtractedError) {
	if c == nil || c.WorkflowContext == nil || err == nil {
		return
	}
	err.WorkflowContext = c.WorkflowContext.Clone()
}
