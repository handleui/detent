package errors

import "regexp"

// Regex patterns for extracting errors from act output.
// Patterns use lazy quantifiers (.+?) where possible to prevent ReDoS.
var (
	// Generic error pattern: "Error: ..." or "error: ..."
	// Uses lazy quantifier to prevent catastrophic backtracking
	errorPattern = regexp.MustCompile(`(?i)error:\s*(.+?)$`)

	// Go compiler/test error pattern: file.go:123:45: message
	// Optimized: captures leading/trailing whitespace to eliminate post-processing TrimSpace
	goErrorPattern = regexp.MustCompile(`^([^\s:]+\.go):(\d+):(\d+):\s*(.+?)\s*$`)

	// TypeScript error pattern: file.ts(10,5): error TS1234: message
	// Group 1: file path
	// Group 2: line number
	// Group 3: column number
	// Group 4: TS error code (e.g., "TS2749") - optional
	// Group 5: message
	// Optimized: captures leading/trailing whitespace to eliminate post-processing TrimSpace
	tsErrorPattern = regexp.MustCompile(`^([^\s:]+\.tsx?)\((\d+),(\d+)\):\s*(?:error\s+(TS\d+):\s*)?(.+?)\s*$`)

	// Python traceback pattern: File "file.py", line 10
	pythonErrorPattern = regexp.MustCompile(`^\s*File\s+"([^"]+)",\s+line\s+(\d+)`)

	// Python exception pattern: ExceptionType: error message
	// Group 1: exception type (e.g., "ValueError", "TypeError", "RuntimeError")
	// Group 2: error message
	pythonExceptionPattern = regexp.MustCompile(`^([A-Z]\w+(?:Error|Exception|Warning)):\s*(.+)$`)

	// Rust error pattern: error[E0123]: message --> file.rs:10:5
	rustErrorPattern = regexp.MustCompile(`-->\s*([^\s:]+\.rs):(\d+):(\d+)`)

	// Rust error message pattern: error[E0123]: message
	// Group 1: error code (e.g., "E0123")
	// Group 2: error message
	rustErrorMessagePattern = regexp.MustCompile(`^error\[([A-Z0-9]+)\]:\s*(.+)$`)

	// Go test failure pattern: --- FAIL: TestName
	goTestFailPattern = regexp.MustCompile(`^---\s+FAIL:\s+(\S+)`)

	// Node.js stack trace pattern: at Function (file.js:10:5)
	// Uses lazy quantifier for the function part
	nodeStackPattern = regexp.MustCompile(`at\s+.+?\(([^:]+):(\d+):(\d+)\)`)

	// ESLint pattern: 10:5 error/warning Message rule-name
	// Group 1: line number
	// Group 2: column number
	// Group 3: severity ("error" or "warning")
	// Group 4: message + rule name (parsed later by ESLintMessageBuilder)
	eslintPattern = regexp.MustCompile(`^\s*(\d+):(\d+)\s+(error|warning)\s+(.+)$`)

	// Generic file:line pattern (fallback)
	// Optimized: anchored to line start or whitespace to prevent mid-line matches
	genericFileLinePattern = regexp.MustCompile(`(?:^|\s)([^\s:]+\.[a-zA-Z0-9]+):(\d+)(?::(\d+))?`)

	// Act job context pattern: [workflow/job] | message
	actContextPattern = regexp.MustCompile(`^\[([^\]]+)\]`)

	// Metadata patterns: workflow infrastructure messages (not code errors)
	// These patterns match act/GitHub Actions metadata and should be categorized separately

	// Exit code pattern: "exit code 1", "exitcode '1': failure"
	exitCodePattern = regexp.MustCompile(`(?i)exit(?:ed)?\s+(?:with\s+)?(?:code\s+)?(\d+)`)

	// Job/workflow status patterns
	jobFailedPattern = regexp.MustCompile(`(?i)(?:job|workflow)\s+['"]?([^'"]+)['"]?\s+failed`)

	// File path pattern: matches standalone file paths (for ESLint multi-line format)
	filePathPattern = regexp.MustCompile(`^([^\s:]+\.(ts|tsx|js|jsx|go|py|rs|java|c|cpp|h|hpp))$`)

	// Docker error patterns: infrastructure failures
	// Matches: "No such container: abc123", "Cannot connect to the Docker daemon", etc.
	dockerErrorPattern = regexp.MustCompile(`(?i)(no such container|cannot connect to.*docker|image pull failed|docker.*error response from daemon|container.*is not running|failed to.*docker|docker.*permission denied)`)

	// Stack trace patterns for multi-line error capture

	// Python traceback start pattern
	pythonTracebackPattern = regexp.MustCompile(`^Traceback \(most recent call last\):`)

	// Python traceback line pattern (captures file/line info and code snippet)
	pythonTraceLinePattern = regexp.MustCompile(`^\s+File\s+"[^"]+",\s+line\s+\d+`)

	// Go panic pattern
	goPanicPattern = regexp.MustCompile(`^panic:`)

	// Go goroutine pattern (indicates start of stack frames)
	goGoroutinePattern = regexp.MustCompile(`^goroutine \d+ \[`)

	// Go stack frame pattern (function call in stack trace)
	goStackFramePattern = regexp.MustCompile(`^\S+\([^)]*\)$|^\s+\S+:\d+`)

	// Node.js "at" stack trace pattern (looser match for accumulation)
	nodeAtPattern = regexp.MustCompile(`^\s+at\s+`)

	// Test output continuation pattern (lines after test failure)
	testOutputPattern = regexp.MustCompile(`^\s{4,}`) // Indented test output lines
)
