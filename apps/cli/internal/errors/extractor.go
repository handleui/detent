package errors

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/detent/cli/internal/ci"
	"github.com/detent/cli/internal/messages"
)

const (
	maxStackTraceLines   = 5000  // Maximum stack trace lines to prevent memory exhaustion
	maxDeduplicationSize = 10000 // Maximum deduplicated errors to prevent unbounded map growth
	maxLineLength        = 65536 // 64KB per line - prevents ReDoS on extremely long lines

	// SeverityError is the interned constant for "error" severity level
	SeverityError = "error"
	// SeverityWarning is the interned constant for "warning" severity level
	SeverityWarning = "warning"
)

// Extractor processes act output and extracts structured errors.
//
// THREAD-SAFETY: This type is NOT thread-safe. Each Extractor instance maintains
// internal state that tracks multi-line error formats (Python tracebacks, Rust errors,
// stack traces) across successive calls to Extract(). This stateful design assumes
// sequential processing of log lines.
//
// STATE MANAGEMENT: The extractor maintains several pieces of mutable state:
//   - lastFile: Remembers the most recent file path for errors without explicit paths
//   - currentWorkflowCtx: Tracks the current workflow/job context from act output
//   - lastPythonError/lastRustError: Hold partially-parsed errors awaiting completion
//   - inStackTrace/stackTraceLines/stackTraceOwner: Accumulate multi-line stack traces
//
// USAGE: Create one Extractor instance per extraction session (per workflow run).
// Do NOT share an Extractor across goroutines. Do NOT reuse an Extractor for
// multiple unrelated extraction sessions, as residual state may cause incorrect parsing.
type Extractor struct {
	lastFile           string           // Tracks the last file path seen (for multi-line error formats)
	currentWorkflowCtx *WorkflowContext // Tracks the current workflow/job context from act output
	lastPythonError    *ExtractedError  // Tracks the last Python error (for multi-line traceback format)
	lastRustError      *ExtractedError  // Tracks the last Rust error (for multi-line error format)

	// Stack trace accumulation
	inStackTrace      bool            // Are we currently reading a stack trace?
	stackTraceBuilder strings.Builder // Accumulate stack trace lines
	stackTraceLines   int             // Count of stack trace lines
	stackTraceOwner   *ExtractedError // Which error owns this stack trace

	// Message builders for language-specific message construction
	pythonBuilder *messages.PythonMessageBuilder
	eslintBuilder *messages.ESLintMessageBuilder
}

// NewExtractor creates a new Extractor with initialized message builders.
func NewExtractor() *Extractor {
	return &Extractor{
		pythonBuilder: messages.NewPythonMessageBuilder(),
		eslintBuilder: messages.NewESLintMessageBuilder(),
	}
}

// errKey is used for deduplication
type errKey struct {
	message string
	file    string
	line    int
}

// ExtractWithContext parses CI output using a context parser for line preprocessing.
// This separates CI platform concerns (act, GitHub Actions, GitLab) from tool output parsing.
// The context parser strips CI-specific prefixes and filters noise before tool pattern matching.
func (e *Extractor) ExtractWithContext(output string, ctxParser ci.ContextParser) []*ExtractedError {
	var extracted []*ExtractedError
	seen := make(map[errKey]struct{}, 256)

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
			e.currentWorkflowCtx = &WorkflowContext{
				Job:  ctx.Job,
				Step: ctx.Step,
			}
		}

		if found := e.extractFromLine(cleanLine); found != nil {
			found.WorkflowContext = e.currentWorkflowCtx.Clone()

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

	// Finalize any pending stack trace
	if e.inStackTrace && e.stackTraceOwner != nil {
		owner := e.stackTraceOwner
		e.finalizeStackTrace()
		if e.currentWorkflowCtx != nil {
			owner.WorkflowContext = e.currentWorkflowCtx.Clone()
		}

		if len(seen) >= maxDeduplicationSize {
			extracted = append(extracted, owner)
		} else {
			key := errKey{owner.Message, owner.File, owner.Line}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				extracted = append(extracted, owner)
			}
		}
	}

	return extracted
}

// parseLineCol parses line and column numbers from regex match groups.
// Returns error if conversion fails.
func parseLineCol(lineStr, colStr string) (line, col int, err error) {
	line, err = strconv.Atoi(lineStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse line number: %w", err)
	}
	col, err = strconv.Atoi(colStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse column number: %w", err)
	}
	return line, col, nil
}

// internSeverity returns an interned severity constant for common values.
// This reduces memory allocations by reusing string constants.
func internSeverity(s string) string {
	if s == "error" {
		return SeverityError
	}
	if s == "warning" {
		return SeverityWarning
	}
	return s
}

// startStackTrace begins accumulating stack trace lines for an error
func (e *Extractor) startStackTrace(owner *ExtractedError) {
	e.inStackTrace = true
	e.stackTraceBuilder.Reset()
	e.stackTraceLines = 0
	e.stackTraceOwner = owner
}

// addStackTraceLine adds a line to the current stack trace
func (e *Extractor) addStackTraceLine(line string) {
	if !e.inStackTrace {
		return
	}

	// Check if we've reached the maximum stack trace lines
	if e.stackTraceLines > maxStackTraceLines {
		// Add truncation message only once (when just exceeded limit)
		if e.stackTraceLines == maxStackTraceLines+1 {
			e.stackTraceBuilder.WriteString("... (stack trace truncated)\n")
		}
		return
	}

	if e.stackTraceLines > 0 {
		e.stackTraceBuilder.WriteByte('\n')
	}
	e.stackTraceBuilder.WriteString(line)
	e.stackTraceLines++
}

// finalizeStackTrace attaches the accumulated stack trace to the owner and resets state
func (e *Extractor) finalizeStackTrace() {
	if e.inStackTrace && e.stackTraceOwner != nil && e.stackTraceLines > 0 {
		e.stackTraceOwner.StackTrace = e.stackTraceBuilder.String()
	}
	e.inStackTrace = false
	e.stackTraceBuilder.Reset()
	e.stackTraceLines = 0
	e.stackTraceOwner = nil
}

// isStackTraceContinuation checks if the line is part of an ongoing stack trace
func (e *Extractor) isStackTraceContinuation(line string) bool {
	if !e.inStackTrace {
		return false
	}

	// Python: traceback lines or code snippets
	if pythonTraceLinePattern.MatchString(line) || strings.HasPrefix(line, "    ") {
		return true
	}

	// Go: stack frames or file:line references
	if goStackFramePattern.MatchString(line) {
		return true
	}

	// Node.js: at Function(...) lines
	if nodeAtPattern.MatchString(line) {
		return true
	}

	// Test output: indented continuation
	if testOutputPattern.MatchString(line) {
		return true
	}

	return false
}

// extractMetadata checks if the line is workflow metadata (not a code error).
// Metadata includes exit codes, job status messages, and other infrastructure messages.
// Returns nil if the line is not metadata.
func (e *Extractor) extractMetadata(line string) *ExtractedError {
	// Job status patterns: "Job 'build' failed", "Error: Job 'build' failed"
	if match := jobFailedPattern.FindStringSubmatch(line); match != nil {
		return &ExtractedError{
			Message:  fmt.Sprintf("Job '%s' failed", match[1]),
			Category: CategoryMetadata,
			Source:   SourceMetadata,
			Raw:      line,
		}
	}

	// Exit code patterns: "exit code 1", "exitcode '1': failure"
	// Only treat as metadata if it doesn't have file/line context (actual errors have that)
	if match := exitCodePattern.FindStringSubmatch(line); match != nil {
		code, _ := strconv.Atoi(match[1])
		// Only non-zero exit codes are issues
		if code != 0 {
			return &ExtractedError{
				Message:  "Exit code " + match[1],
				Category: CategoryMetadata,
				Source:   SourceMetadata,
				Raw:      line,
			}
		}
	}

	return nil
}

func (e *Extractor) extractFromLine(line string) *ExtractedError {
	// Note: Debug noise filtering (e.g., act's Go struct dumps with <nil> values)
	// is now handled in the CI context parser layer (e.g., ci/act.ContextParser).
	// This method assumes the line has already been preprocessed.

	// Check if we're continuing a stack trace
	if e.isStackTraceContinuation(line) {
		e.addStackTraceLine(line)
		return nil
	}

	// If we have an active stack trace but this line doesn't continue it,
	// finalize and return the error (unless it's Python, which has special handling)
	if e.inStackTrace && e.stackTraceOwner != nil && e.lastPythonError == nil {
		// Check if this is not a Python exception (which finalizes in its own handler)
		if !pythonExceptionPattern.MatchString(line) {
			e.finalizeStackTrace()
			err := e.stackTraceOwner
			e.stackTraceOwner = nil
			return err
		}
	}

	// METADATA PATTERNS FIRST - Check for workflow infrastructure messages
	// These should be identified before code error patterns to prevent false categorization
	if metadataErr := e.extractMetadata(line); metadataErr != nil {
		return metadataErr
	}

	// Check for Python traceback start
	if pythonTracebackPattern.MatchString(line) {
		// We'll start accumulating when we get the first File line
		return nil
	}

	// Check for Go panic start
	if goPanicPattern.MatchString(line) {
		e.finalizeStackTrace() // Finalize any previous trace
		panicErr := &ExtractedError{
			Message:  strings.TrimPrefix(line, "panic: "),
			Category: CategoryRuntime,
			Source:   SourceGo,
			Raw:      line,
		}
		e.startStackTrace(panicErr)
		e.addStackTraceLine(line)
		return nil // Don't return yet, accumulate the stack
	}

	// Check for Go goroutine (part of panic stack trace)
	if goGoroutinePattern.MatchString(line) {
		e.addStackTraceLine(line)
		return nil
	}

	// Check if this is a standalone file path line (for multi-line error formats)
	if match := filePathPattern.FindStringSubmatch(line); match != nil {
		e.lastFile = match[1]
		return nil
	}

	// Docker infrastructure errors (high priority - prevents workflow execution)
	if match := dockerErrorPattern.FindStringSubmatch(line); match != nil {
		return &ExtractedError{
			Message:  strings.TrimSpace(match[0]),
			Category: CategoryRuntime,
			Source:   SourceDocker,
			Severity: SeverityError,
			Raw:      line,
		}
	}

	if match := goErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum, err := parseLineCol(match[2], match[3])
		if err != nil {
			// Log error but continue extraction with 0 values
			lineNum, colNum = 0, 0
		}
		return &ExtractedError{
			Message:  strings.TrimSpace(match[4]),
			File:     match[1],
			Line:     lineNum,
			Column:   colNum,
			Category: CategoryCompile,
			Source:   SourceGo,
			Raw:      line,
		}
	}

	if match := tsErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum, err := parseLineCol(match[2], match[3])
		if err != nil {
			// Log error but continue extraction with 0 values
			lineNum, colNum = 0, 0
		}
		return &ExtractedError{
			Message:  strings.TrimSpace(match[5]), // Group 5 is message (group 4 is TS code)
			File:     match[1],
			Line:     lineNum,
			Column:   colNum,
			RuleID:   match[4],            // "TS2749", "TS1234", etc. (may be empty)
			Category: CategoryTypeCheck,
			Source:   SourceTypeScript,
			Raw:      line,
		}
	}

	// Python traceback: first check if this line has the exception message
	if match := pythonExceptionPattern.FindStringSubmatch(line); match != nil {
		// Build the Python error message using the builder
		message := e.pythonBuilder.BuildMessage(match[1], match[2])

		// If we have a pending Python error from traceback, update its message
		if e.lastPythonError != nil {
			e.lastPythonError.Message = message
			e.addStackTraceLine(line) // Add exception line to stack trace
			e.finalizeStackTrace()    // Finalize the Python traceback
			err := e.lastPythonError
			e.lastPythonError = nil
			return err
		}
		// Standalone exception without traceback
		return &ExtractedError{
			Message:  message,
			Category: CategoryRuntime,
			Source:   SourcePython,
			Raw:      line,
		}
	}

	// Python traceback: File "file.py", line 10
	if match := pythonErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, _ := strconv.Atoi(match[2])
		// Store this error and wait for the exception message on next line
		e.lastPythonError = &ExtractedError{
			Message:  "Python error", // Will be replaced when exception line is found
			File:     match[1],
			Line:     lineNum,
			Category: CategoryRuntime,
			Source:   SourcePython,
			Raw:      line,
		}
		// Start accumulating stack trace for Python errors
		e.startStackTrace(e.lastPythonError)
		e.addStackTraceLine(line)
		return nil // Don't return yet, wait for exception message
	}

	// Rust error message: error[E0123]: message (comes before location)
	if match := rustErrorMessagePattern.FindStringSubmatch(line); match != nil {
		// Store this error and wait for the location line
		e.lastRustError = &ExtractedError{
			Message:  match[2],
			RuleID:   match[1], // Error code like "E0123"
			Category: CategoryCompile,
			Source:   SourceRust,
			Raw:      line,
		}
		return nil // Don't return yet, wait for location line
	}

	// Rust location: --> file.rs:10:5
	if match := rustErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum, err := parseLineCol(match[2], match[3])
		if err != nil {
			// Log error but continue extraction with 0 values
			lineNum, colNum = 0, 0
		}
		// If we have a pending Rust error, update its location
		if e.lastRustError != nil {
			e.lastRustError.File = match[1]
			e.lastRustError.Line = lineNum
			e.lastRustError.Column = colNum
			err := e.lastRustError
			e.lastRustError = nil
			return err
		}
		// Standalone location without error message
		return &ExtractedError{
			Message:  "Rust error",
			File:     match[1],
			Line:     lineNum,
			Column:   colNum,
			Category: CategoryCompile,
			Source:   SourceRust,
			Raw:      line,
		}
	}

	if match := goTestFailPattern.FindStringSubmatch(line); match != nil {
		e.finalizeStackTrace() // Finalize any previous stack trace
		testErr := &ExtractedError{
			Message:  "Test failed: " + match[1],
			Category: CategoryTest,
			Source:   SourceGoTest,
			Raw:      line,
		}
		// Start accumulating test output as stack trace
		e.startStackTrace(testErr)
		e.addStackTraceLine(line)
		return nil // Don't return yet, accumulate test output
	}

	if match := nodeStackPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum, err := parseLineCol(match[2], match[3])
		if err != nil {
			// Log error but continue extraction with 0 values
			lineNum, colNum = 0, 0
		}
		nodeErr := &ExtractedError{
			Message:  "Node.js error",
			File:     match[1],
			Line:     lineNum,
			Column:   colNum,
			Category: CategoryRuntime,
			Source:   SourceNodeJS,
			Raw:      line,
		}
		// Start stack trace if this is the first "at" line, or continue accumulating
		if !e.inStackTrace {
			e.startStackTrace(nodeErr)
		}
		e.addStackTraceLine(line)
		return nil // Don't return yet, accumulate the full stack
	}

	if match := eslintPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum, err := parseLineCol(match[1], match[2])
		if err != nil {
			// Log error but continue extraction with 0 values
			lineNum, colNum = 0, 0
		}
		rawMessage := strings.TrimSpace(match[4])

		// Parse rule ID from message using the ESLint builder
		cleanMessage, ruleID := e.eslintBuilder.ParseRuleID(rawMessage)

		extractedErr := &ExtractedError{
			Message:  cleanMessage,
			File:     e.lastFile, // Use the last seen file path
			Line:     lineNum,
			Column:   colNum,
			Severity: internSeverity(match[3]), // "error" or "warning"
			RuleID:   ruleID,
			Category: CategoryLint,
			Source:   SourceESLint,
			Raw:      line,
		}

		return extractedErr
	}

	if match := errorPattern.FindStringSubmatch(line); match != nil {
		err := &ExtractedError{
			Message:  strings.TrimSpace(match[1]),
			Category: CategoryUnknown,
			Raw:      line,
		}
		if fileMatch := genericFileLinePattern.FindStringSubmatch(match[1]); fileMatch != nil {
			err.File = fileMatch[1]
			err.Line, _ = strconv.Atoi(fileMatch[2])
			if fileMatch[3] != "" {
				err.Column, _ = strconv.Atoi(fileMatch[3])
			}
		}
		return err
	}

	// Exit code and job failures are now handled in extractMetadata() above
	return nil
}
