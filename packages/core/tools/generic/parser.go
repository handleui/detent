package generic

import (
	"regexp"
	"strings"

	"github.com/handleui/detent/packages/core/errors"
	"github.com/handleui/detent/packages/core/tools/parser"
)

const (
	parserID       = "generic"
	parserPriority = 10
)

// Parser implements parser.ToolParser as a fallback for unrecognized error formats.
// It matches lines containing common error indicators and flags them for Sentry reporting.
//
// IMPORTANT: This parser is VERY STRICT to avoid false positives in Sentry reports.
// Only genuinely unrecognized error patterns should be flagged. When in doubt, skip it.
//
// Thread Safety: Parser is stateless and thread-safe. Multiple goroutines can safely
// share a single Parser instance. This parser does not mutate ParseContext fields
// (only reads WorkflowContext for cloning into extracted errors).
type Parser struct{}

// NewParser creates a new generic fallback parser instance.
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

// Patterns for strict error detection.
// CRITICAL: These must be very strict to avoid false positives in Sentry.
var (
	// actualErrorPatterns match lines that look like REAL errors (high confidence).
	// These patterns require strong structural signals, not just keyword presence.
	actualErrorPatterns = []*regexp.Regexp{
		// Must start with "Error:" or "error:" (case-insensitive)
		regexp.MustCompile(`(?i)^error:\s+\S`),
		// Must have format: "FATAL:" or "Fatal:" at start
		regexp.MustCompile(`(?i)^fatal:\s+\S`),
		// Structured log format: [ERROR] message or [FATAL] message
		regexp.MustCompile(`(?i)^\s*\[(error|fatal|fail)\]\s*:\s*\S`),
		// Build/compile/test failure with clear structure
		regexp.MustCompile(`(?i)^(build|compilation|compile)\s+failed\s*$`),
		// Non-zero exit codes (must be unambiguous)
		regexp.MustCompile(`(?i)^exit\s+(code|status)\s*[1-9]\d*\s*$`),
		regexp.MustCompile(`(?i)exited\s+with\s+(code\s+)?[1-9]\d*\s*$`),
		// Permission/access errors (unambiguous)
		regexp.MustCompile(`(?i)^permission\s+denied`),
		regexp.MustCompile(`(?i)^access\s+denied`),
		// Command not found (unambiguous)
		regexp.MustCompile(`(?i)^(bash:\s*)?command\s+not\s+found`),
		// File/directory not found (unambiguous format)
		regexp.MustCompile(`(?i)^no\s+such\s+file\s+or\s+directory`),
		// Segfault/crash (unambiguous)
		regexp.MustCompile(`(?i)^segmentation\s+fault`),
		regexp.MustCompile(`(?i)^killed\s*$`),
		regexp.MustCompile(`(?i)^out\s+of\s+memory`),
	}

	// noisePatterns match lines that should NEVER be flagged as errors.
	// These are common CI/CD output patterns that contain error keywords but aren't errors.
	noisePatterns = []*regexp.Regexp{
		// === CODE AND COMMENTS ===
		// NOTE: Removed "--" (SQL comment) as it conflicts with Go test "--- FAIL:" output
		regexp.MustCompile(`(?i)^\s*(#|//|/\*|\*)`), // Comments (excluding SQL --)
		regexp.MustCompile(`(?i)error.*handler`),       // Error handler code
		regexp.MustCompile(`(?i)on_?error`),            // Error callbacks
		regexp.MustCompile(`(?i)error_?(code|type|msg|message|class|kind)`), // Error variables
		regexp.MustCompile(`(?i)if.*error`),                                 // Conditional error handling
		regexp.MustCompile(`(?i)catch.*error`),                              // Try-catch
		regexp.MustCompile(`(?i)return.*error`),                             // Return error
		regexp.MustCompile(`(?i)handle.*error`),                             // Handle error
		regexp.MustCompile(`(?i)throw.*error`),                              // Throw error
		regexp.MustCompile(`(?i)raise.*error`),                              // Python raise
		regexp.MustCompile(`(?i)new\s+error`),                               // new Error()
		regexp.MustCompile(`(?i)^[A-Z_]+_ERROR\s*=`),                        // Constant definitions
		regexp.MustCompile(`(?i)\.error\s*[=(]`),                            // .error property/method

		// === SUCCESS INDICATORS ===
		regexp.MustCompile(`(?i)✓|✔|✅`),                              // Success checkmarks
		regexp.MustCompile(`(?i)(passed|success|succeeded|ok)\s*$`),   // Success endings
		regexp.MustCompile(`(?i)^(pass|ok|success)`),                  // Success starts
		regexp.MustCompile(`(?i)0\s+(error|failure)s?\s*(found)?`),    // "0 errors found"
		regexp.MustCompile(`(?i)no\s+(error|failure)s?\s*(found)?`),   // "no errors found"
		regexp.MustCompile(`(?i)completed\s+(successfully|with\s+exit\s+code\s+0)`), // Success completion
		regexp.MustCompile(`(?i)process\s+completed\s+with\s+exit\s+code`),          // GitHub Actions process completion (even non-zero)
		regexp.MustCompile(`(?i)build\s+succeeded`),                                 // Build success
		regexp.MustCompile(`(?i)all\s+tests?\s+passed`),                             // Test success

		// === PROGRESS/DOWNLOAD/INSTALL MESSAGES ===
		regexp.MustCompile(`(?i)^(downloading|fetching|loading|installing|extracting)`),
		regexp.MustCompile(`(?i)^(pulling|pushing|uploading|cloning|checking\s+out)`),
		regexp.MustCompile(`(?i)(already|successfully)\s+(installed|downloaded|cached)`),
		regexp.MustCompile(`(?i)using\s+cached`),
		regexp.MustCompile(`(?i)cache\s+(hit|restored|saved)`),
		regexp.MustCompile(`(?i)from\s+cache`),
		regexp.MustCompile(`(?i)^resolving\s+`),
		regexp.MustCompile(`(?i)^(npm|yarn|pnpm)\s+(warn|notice|info)`),
		regexp.MustCompile(`(?i)^\d+\s*packages?\s+`), // npm package counts

		// === RETRY/RECOVERY MESSAGES ===
		regexp.MustCompile(`(?i)retry(ing)?\s+`),
		regexp.MustCompile(`(?i)attempt\s+\d+`),
		regexp.MustCompile(`(?i)will\s+retry`),
		regexp.MustCompile(`(?i)retrying\s+in`),
		regexp.MustCompile(`(?i)connection\s+reset.*retry`),

		// === GITHUB ACTIONS WORKFLOW COMMANDS ===
		regexp.MustCompile(`(?i)^::(debug|notice|warning|error|group|endgroup|set-output|save-state|add-mask)::`),
		regexp.MustCompile(`(?i)^##\[`), // GitHub Actions annotation format

		// === TEST FRAMEWORK OUTPUT ===
		regexp.MustCompile(`(?i)^(=== RUN|=== PAUSE|=== CONT|--- PASS|--- SKIP)`), // Go test
		regexp.MustCompile(`(?i)^(PASS|FAIL)\s+\S+\s+[\d.]+s`),                    // Go test summary
		regexp.MustCompile(`(?i)^ok\s+\S+\s+[\d.]+s`),                             // Go test package pass
		regexp.MustCompile(`(?i)^\?\s+\S+\s+\[no test files\]`),                   // Go no tests
		regexp.MustCompile(`(?i)^(it|describe|test)\s*\(`),                        // Jest/Mocha/Vitest
		regexp.MustCompile(`(?i)^(PASSED|FAILED)\s*\(`),                           // pytest
		regexp.MustCompile(`(?i)^\d+\s+(passing|pending|failing)\s*$`),            // Mocha summary

		// === COVERAGE/ANALYSIS TOOLS ===
		regexp.MustCompile(`(?i)^coverage:`),
		regexp.MustCompile(`(?i)codecov`),
		regexp.MustCompile(`(?i)^(uploading|uploaded)\s+.*coverage`),
		regexp.MustCompile(`(?i)\d+%\s+coverage`),

		// === DOCKER/CONTAINER OUTPUT ===
		regexp.MustCompile(`(?i)^(step|layer)\s+\d+/\d+`),  // Docker build steps
		regexp.MustCompile(`(?i)^#\d+\s+`),                 // Docker buildx output
		regexp.MustCompile(`(?i)^(pulling|pushed|built)`),  // Docker operations
		regexp.MustCompile(`(?i)^using\s+docker`),          // Docker info
		regexp.MustCompile(`(?i)image\s+(pulled|built|tagged)`), // Docker image ops
		regexp.MustCompile(`(?i)^sha256:[a-f0-9]+`),        // Docker digests

		// === STACK TRACES (belong to parent error, not separate) ===
		regexp.MustCompile(`(?i)^\s+at\s+`),                     // JS/TS stack traces
		regexp.MustCompile(`(?i)^\s+File\s+".+",\s+line\s+\d+`), // Python traceback
		regexp.MustCompile(`(?i)^goroutine\s+\d+\s+\[`),         // Go goroutine header
		regexp.MustCompile(`^\s+\S+\.go:\d+`),                   // Go stack frame file
		regexp.MustCompile(`^\S+\([^)]*\)\s*$`),                 // Go stack frame function
		regexp.MustCompile(`(?i)^traceback\s+\(most recent`),    // Python traceback header
		regexp.MustCompile(`^\s{4,}`),                           // Heavy indentation (usually stack trace continuation)

		// === VERSION/INFO OUTPUT ===
		regexp.MustCompile(`(?i)^(version|v)\s*[\d.]+`),
		regexp.MustCompile(`(?i)^(node|npm|yarn|go|python|ruby|java)\s+v?[\d.]+`),
		regexp.MustCompile(`(?i)^(using|running)\s+(node|npm|go|python|ruby|java)`),

		// === CI PLATFORM NOISE ===
		regexp.MustCompile(`(?i)^(run|running)\s+`),                 // GitHub Actions "Run" lines
		regexp.MustCompile(`(?i)^\[command\]`),                      // Azure DevOps
		regexp.MustCompile(`(?i)^(starting|finished)\s+`),           // Generic CI
		regexp.MustCompile(`(?i)^(job|step|stage)\s+'\S+'`),         // CI job/step names
		regexp.MustCompile(`(?i)^(added|removed|changed)\s+\d+\s+`), // Git diff summary
		regexp.MustCompile(`(?i)^(time|duration|elapsed)`),          // Timing info

		// === LINTER/TOOL STATUS (not errors themselves) ===
		regexp.MustCompile(`(?i)^(running|checking|analyzing|linting)\s+`),
		regexp.MustCompile(`(?i)^(issues|problems|warnings):\s*\d+`),
		regexp.MustCompile(`(?i)^found\s+\d+\s+(issue|problem|warning|error)s?`),
		regexp.MustCompile(`(?i)^(level|severity)=`), // golangci-lint debug

		// === EMPTY/WHITESPACE/DECORATIVE ===
		regexp.MustCompile(`^\s*$`),            // Empty lines
		regexp.MustCompile(`^[-=_*]{3,}\s*$`),  // Horizontal rules
		regexp.MustCompile(`^[│├└┌┐┘┤┴┬┼]+$`), // Box drawing characters

		// === URLs AND PATHS (often contain "error" in path names) ===
		regexp.MustCompile(`(?i)https?://\S+error`),
		regexp.MustCompile(`(?i)/errors?/`),             // Path containing /error/ or /errors/
		regexp.MustCompile(`(?i)error\.(js|ts|go|py)`), // Error module files
	}
)

// CanParse implements parser.ToolParser.
func (p *Parser) CanParse(line string, _ *parser.ParseContext) float64 {
	trimmed := strings.TrimSpace(line)

	// Skip empty or very short lines (minimum 10 chars for meaningful error message)
	if len(trimmed) < 10 {
		return 0
	}

	// Skip very long lines (likely minified code or data dumps)
	if len(trimmed) > 500 {
		return 0
	}

	// FIRST: Check if this is noise that should NEVER be flagged
	for _, pattern := range noisePatterns {
		if pattern.MatchString(line) {
			return 0
		}
	}

	// SECOND: Only match lines that look like REAL errors with strong structural signals
	// This is intentionally very strict - we'd rather miss some errors than spam Sentry
	for _, pattern := range actualErrorPatterns {
		if pattern.MatchString(line) {
			return 0.15 // Low score - only wins if no specific parser claims it
		}
	}

	// DO NOT match based on generic error keywords alone (too many false positives)
	// Lines like "some error occurred" or "the operation failed" without structure
	// are too ambiguous to flag as unknown patterns for Sentry.
	return 0
}

// Parse implements parser.ToolParser.
func (p *Parser) Parse(line string, ctx *parser.ParseContext) *errors.ExtractedError {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 5 {
		return nil
	}

	// Create an error with the unknown pattern flag
	err := &errors.ExtractedError{
		Message:        trimmed,
		Severity:       "error",
		Raw:            line,
		Category:       errors.CategoryUnknown,
		Source:         errors.SourceGeneric,
		UnknownPattern: true, // Flag for Sentry reporting
	}

	ctx.ApplyWorkflowContext(err)

	return err
}

// IsNoise implements parser.ToolParser.
func (p *Parser) IsNoise(line string) bool {
	for _, pattern := range noisePatterns {
		if pattern.MatchString(line) {
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

// NoisePatterns returns the generic parser's noise detection patterns for registry optimization.
// These patterns cover common CI/CD output that isn't specific to any tool.
func (p *Parser) NoisePatterns() parser.NoisePatterns {
	return parser.NoisePatterns{
		FastPrefixes: []string{
			// GitHub Actions workflow commands
			"::debug::",    // GitHub Actions debug
			"::notice::",   // GitHub Actions notice
			"::warning::",  // GitHub Actions warning
			"::error::",    // GitHub Actions error (annotations, handled separately)
			"::group::",    // GitHub Actions group
			"::endgroup::", // GitHub Actions endgroup
			"##[",          // GitHub Actions annotation format
		},
		FastContains: []string{
			// Success indicators
			"all files pass",
			"build succeeded",
			"all tests passed",
			"completed successfully",
			// Cache indicators
			"using cached",
			"cache hit",
			"cache restored",
			"from cache",
			// Already installed/downloaded
			"already installed",
			"already downloaded",
			"successfully installed",
			"successfully downloaded",
		},
		Regex: noisePatterns,
	}
}

// Ensure Parser implements parser.ToolParser
var _ parser.ToolParser = (*Parser)(nil)

// Ensure Parser implements parser.NoisePatternProvider
var _ parser.NoisePatternProvider = (*Parser)(nil)
