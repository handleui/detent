package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	grepTimeout    = 30 * time.Second
	maxGrepOutput  = 50 * 1024 // 50KB max output
	maxGrepMatches = 100       // Max matches to return
)

// File type mappings for ripgrep --type flag
var fileTypeMap = map[string]string{
	"go":         "go",
	"ts":         "ts",
	"typescript": "ts",
	"js":         "js",
	"javascript": "js",
	"py":         "py",
	"python":     "py",
	"rust":       "rust",
	"rs":         "rust",
	"java":       "java",
	"c":          "c",
	"cpp":        "cpp",
	"css":        "css",
	"html":       "html",
	"json":       "json",
	"yaml":       "yaml",
	"yml":        "yaml",
	"md":         "md",
	"markdown":   "md",
}

// GrepTool searches for patterns in code.
type GrepTool struct {
	ctx *Context
}

// NewGrepTool creates a new grep tool.
func NewGrepTool(ctx *Context) *GrepTool {
	return &GrepTool{ctx: ctx}
}

// Name implements Tool.
func (t *GrepTool) Name() string {
	return "grep"
}

// Description implements Tool.
func (t *GrepTool) Description() string {
	return "Search for a pattern in code using ripgrep. Returns matching lines with file paths and line numbers."
}

// InputSchema implements Tool.
func (t *GrepTool) InputSchema() map[string]any {
	return NewSchema().
		AddString("pattern", "Regular expression pattern to search for").
		AddOptionalString("path", "Directory or file to search in (relative to repo root, default: entire repo)").
		AddOptionalString("type", "File type filter (e.g., 'go', 'ts', 'py', 'rust', 'js')").
		Build()
}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Type    string `json:"type"`
}

// Execute implements Tool.
func (t *GrepTool) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	if in.Pattern == "" {
		return ErrorResult("pattern is required"), nil
	}

	// Determine search path
	searchPath := t.ctx.WorktreePath
	if in.Path != "" {
		absPath, errResult := t.ctx.ValidatePath(in.Path)
		if errResult != nil {
			return *errResult, nil
		}
		searchPath = absPath
	}

	// Build ripgrep command
	args := []string{
		"--line-number",
		"--no-heading",
		"--color=never",
		fmt.Sprintf("--max-count=%d", maxGrepMatches),
	}

	// Add file type filter if specified
	if in.Type != "" {
		rgType, ok := fileTypeMap[strings.ToLower(in.Type)]
		if ok {
			args = append(args, "--type", rgType)
		} else {
			return ErrorResult(fmt.Sprintf("unknown file type: %s (supported: go, ts, js, py, rust, java, c, cpp, css, html, json, yaml, md)", in.Type)), nil
		}
	}

	// Add pattern and path
	args = append(args, "--", in.Pattern, searchPath)

	// Create command with timeout
	execCtx, cancel := context.WithTimeout(ctx, grepTimeout)
	defer cancel()

	// #nosec G204 - args are constructed from validated input, pattern is regex not shell
	cmd := exec.CommandContext(execCtx, "rg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Handle exit codes
	// Exit code 1 means no matches (not an error)
	// Exit code 2 means error
	if err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return ErrorResult("search timed out after 30 seconds"), nil
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				// No matches found
				return SuccessResult("no matches found for pattern: " + in.Pattern), nil
			}
			if exitErr.ExitCode() == 2 {
				// Error in pattern or execution
				return ErrorResult("grep error: " + stderr.String()), nil
			}
		}

		// Check if ripgrep is not installed
		var execErr *exec.Error
		if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return ErrorResult("ripgrep (rg) not found - please install it for code search"), nil
		}

		return ErrorResult("grep failed: " + err.Error()), nil
	}

	output := stdout.String()

	// Truncate if too large
	if len(output) > maxGrepOutput {
		output = output[:maxGrepOutput] + "\n... (truncated, refine your pattern for more specific matches)"
	}

	// Make paths relative to worktree for cleaner output
	output = strings.ReplaceAll(output, t.ctx.WorktreePath+"/", "")

	return SuccessResult(output), nil
}
