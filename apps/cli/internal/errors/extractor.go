package errors

import (
	"bufio"
	"strconv"
	"strings"
)

// Extractor processes act output and extracts structured errors.
type Extractor struct {
	lastFile           string           // Tracks the last file path seen (for multi-line error formats)
	currentWorkflowCtx *WorkflowContext // Tracks the current workflow/job context from act output
	lastPythonError    *ExtractedError  // Tracks the last Python error (for multi-line traceback format)
	lastRustError      *ExtractedError  // Tracks the last Rust error (for multi-line error format)
}

// errKey is used for deduplication
type errKey struct {
	message string
	file    string
	line    int
}

// Extract parses act output and extracts errors.
// Duplicate errors (same message, file, line) are filtered out.
func (e *Extractor) Extract(output string) []*ExtractedError {
	var extracted []*ExtractedError
	seen := make(map[errKey]struct{})

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		ctx, cleanLine := parseActContext(line)

		// Update workflow context if found
		if ctx != nil {
			e.currentWorkflowCtx = ctx
		}

		if found := e.extractFromLine(cleanLine); found != nil {
			// Attach workflow context to the error (clone to prevent stale pointer sharing)
			found.WorkflowContext = e.currentWorkflowCtx.Clone()

			key := errKey{found.Message, found.File, found.Line}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				extracted = append(extracted, found)
			}
		}
	}

	return extracted
}

// parseActContext extracts workflow context and returns both the context and the cleaned line
func parseActContext(line string) (ctx *WorkflowContext, cleanedLine string) {
	if match := actContextPattern.FindStringSubmatchIndex(line); match != nil {
		ctx = &WorkflowContext{
			Job: line[match[2]:match[3]], // Extract directly from indices
		}
		if match[1] < len(line) {
			rest := line[match[1]:]
			if rest != "" && rest[0] == '|' {
				rest = rest[1:]
			}
			cleanedLine = strings.TrimSpace(rest)
			return
		}
	}
	cleanedLine = line
	return
}

// parseLineCol parses line and column numbers from regex match groups.
// Returns 0 for invalid input (regex guarantees \d+ so this shouldn't happen).
func parseLineCol(lineStr, colStr string) (line, col int) {
	line, _ = strconv.Atoi(lineStr)
	col, _ = strconv.Atoi(colStr)
	return line, col
}

func (e *Extractor) extractFromLine(line string) *ExtractedError {
	// Check if this is a standalone file path line (for multi-line error formats)
	if match := filePathPattern.FindStringSubmatch(line); match != nil {
		e.lastFile = match[1]
		return nil
	}

	if match := goErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
		return &ExtractedError{
			Message:  strings.TrimSpace(match[4]),
			File:     match[1],
			Line:     lineNum,
			Column:   colNum,
			Category: CategoryCompile,
			Source:   "go",
			Raw:      line,
		}
	}

	if match := tsErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
		return &ExtractedError{
			Message:  strings.TrimSpace(match[5]), // Group 5 is message (group 4 is TS code)
			File:     match[1],
			Line:     lineNum,
			Column:   colNum,
			RuleID:   match[4],            // "TS2749", "TS1234", etc. (may be empty)
			Category: CategoryTypeCheck,
			Source:   "typescript",
			Raw:      line,
		}
	}

	// Python traceback: first check if this line has the exception message
	if match := pythonExceptionPattern.FindStringSubmatch(line); match != nil {
		// If we have a pending Python error from traceback, update its message
		if e.lastPythonError != nil {
			e.lastPythonError.Message = match[1] + ": " + match[2]
			err := e.lastPythonError
			e.lastPythonError = nil
			return err
		}
		// Standalone exception without traceback
		return &ExtractedError{
			Message:  match[1] + ": " + match[2],
			Category: CategoryRuntime,
			Source:   "python",
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
			Source:   "python",
			Raw:      line,
		}
		return nil // Don't return yet, wait for exception message
	}

	// Rust error message: error[E0123]: message (comes before location)
	if match := rustErrorMessagePattern.FindStringSubmatch(line); match != nil {
		// Store this error and wait for the location line
		e.lastRustError = &ExtractedError{
			Message:  match[2],
			RuleID:   match[1], // Error code like "E0123"
			Category: CategoryCompile,
			Source:   "rust",
			Raw:      line,
		}
		return nil // Don't return yet, wait for location line
	}

	// Rust location: --> file.rs:10:5
	if match := rustErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
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
			Source:   "rust",
			Raw:      line,
		}
	}

	if match := goTestFailPattern.FindStringSubmatch(line); match != nil {
		return &ExtractedError{
			Message:  "Test failed: " + match[1],
			Category: CategoryTest,
			Source:   "go-test",
			Raw:      line,
		}
	}

	if match := nodeStackPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
		return &ExtractedError{
			Message:  "Node.js error",
			File:     match[1],
			Line:     lineNum,
			Column:   colNum,
			Category: CategoryRuntime,
			Source:   "nodejs",
			Raw:      line,
		}
	}

	if match := eslintPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[1], match[2])
		err := &ExtractedError{
			Message:  strings.TrimSpace(match[4]),
			File:     e.lastFile, // Use the last seen file path
			Line:     lineNum,
			Column:   colNum,
			Severity: match[3], // "error" or "warning"
			Category: CategoryLint,
			Source:   "eslint",
			Raw:      line,
		}

		// Parse rule ID from message (format: "Message text rule-name")
		if ruleMatch := eslintRulePattern.FindStringSubmatch(err.Message); ruleMatch != nil {
			err.Message = strings.TrimSpace(ruleMatch[1])
			err.RuleID = ruleMatch[2]
		}

		return err
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

	if match := exitCodePattern.FindStringSubmatch(line); match != nil {
		code, _ := strconv.Atoi(match[1])
		if code != 0 {
			return &ExtractedError{
				Message:  "Exit code " + match[1],
				Category: CategoryRuntime,
				Raw:      line,
			}
		}
	}

	return nil
}
