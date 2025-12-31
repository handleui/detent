package rust

import (
	"regexp"
)

// Rust-specific regex patterns for error extraction.
// Patterns use lazy quantifiers (.+?) where possible to prevent ReDoS.
var (
	// rustErrorHeaderPattern matches Rust compiler error/warning headers:
	// - error[E0308]: mismatched types
	// - warning[W0501]: unused variable
	// - error: cannot find type `Foo` in this scope
	// - warning: unused variable `x`
	// Group 1: level (error/warning)
	// Group 2: error code (optional, e.g., "E0308")
	// Group 3: message
	rustErrorHeaderPattern = regexp.MustCompile(`^(error|warning)(?:\[([A-Z]\d{4})\])?:\s*(.+)$`)

	// rustLocationPattern matches Rust error location arrows:
	// - --> src/main.rs:4:7
	// - --> /path/to/file.rs:123:45
	// Group 1: file path
	// Group 2: line number
	// Group 3: column number
	rustLocationPattern = regexp.MustCompile(`^\s*-->\s*([^:]+):(\d+):(\d+)$`)

	// rustNotePattern matches note/help lines:
	// - = note: expected type `i32`
	// - = help: consider using `.to_string()`
	// Group 1: type (note/help)
	// Group 2: message
	rustNotePattern = regexp.MustCompile(`^\s*=\s*(note|help):\s*(.+)$`)

	// rustClippyLintPattern extracts Clippy lint codes from note lines:
	// - = note: `#[warn(clippy::redundant_clone)]` on by default
	// - = note: `#[deny(clippy::unwrap_used)]` implied by...
	// Group 1: lint name (e.g., "redundant_clone")
	rustClippyLintPattern = regexp.MustCompile(`#\[(?:warn|deny|allow)\(clippy::([a-z_]+)\)\]`)

	// rustCodeLinePattern matches the source code line indicator:
	// |
	// 4 | let x: i32 = "hello";
	// This helps identify continuation lines
	rustCodeLinePattern = regexp.MustCompile(`^\s*\d*\s*\|`)

	// rustCaretPattern matches the caret/underline line that points to the error:
	// |     ^^^^^^^ expected `i32`, found `&str`
	// Group 1: optional label message
	rustCaretPattern = regexp.MustCompile(`^\s*\|\s*[-\^]+\s*(.*)$`)

	// rustTestFailPattern matches test failure markers:
	// - test tests::test_foo ... FAILED
	// Group 1: test name
	rustTestFailPattern = regexp.MustCompile(`^test\s+(\S+)\s+\.{3}\s+FAILED$`)

	// noisePatterns are lines that should be skipped as noise
	noisePatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\s*Compiling\s+`),          // Cargo progress
		regexp.MustCompile(`^\s*Downloading\s+`),        // Crate download
		regexp.MustCompile(`^\s*Downloaded\s+`),         // Crate downloaded
		regexp.MustCompile(`^\s*Finished\s+`),           // Build complete
		regexp.MustCompile(`^\s*Running\s+`),            // Test/binary execution
		regexp.MustCompile(`^\s*Doc-tests\s+`),          // Doc test header
		regexp.MustCompile(`^test result:`),             // Test summary
		regexp.MustCompile(`^running\s+\d+\s+tests?`),   // Test count
		regexp.MustCompile(`^test\s+.+\s+\.{3}\s+ok$`), // Individual test pass
		regexp.MustCompile(`^\s*Caused by:`),            // Cargo error chain (not code error)
		regexp.MustCompile(`^\s*Updating\s+`),           // Cargo update
		regexp.MustCompile(`^\s*Blocking\s+`),           // Cargo blocking message
		regexp.MustCompile(`^\s*Fresh\s+`),              // Cargo fresh (no rebuild needed)
		regexp.MustCompile(`^\s*Packaging\s+`),          // Cargo package
		regexp.MustCompile(`^\s*Verifying\s+`),          // Cargo verify
		regexp.MustCompile(`^\s*Archiving\s+`),          // Cargo archive
		regexp.MustCompile(`^\s*Uploading\s+`),          // Cargo upload
		regexp.MustCompile(`^\s*Waiting\s+`),            // Cargo waiting
		regexp.MustCompile(`^For more information`),     // rustc help hint
		regexp.MustCompile(`^aborting due to`),          // rustc abort summary
		regexp.MustCompile(`^Some errors have`),         // rustc multiple errors hint
		regexp.MustCompile(`^error: could not compile`), // High-level compile fail (not useful)
		regexp.MustCompile(`^warning: build failed`),    // High-level build fail
	}

	// CriticalClippyLints are Clippy lint codes that should be treated as errors
	// even though they're reported as warnings. These indicate potential bugs or
	// unsafe patterns.
	CriticalClippyLints = map[string]bool{
		"unwrap_used":         true, // Panics on None/Err
		"expect_used":         true, // Panics with message
		"panic":               true, // Explicit panic
		"todo":                true, // Unfinished code
		"unimplemented":       true, // Unfinished code
		"unreachable":         true, // Code that shouldn't execute
		"indexing_slicing":    true, // Can panic on out of bounds
		"missing_panics_doc":  true, // Missing panic documentation
		"unwrap_in_result":    true, // Unwrap inside Result-returning fn
		"manual_assert":       true, // Should use assert!
		"arithmetic_side_effects": true, // Overflow/underflow
	}
)
