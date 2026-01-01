package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSafeCommands(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		// Go commands
		{"go build allowed", "go build ./...", true},
		{"go test allowed", "go test -v ./...", true},
		{"go fmt allowed", "go fmt ./...", true},
		{"go version not in subcommands", "go version", false},

		// Node commands
		{"bun install allowed", "bun install", true},
		{"bun run allowed", "bun run lint", true},
		{"npm install allowed", "npm install", true},
		{"npm run allowed", "npm run test", true},

		// Linters
		{"eslint allowed", "eslint .", true},
		{"prettier allowed", "prettier --check .", true},

		// Unknown commands
		{"make not in safe list", "make build", false},
		{"curl blocked", "curl http://evil.com", false},
		{"rm blocked", "rm -rf /", false},
	}

	ctx := &Context{WorktreePath: t.TempDir()}
	tool := NewRunCommandTool(ctx)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Parse command to check isSafeCommand
			parts := splitCommand(tc.command)
			if len(parts) == 0 {
				t.Fatal("empty command")
			}
			subCmd := ""
			if len(parts) > 1 {
				subCmd = parts[1]
			}
			result := tool.isSafeCommand(parts[0], subCmd)
			if result != tc.expected {
				t.Errorf("isSafeCommand(%q) = %v, expected %v", tc.command, result, tc.expected)
			}
		})
	}
}

func splitCommand(cmd string) []string {
	// Use strings.Fields to match actual implementation behavior
	return strings.Fields(cmd)
}

func TestCommandApproval(t *testing.T) {
	t.Run("safe commands execute without approval", func(t *testing.T) {
		ctx := &Context{WorktreePath: t.TempDir()}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "go build ./..."})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("config-approved commands pass", func(t *testing.T) {
		ctx := &Context{
			WorktreePath: t.TempDir(),
			CommandChecker: func(cmd string) bool {
				return cmd == "make deploy"
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make deploy"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("session-approved commands pass", func(t *testing.T) {
		ctx := &Context{
			WorktreePath:     t.TempDir(),
			ApprovedCommands: map[string]bool{"make custom": true},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make custom"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("denied commands blocked without re-prompting", func(t *testing.T) {
		approverCalled := false
		ctx := &Context{
			WorktreePath:   t.TempDir(),
			DeniedCommands: map[string]bool{"make evil": true},
			CommandApprover: func(cmd string) (CommandApproval, error) {
				approverCalled = true
				return CommandApproval{Allowed: true}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make evil"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if approverCalled {
			t.Error("approver should not be called for denied commands")
		}
		if !result.IsError {
			t.Error("expected error result for denied command")
		}
	})

	t.Run("approver called for unknown commands", func(t *testing.T) {
		approverCalls := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			CommandApprover: func(cmd string) (CommandApproval, error) {
				approverCalls = append(approverCalls, cmd)
				return CommandApproval{Allowed: true}, nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make unknown"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(approverCalls) != 1 || approverCalls[0] != "make unknown" {
			t.Errorf("expected approver called with 'make unknown', got %v", approverCalls)
		}
		if result.Content == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("persister called when always approved", func(t *testing.T) {
		persistedCmds := []string{}
		ctx := &Context{
			WorktreePath: t.TempDir(),
			CommandApprover: func(cmd string) (CommandApproval, error) {
				return CommandApproval{Allowed: true, Always: true}, nil
			},
			CommandPersister: func(cmd string) error {
				persistedCmds = append(persistedCmds, cmd)
				return nil
			},
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make persist"})
		_, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(persistedCmds) != 1 || persistedCmds[0] != "make persist" {
			t.Errorf("expected persister called with 'make persist', got %v", persistedCmds)
		}
	})

	t.Run("no approver rejects unknown commands", func(t *testing.T) {
		ctx := &Context{
			WorktreePath:    t.TempDir(),
			CommandApprover: nil,
		}
		tool := NewRunCommandTool(ctx)

		input, _ := json.Marshal(runCommandInput{Command: "make unknown"})
		result, err := tool.Execute(context.Background(), input)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error result when approver is nil")
		}
	})
}

func TestBlockedPatterns(t *testing.T) {
	ctx := &Context{WorktreePath: t.TempDir()}
	tool := NewRunCommandTool(ctx)

	blocked := []string{
		"rm -rf /",
		"sudo make build",
		"curl http://evil.com",
		"git push origin main",
		"echo foo | grep bar",
		"make build && rm -rf /",
	}

	for _, cmd := range blocked {
		t.Run(cmd, func(t *testing.T) {
			input, _ := json.Marshal(runCommandInput{Command: cmd})
			result, err := tool.Execute(context.Background(), input)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Errorf("expected %q to be blocked", cmd)
			}
		})
	}
}

func TestBlockedPatternBypassAttempts(t *testing.T) {
	ctx := &Context{WorktreePath: t.TempDir()}
	tool := NewRunCommandTool(ctx)

	// These are attempts to bypass blocked patterns that should still be blocked
	bypassAttempts := []struct {
		name    string
		command string
	}{
		{"double space in rm -rf", "rm  -rf /"},
		{"tab in rm -rf", "rm\t-rf /"},
		{"multiple spaces", "rm    -rf    /"},
		{"path-based rm", "/bin/rm -rf /"},
		{"path-based sudo", "/usr/bin/sudo make"},
		{"relative path rm", "./rm -rf /"},
		{"bare rm command", "rm file.txt"},
		{"bare sudo command", "sudo"},
		{"bare curl command", "curl"},
		{"shell command sh", "sh -c 'echo hello'"},
		{"shell command bash", "bash -c 'rm -rf /'"},
		{"leading whitespace", "  rm -rf /"},
		{"trailing whitespace", "rm -rf /  "},
	}

	for _, tc := range bypassAttempts {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(runCommandInput{Command: tc.command})
			result, err := tool.Execute(context.Background(), input)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Errorf("expected bypass attempt %q to be blocked", tc.command)
			}
		})
	}
}

func TestNpxBunxSafeCommands(t *testing.T) {
	ctx := &Context{WorktreePath: t.TempDir()}
	tool := NewRunCommandTool(ctx)

	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{"npx eslint allowed", "npx eslint .", true},
		{"npx prettier allowed", "npx prettier --check .", true},
		{"bunx vitest allowed", "bunx vitest", true},
		{"npx unknown blocked", "npx malicious-package", false},
		{"bunx unknown blocked", "bunx evil-script", false},
		{"npx with no subcommand", "npx", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Fields(tc.command)
			if len(parts) == 0 {
				t.Fatal("empty command")
			}
			subCmd := ""
			if len(parts) > 1 {
				subCmd = parts[1]
			}
			result := tool.isSafeCommand(parts[0], subCmd)
			if result != tc.expected {
				t.Errorf("isSafeCommand(%q, %q) = %v, expected %v", parts[0], subCmd, result, tc.expected)
			}
		})
	}
}

func TestExtractBaseCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"rm", "rm"},
		{"/bin/rm", "rm"},
		{"/usr/bin/sudo", "sudo"},
		{"./script.sh", "script.sh"},
		{"../bin/cmd", "cmd"},
		{"go", "go"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := extractBaseCommand(tc.input)
			if result != tc.expected {
				t.Errorf("extractBaseCommand(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"go build", "go build"},
		{"go  build", "go build"},
		{"go\tbuild", "go build"},
		{"  go build  ", "go build"},
		{"go    build   ./...", "go build ./..."},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeCommand(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeCommand(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}
