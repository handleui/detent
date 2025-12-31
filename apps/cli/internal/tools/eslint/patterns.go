package eslint

import (
	"regexp"
)

// ESLint error patterns for various output formats.
// ESLint supports multiple formatters; we focus on the most common CI outputs:
//   - stylish (default): Multi-line with file path header and indented errors
//   - compact: Single-line "file: line N, col N, Severity - Message (rule)"
//   - unix: Single-line "file:line:col: message [severity/rule]"
//
// Note: JSON format is not parsed here (handled by structured output parsers).
// GitHub Actions annotations (::error::) are handled by the generic parser.

var (
	// stylishErrorPattern matches the indented error lines in stylish format (default).
	// Format: "  8:11  error  Message text  rule-name"
	// The pattern handles variable spacing between columns.
	//
	// Groups:
	//   1: line number
	//   2: column number
	//   3: severity ("error" or "warning")
	//   4: message and rule ID (parsed separately by ruleIDPattern)
	stylishErrorPattern = regexp.MustCompile(`^\s+(\d+):(\d+)\s+(error|warning)\s+(.+)$`)

	// stylishFilePattern matches file path headers in stylish format.
	// ESLint outputs the file path on its own line, followed by indented errors.
	// Example: "/path/to/file.js" or "src/components/Button.tsx"
	//
	// Matches common JS/TS file extensions including ESM variants and frameworks.
	// IMPORTANT: Must NOT contain colons to avoid matching Go/Unix error format.
	// Group 1: file path
	stylishFilePattern = regexp.MustCompile(`^([^\s:]+\.(?:js|jsx|ts|tsx|mjs|cjs|mts|cts|vue|svelte|astro))$`)

	// compactPattern matches ESLint compact format output (single-line).
	// Format: "/path/to/file.js: line 8, col 11, Error - Message (rule-id)"
	//
	// Groups:
	//   1: file path
	//   2: line number
	//   3: column number
	//   4: severity ("Error" or "Warning")
	//   5: message
	//   6: rule ID (optional, in parentheses)
	compactPattern = regexp.MustCompile(`^([^\s:]+\.(?:js|jsx|ts|tsx|mjs|cjs|mts|cts|vue|svelte|astro)):\s*line\s+(\d+),\s*col\s+(\d+),\s*(Error|Warning)\s*-\s*(.+?)(?:\s+\(([^)]+)\))?\s*$`)

	// unixPattern matches ESLint unix format output (single-line, colon-separated).
	// Format: "/path/to/file.js:8:11: message [error/rule-id]"
	// This format is similar to GCC/Go errors but has the [severity/rule] suffix.
	//
	// IMPORTANT: The [severity/rule] suffix is REQUIRED to distinguish from Go errors.
	// Without this suffix, we cannot reliably distinguish ESLint unix from Go errors.
	//
	// Groups:
	//   1: file path
	//   2: line number
	//   3: column number
	//   4: message
	//   5: severity ("error" or "warning")
	//   6: rule ID
	unixPattern = regexp.MustCompile(`^([^\s:]+\.(?:js|jsx|ts|tsx|mjs|cjs|mts|cts|vue|svelte|astro)):(\d+):(\d+):\s*(.+?)\s+\[(error|warning)/([^\]]+)\]\s*$`)

	// ruleIDPattern extracts the rule ID from the end of the message in stylish format.
	// ESLint rules can be:
	//   - Simple: "no-var", "semi", "quotes", "eqeqeq"
	//   - Scoped: "react/no-unsafe", "import/no-unresolved"
	//   - Namespaced: "@typescript-eslint/no-unused-vars", "@next/next/no-img-element"
	//   - With numbers: "react/jsx-max-depth", "max-lines-per-function"
	//   - With underscores: "camelcase", "no_underscore_dangle" (rare but valid)
	//
	// Pattern matches the last space-separated token that looks like a rule ID.
	// Must end with a word character (not punctuation) to avoid matching sentences.
	// Group 1: message (everything before the rule)
	// Group 2: rule ID
	ruleIDPattern = regexp.MustCompile(`^(.+?)\s+((?:@[\w-]+/)?[\w-]+(?:/[\w-]+)*)\s*$`)

	// summaryPattern matches the ESLint summary line at the end.
	// Examples: "✖ 2 problems (1 error, 1 warning)"
	//           "✖ 5 problems (5 errors, 0 warnings)"
	summaryPattern = regexp.MustCompile(`[✖X]\s+\d+\s+problems?\s+\(\d+\s+errors?,\s+\d+\s+warnings?\)`)

	// noisePatterns are lines that should be skipped as ESLint-specific noise
	noisePatterns = []*regexp.Regexp{
		// Summary and count lines
		regexp.MustCompile(`[✖X]\s+\d+\s+problems?`),          // Summary: ✖ N problems
		regexp.MustCompile(`^\d+\s+errors?$`),                // Count: N errors
		regexp.MustCompile(`^\d+\s+warnings?$`),              // Count: N warnings
		regexp.MustCompile(`^\d+\s+errors?\s+and\s+\d+`),     // Count: N errors and M warnings
		regexp.MustCompile(`^✓\s+`),                          // Success: ✓ message
		regexp.MustCompile(`^All files pass linting`),        // Success message

		// Empty lines and decorative
		regexp.MustCompile(`^\s*$`), // Empty lines

		// Process/running messages
		regexp.MustCompile(`(?i)^(running|linting)\s+eslint`), // Running ESLint...
		regexp.MustCompile(`(?i)^eslint\s+--`),                // ESLint command echo
		regexp.MustCompile(`(?i)^Done in\s+`),                 // Done in Xs

		// Fixable hints
		regexp.MustCompile(`(?i)potentially\s+fixable`), // Fixable hint
		regexp.MustCompile(`(?i)--fix\s+option`),        // --fix suggestion
	}
)
