package tools

import (
	"strings"
	"testing"
)

func TestContext_IsTargetApproved(t *testing.T) {
	tests := []struct {
		name     string
		approved map[string]bool
		target   string
		expected bool
	}{
		{
			name:     "nil map returns false",
			approved: nil,
			target:   "test",
			expected: false,
		},
		{
			name:     "empty map returns false",
			approved: map[string]bool{},
			target:   "test",
			expected: false,
		},
		{
			name:     "target found returns true",
			approved: map[string]bool{"test": true},
			target:   "test",
			expected: true,
		},
		{
			name:     "target not found returns false",
			approved: map[string]bool{"other": true},
			target:   "test",
			expected: false,
		},
		{
			name:     "case insensitive uppercase query",
			approved: map[string]bool{"test": true},
			target:   "TEST",
			expected: true,
		},
		{
			name:     "case insensitive mixed case query",
			approved: map[string]bool{"test": true},
			target:   "TeSt",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{ApprovedTargets: tt.approved}
			if got := c.IsTargetApproved(tt.target); got != tt.expected {
				t.Errorf("IsTargetApproved(%q) = %v, want %v", tt.target, got, tt.expected)
			}
		})
	}
}

func TestContext_ApproveTarget(t *testing.T) {
	tests := []struct {
		name           string
		initialTargets map[string]bool
		target         string
		checkTarget    string
		expected       bool
	}{
		{
			name:           "lazy init nil map",
			initialTargets: nil,
			target:         "test",
			checkTarget:    "test",
			expected:       true,
		},
		{
			name:           "stores lowercase",
			initialTargets: nil,
			target:         "TEST",
			checkTarget:    "test",
			expected:       true,
		},
		{
			name:           "idempotent approval",
			initialTargets: map[string]bool{"test": true},
			target:         "test",
			checkTarget:    "test",
			expected:       true,
		},
		{
			name:           "preserves existing targets",
			initialTargets: map[string]bool{"existing": true},
			target:         "new",
			checkTarget:    "existing",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{ApprovedTargets: tt.initialTargets}
			c.ApproveTarget(tt.target)
			if got := c.ApprovedTargets[tt.checkTarget]; got != tt.expected {
				t.Errorf("ApprovedTargets[%q] = %v, want %v", tt.checkTarget, got, tt.expected)
			}
		})
	}
}

func TestContext_IsTargetDenied(t *testing.T) {
	tests := []struct {
		name     string
		denied   map[string]bool
		target   string
		expected bool
	}{
		{
			name:     "nil map returns false",
			denied:   nil,
			target:   "test",
			expected: false,
		},
		{
			name:     "empty map returns false",
			denied:   map[string]bool{},
			target:   "test",
			expected: false,
		},
		{
			name:     "target found returns true",
			denied:   map[string]bool{"test": true},
			target:   "test",
			expected: true,
		},
		{
			name:     "target not found returns false",
			denied:   map[string]bool{"other": true},
			target:   "test",
			expected: false,
		},
		{
			name:     "case insensitive uppercase query",
			denied:   map[string]bool{"test": true},
			target:   "TEST",
			expected: true,
		},
		{
			name:     "case insensitive mixed case query",
			denied:   map[string]bool{"test": true},
			target:   "TeSt",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{DeniedTargets: tt.denied}
			if got := c.IsTargetDenied(tt.target); got != tt.expected {
				t.Errorf("IsTargetDenied(%q) = %v, want %v", tt.target, got, tt.expected)
			}
		})
	}
}

func TestContext_DenyTarget(t *testing.T) {
	tests := []struct {
		name           string
		initialTargets map[string]bool
		target         string
		checkTarget    string
		expected       bool
	}{
		{
			name:           "lazy init nil map",
			initialTargets: nil,
			target:         "test",
			checkTarget:    "test",
			expected:       true,
		},
		{
			name:           "stores lowercase",
			initialTargets: nil,
			target:         "TEST",
			checkTarget:    "test",
			expected:       true,
		},
		{
			name:           "idempotent denial",
			initialTargets: map[string]bool{"test": true},
			target:         "test",
			checkTarget:    "test",
			expected:       true,
		},
		{
			name:           "preserves existing targets",
			initialTargets: map[string]bool{"existing": true},
			target:         "new",
			checkTarget:    "existing",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{DeniedTargets: tt.initialTargets}
			c.DenyTarget(tt.target)
			if got := c.DeniedTargets[tt.checkTarget]; got != tt.expected {
				t.Errorf("DeniedTargets[%q] = %v, want %v", tt.checkTarget, got, tt.expected)
			}
		})
	}
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
		{
			name:         "valid simple path",
			worktreePath: "/repo",
			relPath:      "src/main.go",
			wantPath:     "/repo/src/main.go",
			wantError:    false,
		},
		{
			name:         "valid path with dots",
			worktreePath: "/repo",
			relPath:      "src/../lib/util.go",
			wantPath:     "/repo/lib/util.go",
			wantError:    false,
		},
		{
			name:         "valid current dir path",
			worktreePath: "/repo",
			relPath:      "./src/main.go",
			wantPath:     "/repo/src/main.go",
			wantError:    false,
		},
		{
			name:         "parent escape blocked",
			worktreePath: "/repo",
			relPath:      "../secret",
			wantError:    true,
			errorContain: "path escapes worktree",
		},
		{
			name:         "deep parent escape blocked",
			worktreePath: "/repo",
			relPath:      "src/../../secret",
			wantError:    true,
			errorContain: "path escapes worktree",
		},
		{
			name:         "absolute path blocked",
			worktreePath: "/repo",
			relPath:      "/etc/passwd",
			wantError:    true,
			errorContain: "absolute paths not allowed",
		},
		{
			name:         "path normalization dots",
			worktreePath: "/repo",
			relPath:      "src/./test/../main.go",
			wantPath:     "/repo/src/main.go",
			wantError:    false,
		},
		{
			name:         "empty path resolves to worktree",
			worktreePath: "/repo",
			relPath:      "",
			wantPath:     "/repo",
			wantError:    false,
		},
		{
			name:         "dot path resolves to worktree",
			worktreePath: "/repo",
			relPath:      ".",
			wantPath:     "/repo",
			wantError:    false,
		},
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
				if !errResult.IsError {
					t.Errorf("ValidatePath(%q) result.IsError = false, want true", tt.relPath)
				}
				if tt.errorContain != "" {
					if got := errResult.Content; !strings.Contains(got, tt.errorContain) {
						t.Errorf("ValidatePath(%q) error = %q, want to contain %q", tt.relPath, got, tt.errorContain)
					}
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

