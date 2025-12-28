package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	verifyTimeout = 5 * time.Minute
	maxOutput     = 50 * 1024 // 50KB max output
)

// CategoryCommand maps error categories to verification commands.
type CategoryCommand struct {
	Name    string   // Human-readable name
	Command []string // Command to run
	WorkDir string   // Subdirectory to run in (relative to worktree), empty for root
}

// CategoryCommands maps error categories to their verification commands.
// These are the exact commands that would run in CI.
var CategoryCommands = map[string]CategoryCommand{
	// Go commands
	"go-lint": {
		Name:    "Go Lint",
		Command: []string{"golangci-lint", "run", "./..."},
		WorkDir: "apps/cli",
	},
	"go-test": {
		Name:    "Go Test",
		Command: []string{"go", "test", "-v", "./..."},
		WorkDir: "apps/cli",
	},
	"go-build": {
		Name:    "Go Build",
		Command: []string{"go", "build", "./..."},
		WorkDir: "apps/cli",
	},

	// TypeScript/JavaScript commands
	"ts-lint": {
		Name:    "TypeScript Lint",
		Command: []string{"bun", "run", "lint"},
	},
	"ts-typecheck": {
		Name:    "TypeScript Type Check",
		Command: []string{"bun", "run", "check-types"},
	},
	"ts-test": {
		Name:    "TypeScript Test",
		Command: []string{"bun", "run", "test"},
	},

	// Build commands
	"build": {
		Name:    "Build",
		Command: []string{"bun", "run", "build"},
	},
}

// VerifyTool runs verification commands by error category.
type VerifyTool struct {
	ctx *Context
}

// NewVerifyTool creates a new run_check tool.
func NewVerifyTool(ctx *Context) *VerifyTool {
	return &VerifyTool{ctx: ctx}
}

// Name implements Tool.
func (t *VerifyTool) Name() string {
	return "run_check"
}

// Description implements Tool.
func (t *VerifyTool) Description() string {
	categories := sortedCategories()
	return fmt.Sprintf("Run a verification command for an error category. Available categories: %s", strings.Join(categories, ", "))
}

// sortedCategories returns category names in sorted order for consistent output.
func sortedCategories() []string {
	categories := make([]string, 0, len(CategoryCommands))
	for cat := range CategoryCommands {
		categories = append(categories, cat)
	}
	slices.Sort(categories)
	return categories
}

// InputSchema implements Tool.
func (t *VerifyTool) InputSchema() map[string]any {
	return NewSchema().
		AddEnum("category", "Error category to verify", sortedCategories()).
		Build()
}

type verifyInput struct {
	Category string `json:"category"`
}

// Execute implements Tool.
func (t *VerifyTool) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in verifyInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	catCmd, ok := CategoryCommands[in.Category]
	if !ok {
		return ErrorResult(fmt.Sprintf("unknown category: %s (available: %s)", in.Category, strings.Join(sortedCategories(), ", "))), nil
	}

	// Determine working directory
	workDir := t.ctx.WorktreePath
	if catCmd.WorkDir != "" {
		workDir = filepath.Join(t.ctx.WorktreePath, catCmd.WorkDir)
	}

	// Create command with timeout
	execCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	// #nosec G204 - command is from predefined CategoryCommands, not user input
	cmd := exec.CommandContext(execCtx, catCmd.Command[0], catCmd.Command[1:]...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Build result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("=== %s ===\n", catCmd.Name))
	result.WriteString(fmt.Sprintf("Command: %s\n", strings.Join(catCmd.Command, " ")))
	result.WriteString(fmt.Sprintf("Duration: %s\n\n", duration.Round(time.Millisecond)))

	// Combine output
	output := stdout.String() + stderr.String()

	// Truncate if too large
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n... (output truncated)"
	}

	if err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			result.WriteString("TIMEOUT: Command did not complete within 5 minutes\n")
			return Result{Content: result.String(), IsError: true}, nil
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.WriteString(fmt.Sprintf("EXIT CODE: %d (errors found)\n\n", exitErr.ExitCode()))
		} else {
			result.WriteString(fmt.Sprintf("ERROR: %s\n\n", err.Error()))
		}

		result.WriteString("OUTPUT:\n")
		result.WriteString(output)

		return Result{Content: result.String(), IsError: true}, nil
	}

	result.WriteString("PASSED: No errors found\n")
	if output != "" {
		result.WriteString("\nOUTPUT:\n")
		result.WriteString(output)
	}

	return SuccessResult(result.String()), nil
}
