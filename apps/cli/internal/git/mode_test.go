package git

import (
	"os"
	"testing"
)

// TestDetectExecutionMode tests execution mode detection
func TestDetectExecutionMode(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "GitHub Actions environment",
			envValue: "true",
			want:     "github",
		},
		{
			name:     "local/act environment - empty",
			envValue: "",
			want:     "act",
		},
		{
			name:     "local/act environment - false",
			envValue: "false",
			want:     "act",
		},
		{
			name:     "local/act environment - other value",
			envValue: "something-else",
			want:     "act",
		},
		{
			name:     "case sensitive - True (not true)",
			envValue: "True",
			want:     "act",
		},
		{
			name:     "case sensitive - TRUE (not true)",
			envValue: "TRUE",
			want:     "act",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("GITHUB_ACTIONS")
			defer func() {
				if originalValue != "" {
					_ = os.Setenv("GITHUB_ACTIONS", originalValue)
				} else {
					_ = os.Unsetenv("GITHUB_ACTIONS")
				}
			}()

			// Set test value
			if tt.envValue != "" {
				if err := os.Setenv("GITHUB_ACTIONS", tt.envValue); err != nil {
					t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
				}
			} else {
				if err := os.Unsetenv("GITHUB_ACTIONS"); err != nil {
					t.Fatalf("Failed to unset GITHUB_ACTIONS: %v", err)
				}
			}

			got := DetectExecutionMode()
			if got != tt.want {
				t.Errorf("DetectExecutionMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectExecutionMode_DefaultBehavior tests default behavior when env var is not set
func TestDetectExecutionMode_DefaultBehavior(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("GITHUB_ACTIONS")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("GITHUB_ACTIONS", originalValue)
		} else {
			_ = os.Unsetenv("GITHUB_ACTIONS")
		}
	}()

	// Ensure env var is not set
	if err := os.Unsetenv("GITHUB_ACTIONS"); err != nil {
		t.Fatalf("Failed to unset GITHUB_ACTIONS: %v", err)
	}

	got := DetectExecutionMode()
	if got != "act" {
		t.Errorf("DetectExecutionMode() with no env var = %v, want 'act'", got)
	}
}

// TestDetectExecutionMode_ConsecutiveCalls tests that consecutive calls return the same value
func TestDetectExecutionMode_ConsecutiveCalls(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("GITHUB_ACTIONS")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("GITHUB_ACTIONS", originalValue)
		} else {
			_ = os.Unsetenv("GITHUB_ACTIONS")
		}
	}()

	// Test with GITHUB_ACTIONS=true
	if err := os.Setenv("GITHUB_ACTIONS", "true"); err != nil {
		t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
	}

	result1 := DetectExecutionMode()
	result2 := DetectExecutionMode()

	if result1 != result2 {
		t.Errorf("Consecutive calls returned different values: %v vs %v", result1, result2)
	}

	if result1 != "github" {
		t.Errorf("DetectExecutionMode() = %v, want 'github'", result1)
	}

	// Test with GITHUB_ACTIONS unset
	if err := os.Unsetenv("GITHUB_ACTIONS"); err != nil {
		t.Fatalf("Failed to unset GITHUB_ACTIONS: %v", err)
	}

	result3 := DetectExecutionMode()
	result4 := DetectExecutionMode()

	if result3 != result4 {
		t.Errorf("Consecutive calls returned different values: %v vs %v", result3, result4)
	}

	if result3 != "act" {
		t.Errorf("DetectExecutionMode() = %v, want 'act'", result3)
	}
}

// TestDetectExecutionMode_EnvVarChange tests that the function reflects env var changes
func TestDetectExecutionMode_EnvVarChange(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("GITHUB_ACTIONS")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("GITHUB_ACTIONS", originalValue)
		} else {
			_ = os.Unsetenv("GITHUB_ACTIONS")
		}
	}()

	// Start with unset
	if err := os.Unsetenv("GITHUB_ACTIONS"); err != nil {
		t.Fatalf("Failed to unset GITHUB_ACTIONS: %v", err)
	}

	got := DetectExecutionMode()
	if got != "act" {
		t.Errorf("DetectExecutionMode() with unset env = %v, want 'act'", got)
	}

	// Change to true
	if err := os.Setenv("GITHUB_ACTIONS", "true"); err != nil {
		t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
	}

	got = DetectExecutionMode()
	if got != "github" {
		t.Errorf("DetectExecutionMode() with env=true = %v, want 'github'", got)
	}

	// Change to false
	if err := os.Setenv("GITHUB_ACTIONS", "false"); err != nil {
		t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
	}

	got = DetectExecutionMode()
	if got != "act" {
		t.Errorf("DetectExecutionMode() with env=false = %v, want 'act'", got)
	}
}

// TestDetectExecutionMode_WhitespaceHandling tests handling of whitespace in env var
func TestDetectExecutionMode_WhitespaceHandling(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "true with leading space",
			envValue: " true",
			want:     "act",
		},
		{
			name:     "true with trailing space",
			envValue: "true ",
			want:     "act",
		},
		{
			name:     "true with both spaces",
			envValue: " true ",
			want:     "act",
		},
		{
			name:     "true with tab",
			envValue: "true\t",
			want:     "act",
		},
		{
			name:     "true with newline",
			envValue: "true\n",
			want:     "act",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("GITHUB_ACTIONS")
			defer func() {
				if originalValue != "" {
					_ = os.Setenv("GITHUB_ACTIONS", originalValue)
				} else {
					_ = os.Unsetenv("GITHUB_ACTIONS")
				}
			}()

			if err := os.Setenv("GITHUB_ACTIONS", tt.envValue); err != nil {
				t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
			}

			got := DetectExecutionMode()
			if got != tt.want {
				t.Errorf("DetectExecutionMode() with env=%q = %v, want %v", tt.envValue, got, tt.want)
			}
		})
	}
}

// TestDetectExecutionMode_NumericValues tests handling of numeric values
func TestDetectExecutionMode_NumericValues(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "numeric 1",
			envValue: "1",
			want:     "act",
		},
		{
			name:     "numeric 0",
			envValue: "0",
			want:     "act",
		},
		{
			name:     "numeric -1",
			envValue: "-1",
			want:     "act",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("GITHUB_ACTIONS")
			defer func() {
				if originalValue != "" {
					_ = os.Setenv("GITHUB_ACTIONS", originalValue)
				} else {
					_ = os.Unsetenv("GITHUB_ACTIONS")
				}
			}()

			if err := os.Setenv("GITHUB_ACTIONS", tt.envValue); err != nil {
				t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
			}

			got := DetectExecutionMode()
			if got != tt.want {
				t.Errorf("DetectExecutionMode() with env=%q = %v, want %v", tt.envValue, got, tt.want)
			}
		})
	}
}

// TestDetectExecutionMode_EmptyString tests handling of empty string explicitly set
func TestDetectExecutionMode_EmptyString(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("GITHUB_ACTIONS")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("GITHUB_ACTIONS", originalValue)
		} else {
			_ = os.Unsetenv("GITHUB_ACTIONS")
		}
	}()

	// Explicitly set to empty string
	if err := os.Setenv("GITHUB_ACTIONS", ""); err != nil {
		t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
	}

	got := DetectExecutionMode()
	if got != "act" {
		t.Errorf("DetectExecutionMode() with env='' = %v, want 'act'", got)
	}
}

// TestDetectExecutionMode_SpecialCharacters tests handling of special characters
func TestDetectExecutionMode_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "unicode characters",
			envValue: "true™",
			want:     "act",
		},
		{
			name:     "special symbols",
			envValue: "true!",
			want:     "act",
		},
		{
			name:     "path-like value",
			envValue: "/usr/bin/true",
			want:     "act",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("GITHUB_ACTIONS")
			defer func() {
				if originalValue != "" {
					_ = os.Setenv("GITHUB_ACTIONS", originalValue)
				} else {
					_ = os.Unsetenv("GITHUB_ACTIONS")
				}
			}()

			if err := os.Setenv("GITHUB_ACTIONS", tt.envValue); err != nil {
				t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
			}

			got := DetectExecutionMode()
			if got != tt.want {
				t.Errorf("DetectExecutionMode() with env=%q = %v, want %v", tt.envValue, got, tt.want)
			}
		})
	}
}

// TestDetectExecutionMode_ActualGitHubActions tests the expected behavior in actual GitHub Actions
func TestDetectExecutionMode_ActualGitHubActions(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("GITHUB_ACTIONS")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("GITHUB_ACTIONS", originalValue)
		} else {
			_ = os.Unsetenv("GITHUB_ACTIONS")
		}
	}()

	// Simulate actual GitHub Actions environment
	if err := os.Setenv("GITHUB_ACTIONS", "true"); err != nil {
		t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
	}

	got := DetectExecutionMode()
	if got != "github" {
		t.Errorf("DetectExecutionMode() in simulated GitHub Actions = %v, want 'github'", got)
	}

	// Verify that the detection is strict about the value
	if err := os.Setenv("GITHUB_ACTIONS", "TRUE"); err != nil {
		t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
	}

	got = DetectExecutionMode()
	if got != "act" {
		t.Errorf("DetectExecutionMode() should be case-sensitive, got %v", got)
	}
}

// TestDetectExecutionMode_ActEnvironment tests the expected behavior in act environment
func TestDetectExecutionMode_ActEnvironment(t *testing.T) {
	// Save original value
	originalValue := os.Getenv("GITHUB_ACTIONS")
	defer func() {
		if originalValue != "" {
			_ = os.Setenv("GITHUB_ACTIONS", originalValue)
		} else {
			_ = os.Unsetenv("GITHUB_ACTIONS")
		}
	}()

	// Simulate act environment (GITHUB_ACTIONS not set or set to non-true value)
	testCases := []string{"", "false", "0", "act", "local"}

	for _, envValue := range testCases {
		if envValue == "" {
			if err := os.Unsetenv("GITHUB_ACTIONS"); err != nil {
				t.Fatalf("Failed to unset GITHUB_ACTIONS: %v", err)
			}
		} else {
			if err := os.Setenv("GITHUB_ACTIONS", envValue); err != nil {
				t.Fatalf("Failed to set GITHUB_ACTIONS: %v", err)
			}
		}

		got := DetectExecutionMode()
		if got != "act" {
			t.Errorf("DetectExecutionMode() with env=%q = %v, want 'act'", envValue, got)
		}
	}
}
