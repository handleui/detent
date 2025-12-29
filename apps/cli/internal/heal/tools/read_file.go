package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	defaultReadLimit = 2000 // Default max lines to read
	maxLineLength    = 2000 // Truncate lines longer than this
)

// ReadFileTool reads source code from files in the worktree.
type ReadFileTool struct {
	ctx *Context
}

// NewReadFileTool creates a new read_file tool.
func NewReadFileTool(ctx *Context) *ReadFileTool {
	return &ReadFileTool{ctx: ctx}
}

// Name implements Tool.
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Description implements Tool.
func (t *ReadFileTool) Description() string {
	return "Read a file from the codebase. Returns file contents with line numbers. Use offset and limit for large files."
}

// InputSchema implements Tool.
func (t *ReadFileTool) InputSchema() map[string]any {
	return NewSchema().
		AddString("path", "File path relative to repository root").
		AddOptionalInteger("offset", "Line number to start reading from (1-indexed, default: 1)", 1).
		AddOptionalInteger("limit", "Maximum number of lines to read (default: 2000)", defaultReadLimit).
		Build()
}

type readFileInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// Execute implements Tool.
func (t *ReadFileTool) Execute(_ context.Context, input json.RawMessage) (Result, error) {
	var in readFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	// Validate required inputs
	if in.Path == "" {
		return ErrorResult("path is required"), nil
	}

	// Apply defaults
	if in.Offset <= 0 {
		in.Offset = 1
	}
	if in.Limit <= 0 {
		in.Limit = defaultReadLimit
	}

	// Validate path is within worktree
	absPath, errResult := t.ctx.ValidatePath(in.Path)
	if errResult != nil {
		return *errResult, nil
	}

	// Open file
	// #nosec G304 - path is validated to be within worktree
	file, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult("file not found: " + in.Path), nil
		}
		return ErrorResult("failed to open file: " + err.Error()), nil
	}
	defer func() { _ = file.Close() }()

	// Read with offset and limit
	var result strings.Builder
	scanner := bufio.NewScanner(file)

	// Increase scanner buffer to handle long lines (default is 64KB)
	// This allows reading lines up to 1MB before erroring
	const maxScannerBuffer = 1024 * 1024 // 1MB
	scanner.Buffer(make([]byte, 0, maxLineLength), maxScannerBuffer)

	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++

		// Skip lines before offset
		if lineNum < in.Offset {
			continue
		}

		// Stop after limit
		if linesRead >= in.Limit {
			result.WriteString(fmt.Sprintf("\n... (truncated at %d lines, use offset to read more)", in.Limit))
			break
		}

		line := scanner.Text()
		// Truncate long lines
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "..."
		}

		// Format with line numbers (cat -n style)
		result.WriteString(fmt.Sprintf("%6d\t%s\n", lineNum, line))
		linesRead++
	}

	if err := scanner.Err(); err != nil {
		return ErrorResult("error reading file: " + err.Error()), nil
	}

	if linesRead == 0 {
		if in.Offset > 1 {
			return ErrorResult(fmt.Sprintf("offset %d exceeds file length (%d lines)", in.Offset, lineNum)), nil
		}
		return SuccessResult("(empty file)"), nil
	}

	return SuccessResult(result.String()), nil
}
