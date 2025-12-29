// Package agent provides detection and handling for AI agent environments.
// When tools like Claude Code, Cursor, or other AI agents run detent,
// we want to optimize the output for machine parsing rather than human TUI.
package agent

import (
	"os"
	"strings"
)

// knownAgentEnvVars maps environment variable names to their expected values.
// Empty string means any non-empty value indicates an agent.
var knownAgentEnvVars = map[string]string{
	"CLAUDECODE":       "1",  // Claude Code
	"CLAUDE_CODE":      "",   // Claude Code (alternative)
	"CURSOR_AGENT":     "",   // Cursor AI
	"CODEX":            "",   // OpenAI Codex
	"AIDER":            "",   // Aider
	"CONTINUE_SESSION": "",   // Continue.dev
	"CODY_AGENT":       "",   // Sourcegraph Cody
	"AI_AGENT":         "",   // Generic convention
	"AGENT_MODE":       "",   // Generic convention
}

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
// run by an AI agent. This enables automatic optimization of output
// for machine parsing (verbose text, JSON, no TUI).
func Detect() Info {
	for envVar, expectedValue := range knownAgentEnvVars {
		value := os.Getenv(envVar)
		if value == "" {
			continue
		}

		// If expected value is set, check for exact match
		if expectedValue != "" && value != expectedValue {
			continue
		}

		return Info{
			IsAgent: true,
			Name:    getAgentName(envVar),
			EnvVar:  envVar,
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

// IsRunningInAgent is a convenience function that returns true if
// an AI agent environment is detected.
func IsRunningInAgent() bool {
	return Detect().IsAgent
}
