package cmd

import (
	"testing"
)

func TestFrankensteinCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "frankenstein command has correct use",
			wantUse: "frankenstein",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if frankensteinCmd.Use != tt.wantUse {
				t.Errorf("frankensteinCmd.Use = %q, want %q", frankensteinCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestFrankensteinCommandFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		shorthand string
		wantType  string
	}{
		{
			name:     "monster flag exists",
			flagName: "monster",
			wantType: "bool",
		},
		{
			name:      "verbose flag exists",
			flagName:  "verbose",
			shorthand: "v",
			wantType:  "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := frankensteinCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("Flag %q not found", tt.flagName)
				return
			}

			if flag.Value.Type() != tt.wantType {
				t.Errorf("Flag %q has type %q, want %q", tt.flagName, flag.Value.Type(), tt.wantType)
			}

			if tt.shorthand != "" && flag.Shorthand != tt.shorthand {
				t.Errorf("Flag %q has shorthand %q, want %q", tt.flagName, flag.Shorthand, tt.shorthand)
			}
		})
	}
}

func TestFrankensteinCommandNoArgs(t *testing.T) {
	if frankensteinCmd.Args == nil {
		t.Error("frankensteinCmd.Args should be set to NoArgs")
	}
}

func TestNewToolTracker(t *testing.T) {
	tests := []struct {
		name      string
		toolNames []string
		wantCount int
	}{
		{
			name:      "empty tools",
			toolNames: []string{},
			wantCount: 0,
		},
		{
			name:      "single tool",
			toolNames: []string{"glob"},
			wantCount: 1,
		},
		{
			name:      "multiple tools",
			toolNames: []string{"glob", "read_file", "grep"},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := newToolTracker(tt.toolNames)
			if tracker == nil {
				t.Error("newToolTracker() returned nil")
				return
			}

			if len(tracker.expected) != tt.wantCount {
				t.Errorf("newToolTracker() expected count = %d, want %d", len(tracker.expected), tt.wantCount)
			}

			for _, name := range tt.toolNames {
				if !tracker.expected[name] {
					t.Errorf("Tool %q not in expected map", name)
				}
			}
		})
	}
}

func TestToolTrackerRecordCall(t *testing.T) {
	tracker := newToolTracker([]string{"glob", "read_file"})

	tests := []struct {
		name     string
		toolName string
		isError  bool
		errorMsg string
	}{
		{
			name:     "successful call",
			toolName: "glob",
			isError:  false,
			errorMsg: "",
		},
		{
			name:     "error call",
			toolName: "read_file",
			isError:  true,
			errorMsg: "file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker.recordCall(tt.toolName, tt.isError, tt.errorMsg)

			if !tracker.called[tt.toolName] {
				t.Errorf("Tool %q should be marked as called", tt.toolName)
			}

			if tt.isError {
				if errMsg, exists := tracker.errors[tt.toolName]; !exists {
					t.Errorf("Tool %q error should be recorded", tt.toolName)
				} else if errMsg != tt.errorMsg {
					t.Errorf("Tool %q error message = %q, want %q", tt.toolName, errMsg, tt.errorMsg)
				}
			}
		})
	}
}

func TestToolTrackerAllCalled(t *testing.T) {
	tests := []struct {
		name     string
		tools    []string
		calls    []string
		wantAll  bool
	}{
		{
			name:    "none called",
			tools:   []string{"glob", "read_file"},
			calls:   []string{},
			wantAll: false,
		},
		{
			name:    "some called",
			tools:   []string{"glob", "read_file"},
			calls:   []string{"glob"},
			wantAll: false,
		},
		{
			name:    "all called",
			tools:   []string{"glob", "read_file"},
			calls:   []string{"glob", "read_file"},
			wantAll: true,
		},
		{
			name:    "empty list all called",
			tools:   []string{},
			calls:   []string{},
			wantAll: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := newToolTracker(tt.tools)
			for _, call := range tt.calls {
				tracker.recordCall(call, false, "")
			}

			if got := tracker.allCalled(); got != tt.wantAll {
				t.Errorf("toolTracker.allCalled() = %v, want %v", got, tt.wantAll)
			}
		})
	}
}

func TestToolTrackerHasErrors(t *testing.T) {
	tests := []struct {
		name       string
		tools      []string
		errorCalls map[string]string
		wantErrors bool
	}{
		{
			name:       "no errors",
			tools:      []string{"glob", "read_file"},
			errorCalls: map[string]string{},
			wantErrors: false,
		},
		{
			name:  "has errors",
			tools: []string{"glob", "read_file"},
			errorCalls: map[string]string{
				"glob": "pattern invalid",
			},
			wantErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := newToolTracker(tt.tools)
			for tool, errMsg := range tt.errorCalls {
				tracker.recordCall(tool, true, errMsg)
			}

			if got := tracker.hasErrors(); got != tt.wantErrors {
				t.Errorf("toolTracker.hasErrors() = %v, want %v", got, tt.wantErrors)
			}
		})
	}
}

func TestToolTrackerGetCallStatus(t *testing.T) {
	tracker := newToolTracker([]string{"glob", "read_file", "grep"})
	tracker.recordCall("glob", false, "")
	tracker.recordCall("read_file", true, "file not found")

	tests := []struct {
		name         string
		toolName     string
		wantCalled   bool
		wantHasError bool
		wantErrMsg   string
	}{
		{
			name:         "successful call",
			toolName:     "glob",
			wantCalled:   true,
			wantHasError: false,
			wantErrMsg:   "",
		},
		{
			name:         "error call",
			toolName:     "read_file",
			wantCalled:   true,
			wantHasError: true,
			wantErrMsg:   "file not found",
		},
		{
			name:         "not called",
			toolName:     "grep",
			wantCalled:   false,
			wantHasError: false,
			wantErrMsg:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called, errMsg, hasError := tracker.getCallStatus(tt.toolName)

			if called != tt.wantCalled {
				t.Errorf("getCallStatus(%q) called = %v, want %v", tt.toolName, called, tt.wantCalled)
			}
			if hasError != tt.wantHasError {
				t.Errorf("getCallStatus(%q) hasError = %v, want %v", tt.toolName, hasError, tt.wantHasError)
			}
			if errMsg != tt.wantErrMsg {
				t.Errorf("getCallStatus(%q) errMsg = %q, want %q", tt.toolName, errMsg, tt.wantErrMsg)
			}
		})
	}
}

func TestBuildFrankensteinSystemPrompt(t *testing.T) {
	tests := []struct {
		name          string
		monster       bool
		wantContains  []string
	}{
		{
			name:    "normal mode",
			monster: false,
			wantContains: []string{
				"glob",
				"read_file",
				"grep",
			},
		},
		{
			name:    "monster mode",
			monster: true,
			wantContains: []string{
				"glob",
				"read_file",
				"grep",
				"edit_file",
				"run_command",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildFrankensteinSystemPrompt(tt.monster)

			for _, expected := range tt.wantContains {
				if !contains(prompt, expected) {
					t.Errorf("buildFrankensteinSystemPrompt(%v) should contain %q", tt.monster, expected)
				}
			}
		})
	}
}

func TestBuildFrankensteinUserPrompt(t *testing.T) {
	tests := []struct {
		name         string
		monster      bool
		wantContains string
	}{
		{
			name:         "normal mode",
			monster:      false,
			wantContains: "3 tools",
		},
		{
			name:         "monster mode",
			monster:      true,
			wantContains: "5 tools",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildFrankensteinUserPrompt(tt.monster)

			if !contains(prompt, tt.wantContains) {
				t.Errorf("buildFrankensteinUserPrompt(%v) should contain %q", tt.monster, tt.wantContains)
			}
		})
	}
}

func TestTruncateVerbose(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello world",
			maxLen: 11,
			want:   "hello world",
		},
		{
			name:   "long string truncated",
			input:  "this is a very long string",
			maxLen: 10,
			want:   "this is...",
		},
		{
			name:   "newlines replaced",
			input:  "line1\nline2\nline3",
			maxLen: 50,
			want:   "line1 line2 line3",
		},
		{
			name:   "newlines and truncation",
			input:  "line1\nline2\nline3",
			maxLen: 10,
			want:   "line1 l...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateVerbose(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateVerbose(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestTruncateError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short message",
			input: "error",
			want:  "error",
		},
		{
			name:  "exact 30 chars",
			input: "this message is exactly 30chr",
			want:  "this message is exactly 30chr",
		},
		{
			name:  "long message truncated",
			input: "this is a very long error message that should be truncated",
			want:  "this is a very long error m...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateError(tt.input)
			if got != tt.want {
				t.Errorf("truncateError(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewFrankensteinModel(t *testing.T) {
	tracker := newToolTracker([]string{"glob", "read_file"})
	expectedTools := []string{"glob", "read_file"}

	tests := []struct {
		name    string
		monster bool
	}{
		{
			name:    "normal mode",
			monster: false,
		},
		{
			name:    "monster mode",
			monster: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := newFrankensteinModel(tracker, expectedTools, tt.monster)

			if model.tracker != tracker {
				t.Error("model.tracker not set correctly")
			}
			if len(model.expectedTools) != len(expectedTools) {
				t.Errorf("model.expectedTools length = %d, want %d", len(model.expectedTools), len(expectedTools))
			}
			if model.monster != tt.monster {
				t.Errorf("model.monster = %v, want %v", model.monster, tt.monster)
			}
			if model.done {
				t.Error("model.done should be false initially")
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
