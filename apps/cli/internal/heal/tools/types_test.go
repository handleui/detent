package tools

import (
	"strings"
	"testing"
)

func TestContext_IsCommandApproved(t *testing.T) {
	tests := []struct {
		name     string
		approved map[string]bool
		cmd      string
		expected bool
	}{
		{"nil map returns false", nil, "make build", false},
		{"empty map returns false", map[string]bool{}, "make build", false},
		{"cmd found returns true", map[string]bool{"make build": true}, "make build", true},
		{"cmd not found returns false", map[string]bool{"make test": true}, "make build", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{ApprovedCommands: tt.approved}
			if got := c.IsCommandApproved(tt.cmd); got != tt.expected {
				t.Errorf("IsCommandApproved(%q) = %v, want %v", tt.cmd, got, tt.expected)
			}
		})
	}
}

func TestContext_ApproveCommand(t *testing.T) {
	t.Run("lazy init nil map", func(t *testing.T) {
		c := &Context{}
		c.ApproveCommand("make build")
		if !c.ApprovedCommands["make build"] {
			t.Error("expected command to be approved")
		}
	})

	t.Run("preserves existing", func(t *testing.T) {
		c := &Context{ApprovedCommands: map[string]bool{"existing": true}}
		c.ApproveCommand("new")
		if !c.ApprovedCommands["existing"] || !c.ApprovedCommands["new"] {
			t.Error("expected both commands to be approved")
		}
	})
}

func TestContext_IsCommandDenied(t *testing.T) {
	tests := []struct {
		name     string
		denied   map[string]bool
		cmd      string
		expected bool
	}{
		{"nil map returns false", nil, "make build", false},
		{"cmd found returns true", map[string]bool{"make build": true}, "make build", true},
		{"cmd not found returns false", map[string]bool{"make test": true}, "make build", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{DeniedCommands: tt.denied}
			if got := c.IsCommandDenied(tt.cmd); got != tt.expected {
				t.Errorf("IsCommandDenied(%q) = %v, want %v", tt.cmd, got, tt.expected)
			}
		})
	}
}

func TestContext_DenyCommand(t *testing.T) {
	t.Run("lazy init nil map", func(t *testing.T) {
		c := &Context{}
		c.DenyCommand("make evil")
		if !c.DeniedCommands["make evil"] {
			t.Error("expected command to be denied")
		}
	})
}

func TestContext_ValidatePath(t *testing.T) {
	tests := []struct {
		name         string
		worktreePath string
		relPath      string
		wantPath     string
		wantError    bool
		errorContain string
	}{
		{"valid simple path", "/repo", "src/main.go", "/repo/src/main.go", false, ""},
		{"valid path with dots", "/repo", "src/../lib/util.go", "/repo/lib/util.go", false, ""},
		{"parent escape blocked", "/repo", "../secret", "", true, "path escapes worktree"},
		{"absolute path blocked", "/repo", "/etc/passwd", "", true, "absolute paths not allowed"},
		{"empty path resolves to worktree", "/repo", "", "/repo", false, ""},
		{"dot path resolves to worktree", "/repo", ".", "/repo", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{WorktreePath: tt.worktreePath}
			gotPath, errResult := c.ValidatePath(tt.relPath)

			if tt.wantError {
				if errResult == nil {
					t.Errorf("ValidatePath(%q) expected error, got path %q", tt.relPath, gotPath)
					return
				}
				if tt.errorContain != "" && !strings.Contains(errResult.Content, tt.errorContain) {
					t.Errorf("error = %q, want to contain %q", errResult.Content, tt.errorContain)
				}
				return
			}

			if errResult != nil {
				t.Errorf("ValidatePath(%q) unexpected error: %s", tt.relPath, errResult.Content)
				return
			}
			if gotPath != tt.wantPath {
				t.Errorf("ValidatePath(%q) = %q, want %q", tt.relPath, gotPath, tt.wantPath)
			}
		})
	}
}
