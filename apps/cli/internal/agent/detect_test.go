package agent

import (
	"os"
	"sync"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		wantIsAgent  bool
		wantName     string
		wantEnvVar   string
	}{
		{
			name:         "no agent env vars",
			envVars:      map[string]string{},
			wantIsAgent:  false,
			wantName:     "",
			wantEnvVar:   "",
		},
		{
			name:         "CLAUDECODE env var",
			envVars:      map[string]string{"CLAUDECODE": "1"},
			wantIsAgent:  true,
			wantName:     "Claude Code",
			wantEnvVar:   "CLAUDECODE",
		},
		{
			name:         "CLAUDE_CODE env var",
			envVars:      map[string]string{"CLAUDE_CODE": "true"},
			wantIsAgent:  true,
			wantName:     "Claude Code",
			wantEnvVar:   "CLAUDE_CODE",
		},
		{
			name:         "CURSOR_AGENT env var",
			envVars:      map[string]string{"CURSOR_AGENT": "enabled"},
			wantIsAgent:  true,
			wantName:     "Cursor",
			wantEnvVar:   "CURSOR_AGENT",
		},
		{
			name:         "CODEX env var",
			envVars:      map[string]string{"CODEX": "1"},
			wantIsAgent:  true,
			wantName:     "Codex",
			wantEnvVar:   "CODEX",
		},
		{
			name:         "AIDER env var",
			envVars:      map[string]string{"AIDER": "yes"},
			wantIsAgent:  true,
			wantName:     "Aider",
			wantEnvVar:   "AIDER",
		},
		{
			name:         "CONTINUE_SESSION env var",
			envVars:      map[string]string{"CONTINUE_SESSION": "abc123"},
			wantIsAgent:  true,
			wantName:     "Continue",
			wantEnvVar:   "CONTINUE_SESSION",
		},
		{
			name:         "CODY_AGENT env var",
			envVars:      map[string]string{"CODY_AGENT": "active"},
			wantIsAgent:  true,
			wantName:     "Cody",
			wantEnvVar:   "CODY_AGENT",
		},
		{
			name:         "AI_AGENT generic env var",
			envVars:      map[string]string{"AI_AGENT": "1"},
			wantIsAgent:  true,
			wantName:     "AI Agent",
			wantEnvVar:   "AI_AGENT",
		},
		{
			name:         "AGENT_MODE generic env var",
			envVars:      map[string]string{"AGENT_MODE": "enabled"},
			wantIsAgent:  true,
			wantName:     "AI Agent",
			wantEnvVar:   "AGENT_MODE",
		},
		{
			name: "first match wins - CLAUDECODE takes priority",
			envVars: map[string]string{
				"CLAUDECODE":   "1",
				"CURSOR_AGENT": "1",
				"AI_AGENT":     "1",
			},
			wantIsAgent: true,
			wantName:    "Claude Code",
			wantEnvVar:  "CLAUDECODE",
		},
		{
			name: "first match wins - CLAUDE_CODE before CURSOR",
			envVars: map[string]string{
				"CLAUDE_CODE":  "1",
				"CURSOR_AGENT": "1",
			},
			wantIsAgent: true,
			wantName:    "Claude Code",
			wantEnvVar:  "CLAUDE_CODE",
		},
		{
			name: "unrelated env vars don't trigger detection",
			envVars: map[string]string{
				"PATH":     "/usr/bin",
				"HOME":     "/home/user",
				"TERMINAL": "xterm",
			},
			wantIsAgent: false,
			wantName:    "",
			wantEnvVar:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache before each test
			resetCache()

			// Set up environment
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			// Clear all known agent env vars that aren't in this test's envVars
			for _, entry := range knownAgentEnvVars {
				if _, exists := tt.envVars[entry.envVar]; !exists {
					os.Unsetenv(entry.envVar)
				}
			}

			got := Detect()

			if got.IsAgent != tt.wantIsAgent {
				t.Errorf("Detect().IsAgent = %v, want %v", got.IsAgent, tt.wantIsAgent)
			}
			if got.Name != tt.wantName {
				t.Errorf("Detect().Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.EnvVar != tt.wantEnvVar {
				t.Errorf("Detect().EnvVar = %q, want %q", got.EnvVar, tt.wantEnvVar)
			}
		})
	}
}

func TestDetectEmptyEnvVarValue(t *testing.T) {
	// Test that empty env var values are not detected
	resetCache()

	// Empty string should not trigger detection
	t.Setenv("CLAUDECODE", "")

	// Clear other potential agent env vars
	for _, entry := range knownAgentEnvVars {
		if entry.envVar != "CLAUDECODE" {
			os.Unsetenv(entry.envVar)
		}
	}

	got := Detect()

	if got.IsAgent {
		t.Errorf("Detect() with empty CLAUDECODE should not detect agent, got IsAgent=%v", got.IsAgent)
	}
}

func TestDetectCaching(t *testing.T) {
	resetCache()

	// Set up initial environment
	t.Setenv("CLAUDECODE", "1")
	for _, entry := range knownAgentEnvVars {
		if entry.envVar != "CLAUDECODE" {
			os.Unsetenv(entry.envVar)
		}
	}

	// First call
	first := Detect()
	if !first.IsAgent || first.Name != "Claude Code" {
		t.Fatalf("First Detect() = %+v, want IsAgent=true Name='Claude Code'", first)
	}

	// Change environment (should not affect cached result)
	os.Setenv("CURSOR_AGENT", "1")
	os.Unsetenv("CLAUDECODE")

	// Second call should return cached result
	second := Detect()
	if second != first {
		t.Errorf("Detect() should return cached result.\nFirst:  %+v\nSecond: %+v", first, second)
	}
}

func TestDetectConcurrency(t *testing.T) {
	// Test that Detect is safe for concurrent use
	resetCache()

	t.Setenv("CLAUDECODE", "1")
	for _, entry := range knownAgentEnvVars {
		if entry.envVar != "CLAUDECODE" {
			os.Unsetenv(entry.envVar)
		}
	}

	const goroutines = 100
	var wg sync.WaitGroup
	results := make([]Info, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = Detect()
		}(i)
	}
	wg.Wait()

	// All results should be identical
	expected := results[0]
	for i, result := range results {
		if result != expected {
			t.Errorf("Result[%d] = %+v, want %+v (concurrent calls should return identical results)", i, result, expected)
		}
	}
}

func TestGetAgentName(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		wantName string
	}{
		{
			name:     "CLAUDECODE lowercase",
			envVar:   "claudecode",
			wantName: "Claude Code",
		},
		{
			name:     "CLAUDECODE uppercase",
			envVar:   "CLAUDECODE",
			wantName: "Claude Code",
		},
		{
			name:     "CLAUDE_CODE mixed case",
			envVar:   "Claude_Code",
			wantName: "Claude Code",
		},
		{
			name:     "CURSOR_AGENT",
			envVar:   "CURSOR_AGENT",
			wantName: "Cursor",
		},
		{
			name:     "CODEX",
			envVar:   "CODEX",
			wantName: "Codex",
		},
		{
			name:     "AIDER",
			envVar:   "AIDER",
			wantName: "Aider",
		},
		{
			name:     "CONTINUE_SESSION",
			envVar:   "CONTINUE_SESSION",
			wantName: "Continue",
		},
		{
			name:     "CODY_AGENT",
			envVar:   "CODY_AGENT",
			wantName: "Cody",
		},
		{
			name:     "AI_AGENT",
			envVar:   "AI_AGENT",
			wantName: "AI Agent",
		},
		{
			name:     "AGENT_MODE",
			envVar:   "AGENT_MODE",
			wantName: "AI Agent",
		},
		{
			name:     "unknown env var",
			envVar:   "UNKNOWN_AGENT",
			wantName: "AI Agent",
		},
		{
			name:     "empty string",
			envVar:   "",
			wantName: "AI Agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAgentName(tt.envVar)
			if got != tt.wantName {
				t.Errorf("getAgentName(%q) = %q, want %q", tt.envVar, got, tt.wantName)
			}
		})
	}
}

func TestInfoStruct(t *testing.T) {
	// Test that Info struct fields are correctly set
	info := Info{
		IsAgent: true,
		Name:    "Test Agent",
		EnvVar:  "TEST_AGENT",
	}

	if !info.IsAgent {
		t.Error("Info.IsAgent should be true")
	}
	if info.Name != "Test Agent" {
		t.Errorf("Info.Name = %q, want 'Test Agent'", info.Name)
	}
	if info.EnvVar != "TEST_AGENT" {
		t.Errorf("Info.EnvVar = %q, want 'TEST_AGENT'", info.EnvVar)
	}
}

func TestInfoZeroValue(t *testing.T) {
	// Test that zero value of Info represents no agent
	var info Info

	if info.IsAgent {
		t.Error("Zero value Info.IsAgent should be false")
	}
	if info.Name != "" {
		t.Errorf("Zero value Info.Name = %q, want empty string", info.Name)
	}
	if info.EnvVar != "" {
		t.Errorf("Zero value Info.EnvVar = %q, want empty string", info.EnvVar)
	}
}

func TestKnownAgentEnvVarsOrder(t *testing.T) {
	// Verify that knownAgentEnvVars contains expected entries
	// and that more specific vars come before generic ones

	if len(knownAgentEnvVars) == 0 {
		t.Fatal("knownAgentEnvVars should not be empty")
	}

	// Track positions of specific vs generic vars
	claudecodePos := -1
	aiAgentPos := -1
	agentModePos := -1

	for i, entry := range knownAgentEnvVars {
		switch entry.envVar {
		case "CLAUDECODE":
			claudecodePos = i
		case "AI_AGENT":
			aiAgentPos = i
		case "AGENT_MODE":
			agentModePos = i
		}

		// All entries should have empty expectedValue (any non-empty matches)
		if entry.expectedValue != "" {
			t.Errorf("Entry %d (%s) has expectedValue=%q, want empty string", i, entry.envVar, entry.expectedValue)
		}
	}

	// Specific vars should come before generic ones
	if claudecodePos >= aiAgentPos && aiAgentPos != -1 {
		t.Error("CLAUDECODE should come before AI_AGENT in knownAgentEnvVars")
	}
	if claudecodePos >= agentModePos && agentModePos != -1 {
		t.Error("CLAUDECODE should come before AGENT_MODE in knownAgentEnvVars")
	}
}

func TestDetectWithExpectedValue(t *testing.T) {
	// Test the expectedValue logic in detect()
	// Currently all entries have empty expectedValue, but test the mechanism

	resetCache()

	// Temporarily modify knownAgentEnvVars for this test
	originalVars := knownAgentEnvVars
	defer func() { knownAgentEnvVars = originalVars }()

	// Create a test case where we expect a specific value
	knownAgentEnvVars = []struct {
		envVar        string
		expectedValue string
	}{
		{"TEST_AGENT_SPECIFIC", "expected_value"},
		{"TEST_AGENT_ANY", ""},
	}

	tests := []struct {
		name        string
		envVars     map[string]string
		wantIsAgent bool
		wantEnvVar  string
	}{
		{
			name:        "matching expected value",
			envVars:     map[string]string{"TEST_AGENT_SPECIFIC": "expected_value"},
			wantIsAgent: true,
			wantEnvVar:  "TEST_AGENT_SPECIFIC",
		},
		{
			name:        "non-matching expected value",
			envVars:     map[string]string{"TEST_AGENT_SPECIFIC": "wrong_value"},
			wantIsAgent: false,
			wantEnvVar:  "",
		},
		{
			name:        "any non-empty value accepted",
			envVars:     map[string]string{"TEST_AGENT_ANY": "anything"},
			wantIsAgent: true,
			wantEnvVar:  "TEST_AGENT_ANY",
		},
		{
			name: "specific value takes priority",
			envVars: map[string]string{
				"TEST_AGENT_SPECIFIC": "expected_value",
				"TEST_AGENT_ANY":      "anything",
			},
			wantIsAgent: true,
			wantEnvVar:  "TEST_AGENT_SPECIFIC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCache()

			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			got := Detect()

			if got.IsAgent != tt.wantIsAgent {
				t.Errorf("Detect().IsAgent = %v, want %v", got.IsAgent, tt.wantIsAgent)
			}
			if got.EnvVar != tt.wantEnvVar {
				t.Errorf("Detect().EnvVar = %q, want %q", got.EnvVar, tt.wantEnvVar)
			}
		})
	}
}

func BenchmarkDetect(b *testing.B) {
	resetCache()

	os.Setenv("CLAUDECODE", "1")
	defer os.Unsetenv("CLAUDECODE")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Detect()
	}
}

func BenchmarkDetectConcurrent(b *testing.B) {
	resetCache()

	os.Setenv("CLAUDECODE", "1")
	defer os.Unsetenv("CLAUDECODE")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Detect()
		}
	})
}

func BenchmarkGetAgentName(b *testing.B) {
	envVars := []string{
		"CLAUDECODE",
		"CLAUDE_CODE",
		"CURSOR_AGENT",
		"CODEX",
		"AIDER",
		"CONTINUE_SESSION",
		"CODY_AGENT",
		"AI_AGENT",
		"AGENT_MODE",
		"UNKNOWN_AGENT",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getAgentName(envVars[i%len(envVars)])
	}
}

// resetCache is a test helper to reset the detection cache
// This allows tests to run independently
func resetCache() {
	cached = Info{}
	cachedOnce = sync.Once{}
}
