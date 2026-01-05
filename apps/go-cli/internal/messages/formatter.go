package messages

import "strings"

// MessageFormatter formats error messages for different contexts.
// This interface allows for extensibility in message formatting,
// enabling future enhancements like localization or AI-friendly descriptions.
type MessageFormatter interface {
	// Format takes raw pattern matches and produces a formatted message.
	// The matches slice contains the captured groups from regex patterns.
	// The context provides additional information for contextual formatting.
	Format(matches []string, context *MessageContext) string
}

// MessageContext provides additional information for message formatting.
// It contains metadata about the error source, language, tool, and location.
type MessageContext struct {
	Language   string // e.g., "go", "typescript", "python", "rust"
	Tool       string // e.g., "eslint", "rustc", "mypy", "tsc"
	Severity   string // "error", "warning", etc.
	FilePath   string // File where the error occurred
	LineNumber int    // Line number where the error occurred
}

// DefaultFormatter is the standard message formatter.
// It performs basic message extraction and trimming.
type DefaultFormatter struct{}

// Format implements MessageFormatter for DefaultFormatter.
// It extracts the last match (typically the message group) and trims whitespace.
func (f *DefaultFormatter) Format(matches []string, ctx *MessageContext) string {
	if len(matches) > 0 {
		return strings.TrimSpace(matches[len(matches)-1])
	}
	return ""
}

// NewDefaultFormatter creates a new instance of DefaultFormatter.
func NewDefaultFormatter() *DefaultFormatter {
	return &DefaultFormatter{}
}
