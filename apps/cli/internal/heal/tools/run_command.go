package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	commandTimeout = 5 * time.Minute
	maxOutput      = 50 * 1024 // 50KB max output
)

// SafeCommands are always allowed without prompting.
// These are read-only or have minimal side effects.
var SafeCommands = map[string][]string{
	// Go - safe build/test/lint commands
	"go":            {"build", "test", "fmt", "vet", "mod", "generate", "install", "run"},
	"golangci-lint": {"run"},
	"gofumpt":       nil, // any args
	"goimports":     nil,
	"staticcheck":   nil,
	"govulncheck":   nil,

	// Node.js - safe commands
	"npm":  {"install", "ci", "test", "run"},
	"yarn": {"install", "test", "run"},
	"pnpm": {"install", "test", "run"},
	"bun":  {"install", "test", "run", "x"},
	"npx":  nil, // handled specially - linters are safe
	"bunx": nil,

	// Rust
	"cargo":   {"build", "test", "check", "fmt", "clippy", "run"},
	"rustfmt": nil,

	// Python
	"python":  {"-m"},
	"python3": {"-m"},
	"pip":     {"install"},
	"pip3":    {"install"},
	"pytest":  nil,
	"mypy":    nil,
	"ruff":    {"check", "format"},
	"black":   nil,

	// Linters/formatters (always safe)
	"eslint":   nil,
	"prettier": nil,
	"tsc":      nil,
	"biome":    {"check", "format", "lint"},
}

// SafeNpxCommands are npx/bunx commands that are always safe.
var SafeNpxCommands = map[string]bool{
	"eslint": true, "prettier": true, "biome": true, "oxlint": true,
	"tsc": true, "tsc-watch": true,
	"vitest": true, "jest": true,
	"turbo": true, "nx": true,
}

// BlockedPatterns are always rejected regardless of approval.
// Note: These are checked after normalizing whitespace.
var BlockedPatterns = []string{
	"rm -rf", "rm -r", "sudo", "chmod", "chown",
	"curl", "wget", "git push", "git remote", "git config",
	"ssh", "scp", "nc ", "netcat",
	"> /", ">>", "|", "&&", "||", ";",
	"$(", "`", "eval", "exec",
}

// BlockedCommands are base commands that are never allowed.
var BlockedCommands = map[string]bool{
	"rm": true, "sudo": true, "chmod": true, "chown": true,
	"curl": true, "wget": true, "ssh": true, "scp": true,
	"nc": true, "netcat": true, "eval": true, "exec": true,
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true,
}

// RunCommandTool executes commands with user approval for unknown ones.
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
	return "Run a shell command. Common build/test/lint commands are pre-approved. Other commands require user approval."
}

// InputSchema implements Tool.
func (t *RunCommandTool) InputSchema() map[string]any {
	return NewSchema().
		AddString("command", "The command to run (e.g., 'go test ./...', 'npm run lint', 'make build')").
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

	// Normalize whitespace to prevent bypass via tabs/multiple spaces
	normalizedCmd := normalizeCommand(in.Command)

	// Check blocked patterns first (on normalized command)
	for _, pattern := range BlockedPatterns {
		if strings.Contains(normalizedCmd, pattern) {
			return ErrorResult(fmt.Sprintf("blocked pattern: %q", pattern)), nil
		}
	}

	// Parse command
	parts := strings.Fields(normalizedCmd)
	if len(parts) == 0 {
		return ErrorResult("empty command"), nil
	}

	// Check if base command is explicitly blocked
	baseCmd := extractBaseCommand(parts[0])
	if BlockedCommands[baseCmd] {
		return ErrorResult(fmt.Sprintf("blocked command: %q", baseCmd)), nil
	}

	// Check if command is allowed
	if !t.isAllowed(normalizedCmd, parts) {
		return ErrorResult(fmt.Sprintf("command not approved: %s", normalizedCmd)), nil
	}

	// Execute the command with original parts (normalized)
	return t.execute(ctx, normalizedCmd, parts)
}

// normalizeCommand normalizes whitespace in a command string.
func normalizeCommand(cmd string) string {
	return strings.Join(strings.Fields(cmd), " ")
}

// extractBaseCommand extracts the base command name from a path.
// e.g., "/usr/bin/rm" -> "rm", "rm" -> "rm"
func extractBaseCommand(cmd string) string {
	// Handle path-based commands like /bin/rm or ./script.sh
	if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
		cmd = cmd[idx+1:]
	}
	return cmd
}

// isAllowed checks if a command is allowed to run.
func (t *RunCommandTool) isAllowed(fullCmd string, parts []string) bool {
	baseCmd := parts[0]
	subCmd := ""
	if len(parts) > 1 {
		subCmd = parts[1]
	}

	// 1. Check built-in safe commands
	if t.isSafeCommand(baseCmd, subCmd) {
		return true
	}

	// 2. Check local config commands
	if t.ctx.CommandChecker != nil && t.ctx.CommandChecker(fullCmd) {
		return true
	}

	// 3. Check session-approved commands
	if t.ctx.IsCommandApproved(fullCmd) {
		return true
	}

	// 4. Check if already denied this session
	if t.ctx.IsCommandDenied(fullCmd) {
		return false
	}

	// 5. Prompt user for approval
	if t.ctx.CommandApprover != nil {
		result, err := t.ctx.CommandApprover(fullCmd)
		if err != nil {
			return false
		}

		if !result.Allowed {
			t.ctx.DenyCommand(fullCmd)
			return false
		}

		// Persist if user chose "always"
		if result.Always && t.ctx.CommandPersister != nil {
			if persistErr := t.ctx.CommandPersister(fullCmd); persistErr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to save command: %v\n", persistErr)
			}
		}

		t.ctx.ApproveCommand(fullCmd)
		return true
	}

	return false
}

// isSafeCommand checks if a command matches the built-in safe list.
func (t *RunCommandTool) isSafeCommand(baseCmd, subCmd string) bool {
	// Extract base command name in case of path
	baseCmd = extractBaseCommand(baseCmd)

	allowedSubs, exists := SafeCommands[baseCmd]
	if !exists {
		return false
	}

	// nil means any subcommand is allowed
	if allowedSubs == nil {
		// Special handling for npx/bunx - check against safe list
		if baseCmd == "npx" || baseCmd == "bunx" {
			return SafeNpxCommands[subCmd]
		}
		return true
	}

	// Check if subcommand is in allowed list
	for _, allowed := range allowedSubs {
		if subCmd == allowed {
			return true
		}
	}

	return false
}

// execute runs the command and returns the result.
func (t *RunCommandTool) execute(ctx context.Context, fullCmd string, parts []string) (Result, error) {
	execCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	// #nosec G204 - command is validated
	cmd := exec.CommandContext(execCtx, parts[0], parts[1:]...)
	cmd.Dir = t.ctx.WorktreePath
	cmd.Env = safeCommandEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Build result
	var result strings.Builder
	result.WriteString(fmt.Sprintf("$ %s\n", fullCmd))
	result.WriteString(fmt.Sprintf("(completed in %s)\n\n", duration.Round(time.Millisecond)))

	output := stdout.String() + stderr.String()
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n... (truncated)"
	}

	if err != nil {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			result.WriteString("TIMEOUT: exceeded 5 minutes\n")
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

// safeCommandEnv returns a filtered environment for executing commands.
func safeCommandEnv() []string {
	allowedVars := []string{
		"PATH", "HOME", "USER", "TMPDIR", "TEMP", "TMP",
		"LANG", "LC_ALL", "LC_CTYPE", "SHELL", "TERM",
		"GOPATH", "GOROOT", "GOCACHE", "GOMODCACHE", "CGO_ENABLED",
		"NODE_ENV", "NODE_PATH", "NPM_CONFIG_CACHE",
		"CARGO_HOME", "RUSTUP_HOME",
		"JAVA_HOME", "MAVEN_HOME", "GRADLE_HOME",
	}

	blockedSuffixes := []string{
		"_KEY", "_TOKEN", "_SECRET", "_PASSWORD", "_CREDS", "_AUTH",
	}

	allowedSet := make(map[string]struct{}, len(allowedVars))
	for _, v := range allowedVars {
		allowedSet[v] = struct{}{}
	}

	var env []string
	for _, kv := range os.Environ() {
		idx := strings.Index(kv, "=")
		if idx <= 0 {
			continue
		}
		key := kv[:idx]
		upperKey := strings.ToUpper(key)

		// Block secrets
		blocked := false
		for _, suffix := range blockedSuffixes {
			if strings.HasSuffix(upperKey, suffix) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}

		if _, ok := allowedSet[key]; ok {
			env = append(env, kv)
		}
	}

	return env
}
