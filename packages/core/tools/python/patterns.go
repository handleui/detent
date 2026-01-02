package python

import (
	"regexp"
	"unicode/utf8"
)

// Resource limits for multi-line parsing to prevent memory exhaustion.
const (
	maxTracebackFrames = 100
	maxTracebackBytes  = 256 * 1024 // 256KB
	maxMessageLength   = 2000
)

// TruncateMessage safely truncates a message to maxMessageLength bytes,
// ensuring valid UTF-8 output by not splitting multi-byte characters.
func TruncateMessage(msg string) string {
	if len(msg) <= maxMessageLength {
		return msg
	}

	// Find the last valid rune boundary before maxMessageLength
	truncated := msg[:maxMessageLength]

	// Walk backwards until we find a valid UTF-8 string
	for truncated != "" && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}

	return truncated
}

// Python-specific regex patterns for error extraction.
// Patterns use lazy quantifiers (.+?) where possible to prevent ReDoS.
var (
	// --- Standard Traceback Patterns ---

	// tracebackStartPattern matches the start of a Python traceback.
	// Example: "Traceback (most recent call last):"
	tracebackStartPattern = regexp.MustCompile(`^Traceback \(most recent call last\):$`)

	// tracebackFilePattern matches file location lines in tracebacks.
	// Example: '  File "/path/to/file.py", line 42, in function_name'
	// Example: '  File "/path/to/file.py", line 42'
	// Group 1: file path
	// Group 2: line number
	// Group 3: function name (optional)
	tracebackFilePattern = regexp.MustCompile(`^\s+File "([^"]+)", line (\d+)(?:, in (.+))?$`)

	// tracebackCodePattern matches code lines in tracebacks (4+ space indented).
	// Example: "    some_code_here()"
	// Example: "        nested_code()"  (8 spaces for nested contexts)
	tracebackCodePattern = regexp.MustCompile(`^\s{4,}\S.*$`)

	// exceptionPattern matches exception lines at the end of tracebacks.
	// Example: "ValueError: message here"
	// Example: "ModuleNotFoundError: No module named 'foo'"
	// Group 1: exception type (e.g., ValueError, TypeError, ModuleNotFoundError)
	// Group 2: exception message
	exceptionPattern = regexp.MustCompile(`^([A-Z][a-zA-Z0-9]*(?:Error|Exception|Warning)): (.+)$`)

	// chainedExceptionPattern matches chained exception headers.
	// Example: "During handling of the above exception, another exception occurred:"
	// Example: "The above exception was the direct cause of the following exception:"
	chainedExceptionPattern = regexp.MustCompile(`^(?:During handling of the above exception|The above exception was the direct cause)`)

	// --- SyntaxError Patterns ---

	// syntaxErrorFilePattern matches SyntaxError file lines (without function).
	// Example: '  File "script.py", line 5'
	syntaxErrorFilePattern = regexp.MustCompile(`^\s+File "([^"]+)", line (\d+)\s*$`)

	// syntaxErrorCaretPattern matches the caret line indicating error position.
	// Example: "                   ^"
	// Example: "    ^^^"
	syntaxErrorCaretPattern = regexp.MustCompile(`^\s*\^+\s*$`)

	// syntaxErrorPattern matches SyntaxError and related compile errors.
	// Example: "SyntaxError: invalid syntax"
	// Example: "IndentationError: unexpected indent"
	// Group 1: error type
	// Group 2: message
	syntaxErrorPattern = regexp.MustCompile(`^(SyntaxError|IndentationError|TabError): (.+)$`)

	// --- pytest Patterns ---

	// pytestFailedPattern matches pytest FAILED lines.
	// Example: "FAILED tests/test_foo.py::test_bar - AssertionError: assert 1 == 2"
	// Example: "FAILED tests/test_foo.py::TestClass::test_method[param1-param2] - ValueError"
	// Group 1: file path
	// Group 2: test name (may include class and parameters)
	// Group 3: error message
	pytestFailedPattern = regexp.MustCompile(`^FAILED\s+([^\s:]+)::(\S+)\s+-\s+(.+)$`)

	// pytestErrorPattern matches pytest ERROR collection lines.
	// Example: "ERROR tests/test_foo.py - ModuleNotFoundError: No module named 'foo'"
	// Group 1: file path
	// Group 2: error message
	pytestErrorPattern = regexp.MustCompile(`^ERROR\s+(\S+)\s+-\s+(.+)$`)

	// --- mypy Patterns ---

	// mypyPatternAlt matches mypy output lines.
	// Example: "app/main.py:42: error: Argument 1 to \"foo\" has incompatible type \"str\"; expected \"int\"  [arg-type]"
	// Example: "app/main.py:50: note: Revealed type is \"builtins.str\""
	// Group 1: file path
	// Group 2: line number
	// Group 3: severity (error, warning, note)
	// Group 4: message (rule ID extracted separately in parseMypy)
	mypyPatternAlt = regexp.MustCompile(`^([^\s:]+\.pyi?):(\d+): (error|warning|note): (.+)$`)

	// --- ruff/flake8 Patterns ---

	// ruffFlake8Pattern matches ruff and flake8 output.
	// Example: "app/main.py:42:10: E501 Line too long (120 > 88)"
	// Example: "app/main.py:50:1: F401 'os' imported but unused"
	// Group 1: file path
	// Group 2: line number
	// Group 3: column number
	// Group 4: error code (e.g., E501, F401)
	// Group 5: message
	ruffFlake8Pattern = regexp.MustCompile(`^([^\s:]+\.pyi?):(\d+):(\d+): ([A-Z]\d+) (.+)$`)

	// ruffFlake8NoColPattern matches ruff/flake8 output without column.
	// Some configurations don't show column numbers.
	// Group 1: file path
	// Group 2: line number
	// Group 3: error code
	// Group 4: message
	ruffFlake8NoColPattern = regexp.MustCompile(`^([^\s:]+\.pyi?):(\d+): ([A-Z]\d+) (.+)$`)

	// --- pylint Patterns ---

	// pylintPattern matches pylint output.
	// Example: "app/main.py:42:0: C0114: Missing module docstring (missing-module-docstring)"
	// Example: "app/main.py:50:4: E1101: Instance of 'Foo' has no 'bar' member (no-member)"
	// Group 1: file path
	// Group 2: line number
	// Group 3: column number
	// Group 4: code (e.g., C0114, E1101)
	// Group 5: message
	// Group 6: rule ID in parentheses (e.g., "missing-module-docstring")
	pylintPattern = regexp.MustCompile(`^([^\s:]+\.pyi?):(\d+):(\d+): ([RCWEF]\d+): (.+) \(([^)]+)\)$`)

	// --- Noise Patterns ---

	// noisePatterns are lines that should be skipped as noise.
	// IMPORTANT: These patterns should be Python-specific and not match errors from other languages.
	noisePatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*$`),                            // Empty/whitespace lines
		regexp.MustCompile(`^\.{4,}$`),                         // pytest progress dots (4+ to avoid matching ...)
		regexp.MustCompile(`^\d+ passed`),                      // pytest summary
		regexp.MustCompile(`^\d+ failed,`),                     // pytest summary (with comma to be specific)
		regexp.MustCompile(`^\d+ errors?$`),                    // pytest summary (at end of line)
		regexp.MustCompile(`^\d+ warnings?$`),                  // pytest summary (at end of line)
		regexp.MustCompile(`^\d+ skipped`),                     // pytest summary
		regexp.MustCompile(`^test session starts`),             // pytest header
		regexp.MustCompile(`^short test summary info`),         // pytest header
		regexp.MustCompile(`^warnings summary`),                // pytest header
		regexp.MustCompile(`^PASSED$`),                         // pytest passed indicator (exact match)
		regexp.MustCompile(`^SKIPPED$`),                        // pytest skipped indicator (exact match)
		regexp.MustCompile(`^platform (linux|darwin|win)`),     // pytest platform info (specific platforms)
		regexp.MustCompile(`^cachedir:`),                       // pytest cache info
		regexp.MustCompile(`^rootdir:`),                        // pytest root dir
		regexp.MustCompile(`^configfile:`),                     // pytest config
		regexp.MustCompile(`^plugins:`),                        // pytest plugins
		regexp.MustCompile(`^collecting`),                      // pytest collection
		regexp.MustCompile(`^collected\s+\d+`),                 // pytest collected tests
		regexp.MustCompile(`^ok\s+\(`),                         // unittest ok
		regexp.MustCompile(`^Ran\s+\d+\s+test`),                // unittest summary
		regexp.MustCompile(`^OK$`),                             // unittest pass (exact match)
		regexp.MustCompile(`^Success:`),                        // mypy success
		regexp.MustCompile(`^Found\s+\d+\s+errors? in`),        // mypy summary (more specific)
		regexp.MustCompile(`^Your code has been rated`),        // pylint rating
		regexp.MustCompile(`^All checks passed!`),              // ruff success
		regexp.MustCompile(`^\d+ files? (checked|scanned)`),    // ruff/flake8 summary
		regexp.MustCompile(`^Coverage`),                        // coverage output
		regexp.MustCompile(`^Name\s+Stmts\s+Miss`),             // coverage header
		regexp.MustCompile(`^TOTAL\s+`),                        // coverage total
		regexp.MustCompile(`^self = <`),                        // pytest self reference
		regexp.MustCompile(`^During handling of the above`),    // chained exception header (more specific)
		regexp.MustCompile(`^The above exception was`),         // chained exception header (more specific)
		regexp.MustCompile(`^.*\.py::.*PASSED`),                // pytest verbose passed (file.py::test PASSED)
		regexp.MustCompile(`^.*\.py::.*SKIPPED`),               // pytest verbose skipped
		regexp.MustCompile(`^\s+@pytest`),                      // pytest decorators
		regexp.MustCompile(`^\s+@fixture`),                     // pytest fixtures
		regexp.MustCompile(`^\s+@mark`),                        // pytest marks
		regexp.MustCompile(`^=+ warnings summary =+`),          // pytest warnings header
		regexp.MustCompile(`^=+ short test summary info =+`),   // pytest summary header
		regexp.MustCompile(`^=+ FAILURES =+`),                  // pytest failures header
		regexp.MustCompile(`^=+ ERRORS =+`),                    // pytest errors header
		regexp.MustCompile(`^_+ .+ _+$`),                       // pytest test name separators
		regexp.MustCompile(`^in \d+\.\d+s$`),                   // pytest timing (exact end)
		regexp.MustCompile(`^Rerun`),                           // pytest-rerunfailures
		regexp.MustCompile(`^E\s+assert\s+`),                   // pytest assertion detail
		regexp.MustCompile(`^E\s+\+`),                          // pytest diff plus
		regexp.MustCompile(`^E\s+-`),                           // pytest diff minus
		regexp.MustCompile(`^E\s+where`),                       // pytest where clause
		regexp.MustCompile(`^E\s+and`),                         // pytest and clause
		regexp.MustCompile(`^\s+\.{3}$`),                       // traceback continuation
		regexp.MustCompile(`^<frozen `),                        // frozen module paths
		regexp.MustCompile(`^\s+File "<`),                      // internal file refs like <stdin>
	}
)

// ruffFlake8SeverityCodes maps error code prefixes to severity.
// Based on flake8/ruff error code conventions.
var ruffFlake8SeverityCodes = map[string]string{
	// Fatal/syntax errors - always errors
	"E9": "error", // E9xx: Runtime/syntax errors

	// Import errors - typically errors
	"F4": "error", // F4xx: Import-related errors (F401 unused, F403 star import)
	"F8": "error", // F8xx: Undefined names (F821, F811 redefinition)

	// Other pyflakes - can be errors
	"F": "error", // General pyflakes errors

	// Style warnings
	"E": "warning", // E1xx-E5xx: Style issues (whitespace, indentation, etc.)
	"W": "warning", // Wxx: Warnings
	"C": "warning", // Cxx: Complexity/convention
	"N": "warning", // Nxx: Naming conventions
	"D": "warning", // Dxx: Docstring issues
	"B": "warning", // Bxx: Bugbear (potential bugs, but stylistic)
	"I": "warning", // Ixx: isort
	"S": "warning", // Sxx: bandit security (could argue for error)
	"T": "warning", // Txx: flake8-debugger, print statements
	"Q": "warning", // Qxx: quotes
	"A": "warning", // Axx: builtins
	"P": "warning", // Pxx: pytest style
}

// pylintSeverityCodes maps pylint code prefixes to severity.
// C = Convention, R = Refactor, W = Warning, E = Error, F = Fatal
var pylintSeverityCodes = map[string]string{
	"C": "warning", // Convention
	"R": "warning", // Refactor
	"W": "warning", // Warning
	"E": "error",   // Error
	"F": "error",   // Fatal
}

// GetRuffFlake8Severity returns the severity for a ruff/flake8 error code.
func GetRuffFlake8Severity(code string) string {
	if len(code) < 1 {
		return "error"
	}

	// Check 2-character prefix first (more specific)
	if len(code) >= 2 {
		if sev, ok := ruffFlake8SeverityCodes[code[:2]]; ok {
			return sev
		}
	}

	// Check 1-character prefix
	if sev, ok := ruffFlake8SeverityCodes[code[:1]]; ok {
		return sev
	}

	// Default to error for unknown codes
	return "error"
}

// GetPylintSeverity returns the severity for a pylint code.
func GetPylintSeverity(code string) string {
	if len(code) < 1 {
		return "error"
	}

	if sev, ok := pylintSeverityCodes[code[:1]]; ok {
		return sev
	}

	return "error"
}
