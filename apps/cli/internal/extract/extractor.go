package extract

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/detent/cli/internal/ci"
	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/sentry"
	"github.com/detent/cli/internal/tools"
	"github.com/detent/cli/internal/tools/parser"
)

const (
	maxLineLength               = 65536 // 64KB per line - prevents ReDoS on extremely long lines
	maxDeduplicationSize        = 10000 // Maximum deduplicated errors to prevent unbounded map growth
	maxUnknownPatternsToReport  = 10    // Limit unknown pattern reports to prevent Sentry spam
	maxUnknownPatternLineLength = 500   // Truncate long lines in Sentry reports
)

// sanitizer holds a regex pattern and its replacement for sensitive data scrubbing.
type sanitizer struct {
	pattern     *regexp.Regexp
	replacement string
}

// sensitivePatterns are regex patterns that match sensitive data to be redacted.
// SECURITY: These patterns prevent leaking credentials, tokens, paths, and PII to Sentry.
var sensitivePatterns = []sanitizer{
	// API keys and tokens (various formats)
	{regexp.MustCompile(`(?i)(api[_-]?key|apikey|api[_-]?secret|secret[_-]?key)\s*[:=]\s*['"]?[A-Za-z0-9_\-]{8,}['"]?`), "$1=[REDACTED]"},
	{regexp.MustCompile(`(?i)(token|bearer|auth|password|passwd|pwd|secret)\s*[:=]\s*['"]?[A-Za-z0-9_\-\.]{8,}['"]?`), "$1=[REDACTED]"},

	// GitHub/GitLab/AWS/GCP tokens (specific formats)
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`), "[GITHUB_TOKEN]"},
	{regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`), "[GITHUB_OAUTH_TOKEN]"},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`), "[GITHUB_PAT]"},
	{regexp.MustCompile(`glpat-[A-Za-z0-9\-]{20,}`), "[GITLAB_PAT]"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "[AWS_ACCESS_KEY]"},
	{regexp.MustCompile(`(?i)aws[_-]?secret[_-]?access[_-]?key\s*[:=]\s*['"]?[A-Za-z0-9/+=]{40}['"]?`), "aws_secret_access_key=[REDACTED]"},

	// NPM tokens
	{regexp.MustCompile(`npm_[A-Za-z0-9]{36,}`), "[NPM_TOKEN]"},

	// JWT tokens
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`), "[JWT_TOKEN]"},

	// Email addresses
	{regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), "[EMAIL]"},

	// IP addresses (IPv4)
	{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), "[IP_ADDR]"},

	// Home directory paths (Unix and Windows)
	{regexp.MustCompile(`/home/[^/\s]+`), "/home/[USER]"},
	{regexp.MustCompile(`/Users/[^/\s]+`), "/Users/[USER]"},
	{regexp.MustCompile(`(?i)C:\\Users\\[^\\\s]+`), "C:\\Users\\[USER]"},

	// Generic long hex strings (often secrets or hashes - keep short ones for error codes)
	{regexp.MustCompile(`[a-fA-F0-9]{32,}`), "[HEX_STRING]"},

	// Base64 encoded strings that look like secrets (long enough to be meaningful)
	{regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`), "[BASE64_STRING]"},

	// Connection strings
	{regexp.MustCompile(`(?i)(mongodb|postgres|mysql|redis|amqp)://\S+`), "$1://[CONNECTION_STRING]"},

	// URLs with credentials
	{regexp.MustCompile(`https?://[^:]+:[^@]+@`), "https://[CREDENTIALS]@"},
}

// errKey is used for deduplication
type errKey struct {
	message string
	file    string
	line    int
}

// Extractor uses the tool registry to extract errors from CI output.
// It delegates to tool-specific parsers for precise pattern matching
// and falls back to the legacy extractor for patterns not yet migrated.
type Extractor struct {
	registry           *tools.Registry
	legacyExtractor    *errors.Extractor
	currentWorkflowCtx *errors.WorkflowContext
}

// NewExtractor creates a new registry-based extractor with legacy fallback.
func NewExtractor(registry *tools.Registry) *Extractor {
	return &Extractor{
		registry:        registry,
		legacyExtractor: errors.NewExtractor(),
	}
}

// Extract parses CI output using the tool registry for error extraction.
// It uses tool-specific parsers (Go, TypeScript, etc.) for precise pattern matching
// and falls back to legacy extraction for patterns not yet migrated.
//
// The extraction strategy is:
// 1. Try tool-specific parsers first (better multi-line handling, more precise)
// 2. Fall back to legacy extractor for patterns not yet migrated (Python, Rust, ESLint, etc.)
// 3. Deduplicate results from both extractors
func (e *Extractor) Extract(output string, ctxParser ci.ContextParser) []*errors.ExtractedError {
	var extracted []*errors.ExtractedError
	seen := make(map[errKey]struct{}, 256)

	// Create parse context for tool parsers
	parseCtx := &parser.ParseContext{
		WorkflowContext: e.currentWorkflowCtx,
	}

	// Track active multi-line parser
	var activeParser tools.ToolParser

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > maxLineLength {
			continue // Skip extremely long lines to prevent ReDoS
		}

		// Use the context parser to extract CI context and clean the line
		ctx, cleanLine, skip := ctxParser.ParseLine(line)
		if skip {
			continue
		}

		// Convert CI context to workflow context
		if ctx != nil && ctx.Job != "" {
			e.currentWorkflowCtx = &errors.WorkflowContext{
				Job:  ctx.Job,
				Step: ctx.Step,
			}
			parseCtx.WorkflowContext = e.currentWorkflowCtx
			parseCtx.Job = ctx.Job
			parseCtx.Step = ctx.Step
		}

		// Check if registry considers this line as noise
		if e.registry.IsNoise(cleanLine) {
			continue
		}

		var found *errors.ExtractedError

		// If we have an active multi-line parser, try to continue
		if activeParser != nil && activeParser.SupportsMultiLine() {
			if activeParser.ContinueMultiLine(cleanLine, parseCtx) {
				continue // Line consumed by multi-line parser
			}
			// Multi-line sequence ended, finalize it
			found = activeParser.FinishMultiLine(parseCtx)
			activeParser = nil
		}

		// Try to find a parser for this line
		if found == nil {
			p := e.registry.FindParser(cleanLine, parseCtx)
			if p != nil {
				found = p.Parse(cleanLine, parseCtx)

				// Check if this parser starts a multi-line sequence
				if found == nil && p.SupportsMultiLine() {
					// Parser may have started accumulating but not returned an error yet
					activeParser = p
				}
			}
		}

		if found != nil {
			if found.WorkflowContext == nil && e.currentWorkflowCtx != nil {
				found.WorkflowContext = e.currentWorkflowCtx.Clone()
			}

			if len(seen) >= maxDeduplicationSize {
				extracted = append(extracted, found)
				continue
			}

			key := errKey{found.Message, found.File, found.Line}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				extracted = append(extracted, found)
			}
		}
	}

	// Finalize any pending multi-line parser
	if activeParser != nil && activeParser.SupportsMultiLine() {
		if found := activeParser.FinishMultiLine(parseCtx); found != nil {
			if found.WorkflowContext == nil && e.currentWorkflowCtx != nil {
				found.WorkflowContext = e.currentWorkflowCtx.Clone()
			}
			key := errKey{found.Message, found.File, found.Line}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				extracted = append(extracted, found)
			}
		}
	}

	// Use legacy extractor for patterns not yet migrated to tool parsers
	// (Python, Rust, ESLint, Node.js, Docker, metadata patterns)
	legacyErrors := e.legacyExtractor.ExtractWithContext(output, ctxParser)
	for _, err := range legacyErrors {
		// Skip Go and TypeScript errors - they're handled by tool parsers
		if err.Source == errors.SourceGo || err.Source == errors.SourceGoTest || err.Source == errors.SourceTypeScript {
			continue
		}

		if len(seen) >= maxDeduplicationSize {
			extracted = append(extracted, err)
			continue
		}

		key := errKey{err.Message, err.File, err.Line}
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			extracted = append(extracted, err)
		}
	}

	return extracted
}

// Reset clears any accumulated state in the extractor and all parsers.
func (e *Extractor) Reset() {
	e.currentWorkflowCtx = nil
	e.registry.ResetAll()
}

// ReportUnknownPatterns sends unknown error patterns to Sentry for later analysis.
// This helps identify new error formats that should be added as dedicated parsers.
// Note: Raw patterns are sanitized by Sentry's scrubPII before transmission.
func ReportUnknownPatterns(errs []*errors.ExtractedError) {
	var unknownPatterns []string
	for _, err := range errs {
		if err.UnknownPattern && len(unknownPatterns) < maxUnknownPatternsToReport {
			raw := err.Raw
			if len(raw) > maxUnknownPatternLineLength {
				raw = raw[:maxUnknownPatternLineLength] + "..."
			}
			// Sanitize the pattern to remove potential PII before adding to list
			// The pattern structure is what matters for creating new parsers, not the actual values
			sanitized := sanitizePatternForSentry(raw)
			unknownPatterns = append(unknownPatterns, sanitized)
		}
	}

	if len(unknownPatterns) == 0 {
		return
	}

	sentry.AddBreadcrumb("extract", "unknown error patterns detected")
	// Only set a single tag with the count, not the actual patterns
	// This avoids leaking any sensitive information in tags
	sentry.CaptureMessage("Unknown error patterns detected: " + unknownPatterns[0])
}

// sanitizePatternForSentry removes potentially sensitive information from error patterns.
// It preserves the structure of the error (file extensions, line/column numbers, keywords)
// while removing actual file paths, credentials, and other PII.
//
// SECURITY: This function is critical for preventing sensitive data from being sent to Sentry.
// When in doubt, redact more rather than less.
func sanitizePatternForSentry(pattern string) string {
	// Limit length to prevent excessive data
	if len(pattern) > maxUnknownPatternLineLength {
		pattern = pattern[:maxUnknownPatternLineLength] + "..."
	}

	result := pattern

	// Replace sensitive patterns using regex
	for _, sanitizer := range sensitivePatterns {
		result = sanitizer.pattern.ReplaceAllString(result, sanitizer.replacement)
	}

	// Replace file paths with placeholders while keeping the extension
	// This preserves the pattern structure for parser development
	for _, ext := range []string{".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".c", ".cpp", ".h"} {
		// Match paths ending with this extension
		idx := 0
		for {
			extIdx := indexOf(result[idx:], ext)
			if extIdx == -1 {
				break
			}
			extIdx += idx
			// Find the start of the path (first space or start of string before the extension)
			pathStart := extIdx
			for pathStart > 0 && result[pathStart-1] != ' ' && result[pathStart-1] != '\t' && result[pathStart-1] != '"' && result[pathStart-1] != '\'' {
				pathStart--
			}
			if pathStart < extIdx {
				result = result[:pathStart] + "[path]" + ext + result[extIdx+len(ext):]
			}
			idx = pathStart + len("[path]") + len(ext)
			if idx >= len(result) {
				break
			}
		}
	}

	return result
}

// indexOf returns the index of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
