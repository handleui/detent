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
	"npx": {AllowedSubcommands: []string{
		// Linters and formatters
		"eslint", "prettier", "biome", "oxlint",
		// TypeScript
		"tsc", "tsc-watch",
		// Test runners
		"vitest", "jest",
		// Monorepo tools
		"turbo", "nx",
		// Common utilities
		"rimraf", "nodemon",
	}},

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
	"python":  {AllowedSubcommands: []string{"-m"}},
	"python3": {AllowedSubcommands: []string{"-m"}},
	"pip":     {AllowedSubcommands: []string{"install"}},
	"pip3":    {AllowedSubcommands: []string{"install"}},
	"pytest":  {AllowedSubcommands: nil},
	"mypy":    {AllowedSubcommands: nil},
	"ruff":    {AllowedSubcommands: []string{"check", "format"}},
	"black":   {AllowedSubcommands: nil},

	// Build tools
	//
	// SECURITY NOTE: Make, Gradle, Maven execute project-defined targets/tasks.
	// Unlike fixed-verb tools (go, cargo), these cannot have restricted subcommands
	// because target names are project-specific (e.g., "make lint", "make deploy-prod").
	//
	// ACCEPTED RISK: A malicious Makefile/build.gradle could contain dangerous targets.
	// MITIGATIONS:
	//   1. Worktree sandbox isolates file operations
	//   2. Environment filtering (safeCommandEnv) prevents credential exfiltration
	//   3. Users run detent on trusted codebases (same trust model as "npm install")
	//
	"make":   {AllowedSubcommands: nil},
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

// allowedMakeTargets are auto-allowed for any trusted repository.
// Unknown targets require user approval (prompted via TargetApprover).
var allowedMakeTargets = map[string]bool{
	// Build & compile
	"all": true, "build": true, "compile": true, "install": true,
	// Testing
	"test": true, "check": true, "verify": true, "validate": true,
	// Code quality
	"lint": true, "fmt": true, "format": true, "vet": true, "staticcheck": true,
	// Cleaning
	"clean": true, "distclean": true,
	// Dependencies
	"deps": true, "vendor": true, "mod": true, "tidy": true,
	// Generation
	"generate": true, "gen": true, "proto": true,
	// Run
	"run": true, "dev": true, "serve": true,
}

// isAllowedMakeTarget checks if a make target is in the allowlist.
// Case-insensitive: "BUILD" and "build" are treated the same.
func isAllowedMakeTarget(target string) bool {
	return allowedMakeTargets[strings.ToLower(target)]
}

// allowedEnvVars is an allowlist of environment variable names that are safe to pass
// to executed commands. Uses exact matches for specific variables.
var allowedEnvVars = []string{
	// Essential system variables
	"PATH",
	"HOME",
	"USER",
	"TMPDIR",
	"TEMP",
	"TMP",
	"LANG",
	"SHELL",
	"TERM",
	"COLORTERM",
	"FORCE_COLOR",
	"NO_COLOR",
	"CLICOLOR",
	"CLICOLOR_FORCE",

	// Go
	"GOPATH",
	"GOROOT",
	"GOBIN",
	"GOCACHE",
	"GOMODCACHE",
	"GOPROXY",
	"GOPRIVATE",
	"GOFLAGS",
	"CGO_ENABLED",
	"CGO_CFLAGS",
	"CGO_LDFLAGS",

	// Node.js / JavaScript
	"NODE_ENV",
	"NODE_PATH",
	"NODE_OPTIONS",
	"NPM_CONFIG_REGISTRY",
	"NPM_CONFIG_CACHE",
	"YARN_CACHE_FOLDER",
	"PNPM_HOME",
	"BUN_INSTALL",

	// Python
	"PYTHONPATH",
	"PYTHONHOME",
	"VIRTUAL_ENV",
	"CONDA_PREFIX",
	"CONDA_DEFAULT_ENV",
	"PIPENV_VENV_IN_PROJECT",

	// Rust
	"CARGO_HOME",
	"RUSTUP_HOME",
	"RUSTFLAGS",
	"CARGO_TARGET_DIR",

	// Java / JVM
	"JAVA_HOME",
	"JDK_HOME",
	"MAVEN_HOME",
	"M2_HOME",
	"GRADLE_HOME",
	"GRADLE_USER_HOME",

	// Ruby
	"GEM_HOME",
	"GEM_PATH",
	"BUNDLE_PATH",
	"RBENV_ROOT",
	"RUBY_VERSION",

	// Build tools
	"CC",
	"CXX",
	"CFLAGS",
	"CXXFLAGS",
	"LDFLAGS",
	"PKG_CONFIG_PATH",
	"CMAKE_PREFIX_PATH",

	// Editor integration (for some tools)
	"EDITOR",
	"VISUAL",
}

// allowedEnvPrefixes is an allowlist of environment variable prefixes that are safe.
// Variables starting with these prefixes will be included (unless they match a blocked suffix).
var allowedEnvPrefixes = []string{
	"LC_", // Locale settings (LC_ALL, LC_CTYPE, LC_MESSAGES, etc.)
	"XDG_", // XDG base directory specification
}

// blockedEnvSuffixes are suffixes that indicate secrets, even if the prefix is allowed.
// Any variable ending with these will be excluded.
var blockedEnvSuffixes = []string{
	"_KEY",
	"_TOKEN",
	"_SECRET",
	"_PASSWORD",
	"_CREDS",
	"_CREDENTIAL",
	"_CREDENTIALS",
	"_PRIVATE_KEY",
	"_SIGNING_KEY",
	"_AUTH",
	"_API_KEY",
}

// safeCommandEnv returns a filtered environment for executing commands.
// Uses an allowlist approach to prevent secrets from being exposed to executed commands.
//
// This function:
// 1. Includes only explicitly allowed environment variables
// 2. Includes variables with allowed prefixes (like LC_*)
// 3. Excludes any variable with a blocked suffix (like _TOKEN, _SECRET)
//
// This prevents malicious code from exfiltrating API keys, tokens, and other secrets
// that may be present in the parent environment.
func safeCommandEnv() []string {
	// Build a set of allowed exact variable names for O(1) lookup
	allowedSet := make(map[string]struct{}, len(allowedEnvVars))
	for _, v := range allowedEnvVars {
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

		// Check if the variable has a blocked suffix (secrets)
		blocked := false
		for _, suffix := range blockedEnvSuffixes {
			if strings.HasSuffix(upperKey, suffix) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}

		// Check if exact match in allowlist
		if _, ok := allowedSet[key]; ok {
			env = append(env, kv)
			continue
		}

		// Check if matches an allowed prefix
		for _, prefix := range allowedEnvPrefixes {
			if strings.HasPrefix(key, prefix) {
				env = append(env, kv)
				break
			}
		}
	}

	return env
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

	// Parse command using simple space-splitting.
	// NOTE: This doesn't handle quoted arguments (e.g., `npm run "build --prod"`).
	// For security, we intentionally avoid shell-like parsing to prevent escaping attacks.
	// Commands requiring complex quoting should use multiple simple commands instead.
	parts := strings.Fields(in.Command)
	if len(parts) == 0 {
		return ErrorResult("empty command"), nil
	}

	baseCmd := parts[0]
	spec, allowed := CommandWhitelist[baseCmd]
	if !allowed {
		// Check local config allowlist
		if t.ctx.LocalCommandChecker != nil && t.ctx.LocalCommandChecker(in.Command) {
			allowed = true
		}
	}
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

	// For make commands, validate ALL targets against allowlist.
	// make can run multiple targets: `make build test deploy`
	// We must validate each one, not just the first.
	//
	// EXTENSION POINT: gradle/mvn task approval
	// gradle and mvn also have project-defined tasks (like make targets) that need validation.
	// When implementing, extract this to a TargetValidator interface with per-tool implementations.
	if baseCmd == "make" && len(parts) > 1 {
		for _, arg := range parts[1:] {
			// Skip flags (start with -)
			if strings.HasPrefix(arg, "-") {
				continue
			}
			// Skip variable assignments (contain =) - these modify behavior, not run targets
			// e.g., `make CC=gcc build` - CC=gcc is not a target
			if strings.Contains(arg, "=") {
				continue
			}

			target := arg

			// Check in order: hardcoded allowlist → local config → per-repo config → session approved → session denied
			if isAllowedMakeTarget(target) {
				continue
			}
			if t.ctx.LocalTargetChecker != nil && t.ctx.LocalTargetChecker(target) {
				continue
			}
			if t.ctx.RepoTargetChecker != nil && t.ctx.RepoTargetChecker(target) {
				continue
			}
			if t.ctx.IsTargetApproved(target) {
				continue
			}
			// Check if already denied this session - don't prompt again
			if t.ctx.IsTargetDenied(target) {
				return ErrorResult(fmt.Sprintf("make target %q was denied earlier in this session", target)), nil
			}

			// Not approved anywhere - ask user
			if t.ctx.TargetApprover != nil {
				result, approveErr := t.ctx.TargetApprover(target)
				if approveErr != nil {
					return ErrorResult(fmt.Sprintf("target approval failed: %v", approveErr)), nil
				}
				if !result.Allowed {
					t.ctx.DenyTarget(target) // Remember denial to prevent retry
					return ErrorResult(fmt.Sprintf("make target %q denied by user", target)), nil
				}

				// Persist if user chose "always for this repo"
				if result.Always && t.ctx.TargetPersister != nil {
					if persistErr := t.ctx.TargetPersister(target); persistErr != nil {
						// Log but don't fail - session approval still works
						fmt.Fprintf(os.Stderr, "warning: failed to persist target approval: %v\n", persistErr)
					}
				}

				t.ctx.ApproveTarget(target) // Remember for this session
			} else {
				return ErrorResult(fmt.Sprintf("make target %q not in allowlist (allowed: all, build, test, lint, fmt, check, clean, etc.)", target)), nil
			}
		}
	}

	// Create command with timeout
	execCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	// #nosec G204 - command is validated against whitelist and blocked patterns
	cmd := exec.CommandContext(execCtx, parts[0], parts[1:]...)
	// SECURITY NOTE: Symlinks inside the worktree are not validated. Commands could
	// follow symlinks outside the worktree. Accepted limitation (same as any build tool).
	cmd.Dir = t.ctx.WorktreePath
	cmd.Env = safeCommandEnv()

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
