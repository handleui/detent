package cmd

import (
	"strings"
	"testing"

	"github.com/detent/go-cli/internal/persistence"
)

func TestHealCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "heal command has correct use",
			wantUse: "heal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if healCmd.Use != tt.wantUse {
				t.Errorf("healCmd.Use = %q, want %q", healCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestHealCommandFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		shorthand string
		wantType  string
	}{
		{
			name:      "force flag exists",
			flagName:  "force",
			shorthand: "f",
			wantType:  "bool",
		},
		{
			name:     "test flag exists",
			flagName: "test",
			wantType: "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := healCmd.Flags().Lookup(tt.flagName)
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

func TestHealCommandNoArgs(t *testing.T) {
	if healCmd.Args == nil {
		t.Error("healCmd.Args should be set to NoArgs")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	tests := []struct {
		name         string
		errRecords   []*persistence.ErrorRecord
		wantContains []string
		wantCount    int
	}{
		{
			name:       "empty errors",
			errRecords: []*persistence.ErrorRecord{},
			wantContains: []string{
				"Fix the following CI errors:",
			},
			wantCount: 0,
		},
		{
			name: "single error",
			errRecords: []*persistence.ErrorRecord{
				{
					FilePath:     "main.go",
					LineNumber:   10,
					ColumnNumber: 5,
					Message:      "undefined: foo",
					ErrorType:    "compile",
					Severity:     "error",
				},
			},
			wantContains: []string{
				"main.go",
				"undefined: foo",
				"[compile]",
				"main.go:10:5:",
			},
			wantCount: 1,
		},
		{
			name: "multiple errors same file",
			errRecords: []*persistence.ErrorRecord{
				{
					FilePath:     "app.go",
					LineNumber:   5,
					ColumnNumber: 3,
					Message:      "syntax error",
					ErrorType:    "compile",
					Severity:     "error",
				},
				{
					FilePath:     "app.go",
					LineNumber:   15,
					ColumnNumber: 7,
					Message:      "type mismatch",
					ErrorType:    "type",
					Severity:     "error",
				},
			},
			wantContains: []string{
				"app.go",
				"(2 errors)",
				"syntax error",
				"type mismatch",
			},
			wantCount: 2,
		},
		{
			name: "error with rule and source",
			errRecords: []*persistence.ErrorRecord{
				{
					FilePath:     "test.ts",
					LineNumber:   20,
					ColumnNumber: 1,
					Message:      "missing semicolon",
					ErrorType:    "lint",
					RuleID:       "semi",
					Source:       "eslint",
					Severity:     "error",
				},
			},
			wantContains: []string{
				"test.ts",
				"missing semicolon",
				"Rule: semi",
				"Source: eslint",
			},
			wantCount: 1,
		},
		{
			name: "error with stack trace",
			errRecords: []*persistence.ErrorRecord{
				{
					FilePath:     "index.js",
					LineNumber:   100,
					ColumnNumber: 10,
					Message:      "runtime error",
					ErrorType:    "runtime",
					StackTrace:   "Error: runtime error\n  at func1 (file1.js:10)\n  at func2 (file2.js:20)",
					Severity:     "error",
				},
			},
			wantContains: []string{
				"index.js",
				"runtime error",
				"Stack trace:",
				"at func1",
			},
			wantCount: 1,
		},
		{
			name: "multiple files",
			errRecords: []*persistence.ErrorRecord{
				{
					FilePath:     "file1.go",
					LineNumber:   10,
					ColumnNumber: 5,
					Message:      "error 1",
					ErrorType:    "type1",
					Severity:     "error",
				},
				{
					FilePath:     "file2.go",
					LineNumber:   20,
					ColumnNumber: 3,
					Message:      "error 2",
					ErrorType:    "type2",
					Severity:     "error",
				},
			},
			wantContains: []string{
				"file1.go",
				"file2.go",
				"error 1",
				"error 2",
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildUserPrompt(tt.errRecords)

			for _, expected := range tt.wantContains {
				if !strings.Contains(result, expected) {
					t.Errorf("buildUserPrompt() should contain %q\nGot:\n%s", expected, result)
				}
			}

			// Count occurrences of error type markers to verify all errors are included
			if tt.wantCount > 0 {
				count := strings.Count(result, "[")
				if count < tt.wantCount {
					t.Errorf("buildUserPrompt() found %d error markers, want at least %d", count, tt.wantCount)
				}
			}
		})
	}
}

func TestBuildUserPromptGrouping(t *testing.T) {
	// Test that errors are properly grouped by file
	errRecords := []*persistence.ErrorRecord{
		{
			FilePath:     "app.go",
			LineNumber:   5,
			ColumnNumber: 3,
			Message:      "error 1",
			ErrorType:    "type1",
			Severity:     "error",
		},
		{
			FilePath:     "main.go",
			LineNumber:   10,
			ColumnNumber: 5,
			Message:      "error 2",
			ErrorType:    "type2",
			Severity:     "error",
		},
		{
			FilePath:     "app.go",
			LineNumber:   15,
			ColumnNumber: 7,
			Message:      "error 3",
			ErrorType:    "type3",
			Severity:     "error",
		},
	}

	result := buildUserPrompt(errRecords)

	// Should have 2 file headers (app.go with 2 errors, main.go with 1 error)
	if !strings.Contains(result, "app.go (2 errors)") {
		t.Error("buildUserPrompt() should show app.go with 2 errors")
	}
	if !strings.Contains(result, "main.go (1 errors)") {
		t.Error("buildUserPrompt() should show main.go with 1 error")
	}

	// All errors should be present
	for i := 1; i <= 3; i++ {
		expected := "error " + string(rune('0'+i))
		if !strings.Contains(result, expected) {
			t.Errorf("buildUserPrompt() should contain %q", expected)
		}
	}
}

func TestBuildUserPromptStackTraceLimit(t *testing.T) {
	// Create a long stack trace
	var stackLines []string
	for i := 0; i < 20; i++ {
		stackLines = append(stackLines, "  at function"+string(rune('A'+i))+" (file.js:"+string(rune('0'+i))+")")
	}
	longStackTrace := strings.Join(stackLines, "\n")

	errRecords := []*persistence.ErrorRecord{
		{
			FilePath:     "test.js",
			LineNumber:   1,
			ColumnNumber: 1,
			Message:      "error with long trace",
			ErrorType:    "runtime",
			StackTrace:   longStackTrace,
			Severity:     "error",
		},
	}

	result := buildUserPrompt(errRecords)

	// Count how many stack trace lines are included
	stackLineCount := 0
	for i := 0; i < 20; i++ {
		if strings.Contains(result, "at function"+string(rune('A'+i))) {
			stackLineCount++
		}
	}

	// Should only include first 10 lines
	if stackLineCount > 10 {
		t.Errorf("buildUserPrompt() included %d stack trace lines, should be limited to 10", stackLineCount)
	}
}

func TestHealCommandSilenceSettings(t *testing.T) {
	if !healCmd.SilenceUsage {
		t.Error("healCmd.SilenceUsage should be true")
	}
	if !healCmd.SilenceErrors {
		t.Error("healCmd.SilenceErrors should be true")
	}
}
