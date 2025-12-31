package rust

import (
	"strconv"
	"strings"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

const (
	// maxContextLines limits context accumulation to prevent memory exhaustion
	maxContextLines = 200
	// maxContextBytes limits total context size (256KB)
	maxContextBytes = 256 * 1024
)

// Parser implements parser.ToolParser for Rust compiler (rustc), Cargo, and Clippy output.
type Parser struct {
	// Multi-line state for error accumulation
	inError      bool
	errorLevel   string // "error" or "warning"
	errorCode    string // e.g., "E0308" or ""
	errorMessage string
	errorFile    string
	errorLine    int
	errorColumn  int

	// Context accumulation
	contextLines strings.Builder
	contextCount int

	// Notes and help messages
	notes []string
	helps []string

	// Clippy lint code (extracted from notes)
	clippyLint string
}

// NewParser creates a new Rust parser instance.
func NewParser() *Parser {
	return &Parser{}
}

// ID returns the unique identifier for this parser.
func (p *Parser) ID() string {
	return "rust"
}

// Priority returns the parse order priority.
// Rust errors have a specific format with error codes and location arrows.
func (p *Parser) Priority() int {
	return 85
}

// CanParse returns a confidence score for whether this parser can handle the line.
func (p *Parser) CanParse(line string, _ *parser.ParseContext) float64 {
	// Check if we're in a multi-line state
	if p.inError {
		return 0.9
	}

	// Check for error/warning header with code (high confidence)
	if match := rustErrorHeaderPattern.FindStringSubmatch(line); match != nil {
		// Higher confidence if it has an error code
		if match[2] != "" {
			return 0.95
		}
		return 0.85
	}

	// Location arrow is Rust-specific
	if rustLocationPattern.MatchString(line) {
		return 0.90
	}

	// Test failure
	if rustTestFailPattern.MatchString(line) {
		return 0.95
	}

	return 0
}

// Parse extracts an error from the line.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	// Handle error/warning header
	if match := rustErrorHeaderPattern.FindStringSubmatch(line); match != nil {
		// If we have a pending error, finalize it first
		if p.inError {
			err := p.buildError(ctx)
			p.Reset()
			p.startError(match[1], match[2], match[3], line)
			return err
		}
		p.startError(match[1], match[2], match[3], line)
		return nil // Wait for location and context
	}

	// Handle location arrow (extract file/line/col)
	if match := rustLocationPattern.FindStringSubmatch(line); match != nil {
		if p.inError && p.errorFile == "" {
			p.errorFile = match[1]
			p.errorLine, _ = strconv.Atoi(match[2])
			p.errorColumn, _ = strconv.Atoi(match[3])
		}
		p.addContextLine(line)
		return nil
	}

	// Handle note/help lines
	if match := rustNotePattern.FindStringSubmatch(line); match != nil {
		if p.inError {
			noteType := match[1]
			noteMsg := match[2]

			switch noteType {
			case "note":
				p.notes = append(p.notes, noteMsg)
				// Check for Clippy lint code in note
				if lintMatch := rustClippyLintPattern.FindStringSubmatch(noteMsg); lintMatch != nil {
					p.clippyLint = lintMatch[1]
				}
			case "help":
				p.helps = append(p.helps, noteMsg)
			}
		}
		p.addContextLine(line)
		return nil
	}

	// Handle test failure
	if match := rustTestFailPattern.FindStringSubmatch(line); match != nil {
		return &errors.ExtractedError{
			Message:  "test failed: " + match[1],
			Severity: "error",
			Raw:      line,
			Category: errors.CategoryTest,
			Source:   errors.SourceRust,
		}
	}

	return nil
}

// startError begins accumulating a Rust error.
func (p *Parser) startError(level, code, message, rawLine string) {
	p.inError = true
	p.errorLevel = level
	p.errorCode = code
	p.errorMessage = message
	p.contextLines.Reset()
	p.contextLines.WriteString(rawLine)
	p.contextLines.WriteString("\n")
	p.contextCount = 1
	p.notes = nil
	p.helps = nil
	p.clippyLint = ""
	p.errorFile = ""
	p.errorLine = 0
	p.errorColumn = 0
}

// addContextLine adds a line to the accumulated context.
func (p *Parser) addContextLine(line string) {
	if !p.inError {
		return
	}

	// Check resource limits
	if p.contextCount >= maxContextLines || p.contextLines.Len() >= maxContextBytes {
		return
	}

	p.contextLines.WriteString(line)
	p.contextLines.WriteString("\n")
	p.contextCount++
}

// buildError creates an ExtractedError from accumulated state.
func (p *Parser) buildError(ctx *parser.ParseContext) *errors.ExtractedError {
	if !p.inError {
		return nil
	}

	// Determine severity
	severity := p.errorLevel
	if severity == "warning" {
		// Check if this Clippy lint should be treated as error
		if p.clippyLint != "" && CriticalClippyLints[p.clippyLint] {
			severity = "error"
		}
	}

	// Determine rule ID
	ruleID := p.errorCode
	if p.clippyLint != "" {
		if ruleID != "" {
			ruleID = ruleID + "/clippy::" + p.clippyLint
		} else {
			ruleID = "clippy::" + p.clippyLint
		}
	}

	// Determine category
	category := errors.CategoryCompile
	if p.clippyLint != "" {
		category = errors.CategoryLint
	}

	stackTrace := strings.TrimSuffix(p.contextLines.String(), "\n")

	err := &errors.ExtractedError{
		Message:    p.errorMessage,
		File:       p.errorFile,
		Line:       p.errorLine,
		Column:     p.errorColumn,
		Severity:   severity,
		Raw:        stackTrace,
		StackTrace: stackTrace,
		RuleID:     ruleID,
		Category:   category,
		Source:     errors.SourceRust,
	}

	if ctx != nil && ctx.WorkflowContext != nil {
		err.WorkflowContext = ctx.WorkflowContext.Clone()
	}

	return err
}

// IsNoise returns true if the line is Rust-specific noise.
func (p *Parser) IsNoise(line string) bool {
	for _, pattern := range noisePatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

// SupportsMultiLine returns true as Rust errors span multiple lines.
func (p *Parser) SupportsMultiLine() bool {
	return true
}

// ContinueMultiLine processes a continuation line for multi-line errors.
func (p *Parser) ContinueMultiLine(line string, _ *parser.ParseContext) bool {
	if !p.inError {
		return false
	}

	// Check resource limits
	if p.contextCount >= maxContextLines || p.contextLines.Len() >= maxContextBytes {
		// Check if this line ends the error
		if isErrorBoundary(line) {
			return false
		}
		return true // Continue but don't accumulate
	}

	// Empty line might signal end of error
	if strings.TrimSpace(line) == "" {
		// If we've seen location, empty line likely ends the error
		if p.errorFile != "" {
			return false
		}
		// Before location, include empty lines
		p.addContextLine(line)
		return true
	}

	// Code line with pipe (|) - continue accumulating
	if rustCodeLinePattern.MatchString(line) {
		p.addContextLine(line)
		return true
	}

	// Caret/underline line - continue accumulating
	if rustCaretPattern.MatchString(line) {
		p.addContextLine(line)
		return true
	}

	// Note/help lines - extract content and continue
	if match := rustNotePattern.FindStringSubmatch(line); match != nil {
		noteType := match[1]
		noteMsg := match[2]

		switch noteType {
		case "note":
			p.notes = append(p.notes, noteMsg)
			// Check for Clippy lint code in note
			if lintMatch := rustClippyLintPattern.FindStringSubmatch(noteMsg); lintMatch != nil {
				p.clippyLint = lintMatch[1]
			}
		case "help":
			p.helps = append(p.helps, noteMsg)
		}
		p.addContextLine(line)
		return true
	}

	// Location arrow for secondary spans
	if rustLocationPattern.MatchString(line) {
		p.addContextLine(line)
		return true
	}

	// New error/warning header ends current error
	if rustErrorHeaderPattern.MatchString(line) {
		return false
	}

	// Noise patterns end the error context
	if p.IsNoise(line) {
		return false
	}

	// Other lines - if we have a file location, probably end of error
	if p.errorFile != "" {
		return false
	}

	// Otherwise include and continue
	p.addContextLine(line)
	return true
}

// isErrorBoundary checks if a line signals the end of an error block.
func isErrorBoundary(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	if rustErrorHeaderPattern.MatchString(line) {
		return true
	}
	return false
}

// FinishMultiLine finalizes the current multi-line error.
func (p *Parser) FinishMultiLine(ctx *parser.ParseContext) *errors.ExtractedError {
	if !p.inError {
		return nil
	}

	err := p.buildError(ctx)
	p.Reset()
	return err
}

// Reset clears any accumulated multi-line state.
func (p *Parser) Reset() {
	p.inError = false
	p.errorLevel = ""
	p.errorCode = ""
	p.errorMessage = ""
	p.errorFile = ""
	p.errorLine = 0
	p.errorColumn = 0
	p.contextLines.Reset()
	p.contextCount = 0
	p.notes = nil
	p.helps = nil
	p.clippyLint = ""
}

// Ensure Parser implements parser.ToolParser
var _ parser.ToolParser = (*Parser)(nil)
