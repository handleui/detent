// Package agent provides detection and handling for AI agent environments.
// When tools like Claude Code, Cursor, or other AI agents run detent,
// we want to optimize the output for machine parsing rather than human TUI.
package agent

import (
	"os"
	"strings"
	"sync"
)

// knownAgentEnvVars maps environment variable names to their expected values.
// Empty string means any non-empty value indicates an agent.
// Detection order: first match wins, so more specific vars should come first.
var knownAgentEnvVars = []struct {
	envVar        string
	expectedValue string // empty = any non-empty value matches
}{
	{"CLAUDECODE", "1"},        // Claude Code (exact match)
	{"CLAUDE_CODE", ""},        // Claude Code (alternative)
	{"CURSOR_AGENT", ""},       // Cursor AI
	{"CODEX", ""},              // OpenAI Codex
	{"AIDER", ""},              // Aider
	{"CONTINUE_SESSION", ""},   // Continue.dev
	{"CODY_AGENT", ""},         // Sourcegraph Cody
	{"AI_AGENT", ""},           // Generic convention
	{"AGENT_MODE", ""},         // Generic convention
}

// cached stores the detection result (immutable after first detection)
var (
	cached     Info
	cachedOnce sync.Once
)

// Info contains information about the detected AI agent environment.
type Info struct {
	// IsAgent is true if an AI agent environment was detected
	IsAgent bool

	// Name is the detected agent name (e.g., "Claude Code", "Cursor")
	Name string

	// EnvVar is the environment variable that triggered detection
	EnvVar string
}

// Detect checks environment variables to determine if detent is being
// run by an AI agent. Results are cached since env vars are immutable
// during process lifetime. Safe for concurrent use.
func Detect() Info {
	cachedOnce.Do(func() {
		cached = detect()
	})
	return cached
}

// detect performs the actual detection (called once via sync.Once).
func detect() Info {
	for _, entry := range knownAgentEnvVars {
		value := os.Getenv(entry.envVar)
		if value == "" {
			continue
		}

		// If expected value is set, check for exact match
		if entry.expectedValue != "" && value != entry.expectedValue {
			continue
		}

		return Info{
			IsAgent: true,
			Name:    getAgentName(entry.envVar),
			EnvVar:  entry.envVar,
		}
	}

	return Info{IsAgent: false}
}

// getAgentName returns a human-readable name for the agent based on env var.
func getAgentName(envVar string) string {
	switch strings.ToUpper(envVar) {
	case "CLAUDECODE", "CLAUDE_CODE":
		return "Claude Code"
	case "CURSOR_AGENT":
		return "Cursor"
	case "CODEX":
		return "Codex"
	case "AIDER":
		return "Aider"
	case "CONTINUE_SESSION":
		return "Continue"
	case "CODY_AGENT":
		return "Cody"
	default:
		return "AI Agent"
	}
}
