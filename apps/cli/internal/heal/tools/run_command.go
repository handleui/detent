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
	commandTimeout = 5 * time.Minute
)

// CommandSpec defines allowed subcommands for a base command.
type CommandSpec struct {
	// AllowedSubcommands lists allowed subcommands. If nil, any args are allowed.
	AllowedSubcommands []string
}

// CommandWhitelist maps base commands to their specifications.
var CommandWhitelist = map[string]CommandSpec{
	// Node.js package managers
	"npm":  {AllowedSubcommands: []string{"run", "test", "install", "ci", "build"}},
	"yarn": {AllowedSubcommands: []string{"run", "test", "install", "build"}},
	"pnpm": {AllowedSubcommands: []string{"run", "test", "install", "build"}},
	"bun":  {AllowedSubcommands: []string{"run", "test", "install", "build", "x"}},
	"npx":  {AllowedSubcommands: nil}, // Any args allowed for npx

	// Go
	"go":             {AllowedSubcommands: []string{"build", "test", "run", "fmt", "vet", "mod", "generate", "install"}},
	"golangci-lint":  {AllowedSubcommands: []string{"run"}},
	"gofumpt":        {AllowedSubcommands: nil},
	"goimports":      {AllowedSubcommands: nil},
	"staticcheck":    {AllowedSubcommands: nil},
	"govulncheck":    {AllowedSubcommands: nil},

	// Rust
	"cargo":  {AllowedSubcommands: []string{"build", "test", "check", "fmt", "clippy", "run"}},
	"rustfmt": {AllowedSubcommands: nil},

	// Python
	"python":  {AllowedSubcommands: []string{"-m", "-c"}},
	"python3": {AllowedSubcommands: []string{"-m", "-c"}},
	"pip":     {AllowedSubcommands: []string{"install"}},
	"pip3":    {AllowedSubcommands: []string{"install"}},
	"pytest":  {AllowedSubcommands: nil},
	"mypy":    {AllowedSubcommands: nil},
	"ruff":    {AllowedSubcommands: []string{"check", "format"}},
	"black":   {AllowedSubcommands: nil},

	// Build tools
	"make":   {AllowedSubcommands: nil}, // Any target allowed
	"cmake":  {AllowedSubcommands: nil},
	"gradle": {AllowedSubcommands: nil},
	"mvn":    {AllowedSubcommands: nil},

	// Linters/formatters
	"eslint":   {AllowedSubcommands: nil},
	"prettier": {AllowedSubcommands: nil},
	"tsc":      {AllowedSubcommands: nil},
	"biome":    {AllowedSubcommands: []string{"check", "format", "lint"}},
}

// BlockedPatterns are always rejected regardless of whitelist.
var BlockedPatterns = []string{
	"rm -rf",
	"rm -r",
	"sudo",
	"chmod",
	"chown",
	"curl",
	"wget",
	"git push",
	"git remote",
	"git config",
	"ssh",
	"scp",
	"nc ",
	"netcat",
	"> /",
	">>",
	"|",
	"&&",
	"||",
	";",
	"$(",
	"`",
	"eval",
	"exec",
}

// RunCommandTool executes whitelisted commands.
type RunCommandTool struct {
	ctx *Context
}

// NewRunCommandTool creates a new run_command tool.
func NewRunCommandTool(ctx *Context) *RunCommandTool {
	return &RunCommandTool{ctx: ctx}
}

// Name implements Tool.
func (t *RunCommandTool) Name() string {
	return "run_command"
}

// Description implements Tool.
func (t *RunCommandTool) Description() string {
	cmds := make([]string, 0, len(CommandWhitelist))
	for cmd := range CommandWhitelist {
		cmds = append(cmds, cmd)
	}
	return fmt.Sprintf("Run a whitelisted command. Allowed commands: %s. Shell operators (|, &&, ||, ;) are not allowed.", strings.Join(cmds, ", "))
}

// InputSchema implements Tool.
func (t *RunCommandTool) InputSchema() map[string]any {
	return NewSchema().
		AddString("command", "The command to run (e.g., 'go test ./...', 'npm run lint')").
		Build()
}

type runCommandInput struct {
	Command string `json:"command"`
}

// Execute implements Tool.
func (t *RunCommandTool) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var in runCommandInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ErrorResult("invalid input: " + err.Error()), nil
	}

	if in.Command == "" {
		return ErrorResult("command is required"), nil
	}

	// Check blocked patterns first (security critical)
	for _, pattern := range BlockedPatterns {
		if strings.Contains(in.Command, pattern) {
			return ErrorResult(fmt.Sprintf("command contains blocked pattern: %q", pattern)), nil
		}
	}

	// Parse command
	parts := strings.Fields(in.Command)
	if len(parts) == 0 {
		return ErrorResult("empty command"), nil
	}

	baseCmd := parts[0]
	spec, allowed := CommandWhitelist[baseCmd]
	if !allowed {
		return ErrorResult(fmt.Sprintf("command %q not in whitelist", baseCmd)), nil
	}

	// Validate subcommand if required
	if len(spec.AllowedSubcommands) > 0 {
		if len(parts) < 2 {
			return ErrorResult(fmt.Sprintf("command %q requires a subcommand (allowed: %s)",
				baseCmd, strings.Join(spec.AllowedSubcommands, ", "))), nil
		}
		subCmd := parts[1]
		valid := false
		for _, allowedSub := range spec.AllowedSubcommands {
			if subCmd == allowedSub {
				valid = true
				break
			}
		}
		if !valid {
			return ErrorResult(fmt.Sprintf("subcommand %q not allowed for %s (allowed: %s)",
				subCmd, baseCmd, strings.Join(spec.AllowedSubcommands, ", "))), nil
		}
	}

	// Create command with timeout
	execCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	// #nosec G204 - command is validated against whitelist and blocked patterns
	cmd := exec.CommandContext(execCtx, parts[0], parts[1:]...)
	cmd.Dir = t.ctx.WorktreePath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Build result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("$ %s\n", in.Command))
	result.WriteString(fmt.Sprintf("(completed in %s)\n\n", duration.Round(time.Millisecond)))

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
			result.WriteString(fmt.Sprintf("Exit code: %d\n\n", exitErr.ExitCode()))
		} else {
			result.WriteString(fmt.Sprintf("Error: %s\n\n", err.Error()))
		}

		result.WriteString(output)
		return Result{Content: result.String(), IsError: true}, nil
	}

	result.WriteString(output)
	return SuccessResult(result.String()), nil
}
