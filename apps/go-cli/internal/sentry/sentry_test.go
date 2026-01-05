package sentry

import "testing"

func TestScrubPII(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "macOS home path",
			input:    "/Users/john/code/project",
			expected: "/Users/[user]/code/project",
		},
		{
			name:     "Linux home path",
			input:    "/home/jane/workspace/app",
			expected: "/home/[user]/workspace/app",
		},
		{
			name:     "Windows home path",
			input:    "C:\\Users\\admin\\Documents\\project",
			expected: "C:\\Users\\[user]\\Documents\\project",
		},
		{
			name:     "Anthropic API key",
			input:    "Error: invalid API key sk-ant-api03-abc123xyz789",
			expected: "Error: invalid API key sk-ant-api03-[REDACTED]",
		},
		{
			name:     "Generic API key in config",
			input:    "api_key: sk-abc123xyz789def456",
			expected: "api_key: [REDACTED]",
		},
		{
			name:     "Email address",
			input:    "Contact: john.doe@example.com for help",
			expected: "Contact: [email] for help",
		},
		{
			name:     "Multiple PIIs in one string",
			input:    "/Users/alice/code with email alice@company.com and key sk-test1234567890",
			expected: "/Users/[user]/code with email [email] and key sk-[REDACTED]",
		},
		{
			name:     "No PII present",
			input:    "failed to read file: permission denied",
			expected: "failed to read file: permission denied",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Path without home dir",
			input:    "/var/log/app.log",
			expected: "/var/log/app.log",
		},
		{
			name:     "Case insensitive home path",
			input:    "/HOME/testuser/data",
			expected: "/HOME/[user]/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := scrubPII(tt.input)
			if result != tt.expected {
				t.Errorf("scrubPII(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestScrubPII_MultipleEmails(t *testing.T) {
	input := "From: alice@example.com, To: bob@company.org"
	result := scrubPII(input)
	expected := "From: [email], To: [email]"
	if result != expected {
		t.Errorf("scrubPII(%q) = %q, want %q", input, result, expected)
	}
}

func TestScrubPII_NestedPaths(t *testing.T) {
	// Test that multiple path segments with usernames are scrubbed
	input := "comparing /Users/alice/old with /Users/bob/new"
	result := scrubPII(input)
	expected := "comparing /Users/[user]/old with /Users/[user]/new"
	if result != expected {
		t.Errorf("scrubPII(%q) = %q, want %q", input, result, expected)
	}
}
