package errors

import (
	"bufio"
	"strconv"
	"strings"
)

// Extractor processes act output and extracts structured errors.
// It is stateless and can be reused across multiple Extract calls.
type Extractor struct{}

// errKey is used for deduplication
type errKey struct {
	message string
	file    string
	line    int
}

// Extract parses act output and extracts errors.
// Duplicate errors (same message, file, line) are filtered out.
func (e Extractor) Extract(output string) []*ExtractedError {
	var extracted []*ExtractedError
	seen := make(map[errKey]bool)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		line = stripActContext(line)

		if found := e.extractFromLine(line); found != nil {
			key := errKey{found.Message, found.File, found.Line}
			if !seen[key] {
				seen[key] = true
				extracted = append(extracted, found)
			}
		}
	}

	return extracted
}

func stripActContext(line string) string {
	if match := actContextPattern.FindStringSubmatch(line); match != nil {
		idx := strings.Index(line, "]")
		if idx >= 0 && idx+1 < len(line) {
			rest := strings.TrimSpace(line[idx+1:])
			rest = strings.TrimPrefix(rest, "|")
			return strings.TrimSpace(rest)
		}
	}
	return line
}

// parseLineCol parses line and column numbers from regex match groups.
// Returns 0 for invalid input (regex guarantees \d+ so this shouldn't happen).
func parseLineCol(lineStr, colStr string) (line, col int) {
	line, _ = strconv.Atoi(lineStr)
	col, _ = strconv.Atoi(colStr)
	return line, col
}

func (e Extractor) extractFromLine(line string) *ExtractedError {
	if match := goErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
		return &ExtractedError{
			Message: strings.TrimSpace(match[4]),
			File:    match[1],
			Line:    lineNum,
			Column:  colNum,
			Raw:     line,
		}
	}

	if match := tsErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
		return &ExtractedError{
			Message: strings.TrimSpace(match[4]),
			File:    match[1],
			Line:    lineNum,
			Column:  colNum,
			Raw:     line,
		}
	}

	if match := pythonErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, _ := strconv.Atoi(match[2])
		return &ExtractedError{
			Message: "Python error",
			File:    match[1],
			Line:    lineNum,
			Raw:     line,
		}
	}

	if match := rustErrorPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
		return &ExtractedError{
			Message: "Rust error",
			File:    match[1],
			Line:    lineNum,
			Column:  colNum,
			Raw:     line,
		}
	}

	if match := goTestFailPattern.FindStringSubmatch(line); match != nil {
		return &ExtractedError{
			Message: "Test failed: " + match[1],
			Raw:     line,
		}
	}

	if match := nodeStackPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[2], match[3])
		return &ExtractedError{
			Message: "Node.js error",
			File:    match[1],
			Line:    lineNum,
			Column:  colNum,
			Raw:     line,
		}
	}

	if match := eslintPattern.FindStringSubmatch(line); match != nil {
		lineNum, colNum := parseLineCol(match[1], match[2])
		return &ExtractedError{
			Message: strings.TrimSpace(match[3]),
			Line:    lineNum,
			Column:  colNum,
			Raw:     line,
		}
	}

	if match := errorPattern.FindStringSubmatch(line); match != nil {
		err := &ExtractedError{
			Message: strings.TrimSpace(match[1]),
			Raw:     line,
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
				Message: "Exit code " + match[1],
				Raw:     line,
			}
		}
	}

	return nil
}
