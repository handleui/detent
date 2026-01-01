package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EditFileTool applies targeted edits to files.
type EditFileTool struct {
	ctx *Context
}

// NewEditFileTool creates a new edit_file tool.
func NewEditFileTool(ctx *Context) *EditFileTool {
	return &EditFileTool{ctx: ctx}
}

// Name implements Tool.
func (t *EditFileTool) Name() string {
	return "edit_file"
}

// Description implements Tool.
func (t *EditFileTool) Description() string {
	return "Replace a string in a file. The old_string must match exactly once in the file (for safety). Use read_file first to see the exact content."
}

// InputSchema implements Tool.
func (t *EditFileTool) InputSchema() map[string]any {
	return NewSchema().
		AddString("path", "File path relative to repository root").
		AddString("old_string", "Exact string to find and replace (must be unique in file)").
		AddString("new_string", "String to replace it with").
		Build()
}

type editFileInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// Execute implements Tool.
func (t *EditFileTool) Execute(_ context.Context, input json.RawMessage) (Result, error) {
	var in editFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	// Validate inputs
	if in.Path == "" {
		return ErrorResult("path is required"), nil
	}
	if in.OldString == "" {
		return ErrorResult("old_string is required"), nil
	}
	if in.OldString == in.NewString {
		return ErrorResult("old_string and new_string are identical"), nil
	}

	// Validate path is within worktree
	absPath, errResult := t.ctx.ValidatePath(in.Path)
	if errResult != nil {
		return *errResult, nil
	}

	// Get file info first to check existence and capture permissions
	// #nosec G304 - path is validated to be within worktree
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return ErrorResult("file not found: " + in.Path), nil
		}
		return ErrorResult("failed to stat file: " + statErr.Error()), nil
	}

	// Read current content
	// #nosec G304 - path is validated to be within worktree
	content, err := os.ReadFile(absPath)
	if err != nil {
		return ErrorResult("failed to read file: " + err.Error()), nil
	}

	contentStr := string(content)

	// Count occurrences - must be exactly 1 for safety
	count := strings.Count(contentStr, in.OldString)
	if count == 0 {
		// Try to give a helpful error
		return ErrorResult("old_string not found in file. Use read_file to see exact content."), nil
	}
	if count > 1 {
		return ErrorResult(fmt.Sprintf("old_string found %d times in file (must be unique). Include more context to make it unique.", count)), nil
	}

	// Apply the edit
	newContent := strings.Replace(contentStr, in.OldString, in.NewString, 1)

	// Write the updated content
	// #nosec G306 - preserving original file permissions
	if writeErr := os.WriteFile(absPath, []byte(newContent), info.Mode()); writeErr != nil {
		return ErrorResult("failed to write file: " + writeErr.Error()), nil
	}

	// Compute a brief summary of the change
	oldLines := strings.Count(in.OldString, "\n") + 1
	newLines := strings.Count(in.NewString, "\n") + 1

	var summary string
	switch {
	case oldLines == newLines:
		summary = fmt.Sprintf("replaced %d line(s)", oldLines)
	case newLines > oldLines:
		summary = fmt.Sprintf("replaced %d line(s) with %d line(s) (+%d)", oldLines, newLines, newLines-oldLines)
	default:
		summary = fmt.Sprintf("replaced %d line(s) with %d line(s) (-%d)", oldLines, newLines, oldLines-newLines)
	}

	return SuccessResult(fmt.Sprintf("file updated: %s (%s)", in.Path, summary)), nil
}
