package errors

import "regexp"

// Regex patterns for extracting errors from act output.
// Patterns use lazy quantifiers (.+?) where possible to prevent ReDoS.
var (
	// Generic error pattern: "Error: ..." or "error: ..."
	// Uses lazy quantifier to prevent catastrophic backtracking
	errorPattern = regexp.MustCompile(`(?i)error:\s*(.+?)$`)

	// Go compiler/test error pattern: file.go:123:45: message
	goErrorPattern = regexp.MustCompile(`^([^\s:]+\.go):(\d+):(\d+):\s*(.+)$`)

	// TypeScript error pattern: file.ts(10,5): error TS1234: message
	tsErrorPattern = regexp.MustCompile(`^([^\s:]+\.tsx?)\((\d+),(\d+)\):\s*(?:error\s+TS\d+:\s*)?(.+)$`)

	// Python traceback pattern: File "file.py", line 10
	pythonErrorPattern = regexp.MustCompile(`^\s*File\s+"([^"]+)",\s+line\s+(\d+)`)

	// Rust error pattern: error[E0123]: message --> file.rs:10:5
	rustErrorPattern = regexp.MustCompile(`-->\s*([^\s:]+\.rs):(\d+):(\d+)`)

	// Go test failure pattern: --- FAIL: TestName
	goTestFailPattern = regexp.MustCompile(`^---\s+FAIL:\s+(\S+)`)

	// Node.js stack trace pattern: at Function (file.js:10:5)
	// Uses lazy quantifier for the function part
	nodeStackPattern = regexp.MustCompile(`at\s+.+?\(([^:]+):(\d+):(\d+)\)`)

	// ESLint pattern: 10:5 error Message rule-name
	eslintPattern = regexp.MustCompile(`^\s*(\d+):(\d+)\s+error\s+(.+?)\s+\S+$`)

	// Generic file:line pattern (fallback)
	genericFileLinePattern = regexp.MustCompile(`([^\s:]+\.[a-zA-Z0-9]+):(\d+)(?::(\d+))?`)

	// Act job context pattern: [workflow/job] | message
	actContextPattern = regexp.MustCompile(`^\[([^\]]+)\]`)

	// Exit code pattern
	exitCodePattern = regexp.MustCompile(`(?i)exit(?:ed)?\s+(?:with\s+)?(?:code\s+)?(\d+)`)
)
