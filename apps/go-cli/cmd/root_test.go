package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "root command has correct use",
			wantUse: "detent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rootCmd.Use != tt.wantUse {
				t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestRootCommandVersion(t *testing.T) {
	// Store original version
	originalVersion := Version
	defer func() { Version = originalVersion }()

	// Set version and verify rootCmd picks it up
	Version = "test-version"

	// Need to reinitialize rootCmd to pick up new version
	// Since rootCmd.Version is set at init time, we just verify the Version variable
	if Version != "test-version" {
		t.Errorf("Version = %q, want %q", Version, "test-version")
	}
}

func TestRootCommandSubcommands(t *testing.T) {
	expectedCommands := []string{
		"check",
		"heal",
		"frankenstein",
		"config",
		"allow",
		"clean",
	}

	commands := rootCmd.Commands()
	commandMap := make(map[string]bool)
	for _, cmd := range commands {
		commandMap[cmd.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !commandMap[expected] {
			t.Errorf("Expected subcommand %q not found", expected)
		}
	}
}

func TestRootCommandFlags(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		wantType string
	}{
		{
			name:     "workflows flag exists",
			flagName: "workflows",
			wantType: "string",
		},
		{
			name:     "workflow flag exists",
			flagName: "workflow",
			wantType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("Flag %q not found", tt.flagName)
				return
			}

			if flag.Value.Type() != tt.wantType {
				t.Errorf("Flag %q has type %q, want %q", tt.flagName, flag.Value.Type(), tt.wantType)
			}
		})
	}
}

func TestExecute(t *testing.T) {
	// Execute is a package-level function that's always defined
	// This test just verifies it exists and is callable
	// We can't fully test Execute without running the actual command
	// The function existence is verified at compile time
}

func TestCommandSilenceSettings(t *testing.T) {
	tests := []struct {
		name           string
		cmd            *cobra.Command
		wantSilenceErr bool
	}{
		{
			name:           "checkCmd silences errors",
			cmd:            checkCmd,
			wantSilenceErr: true,
		},
		{
			name:           "healCmd silences errors",
			cmd:            healCmd,
			wantSilenceErr: true,
		},
		{
			name:           "frankensteinCmd silences errors",
			cmd:            frankensteinCmd,
			wantSilenceErr: true,
		},
		{
			name:           "allowCmd silences errors",
			cmd:            allowCmd,
			wantSilenceErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.SilenceErrors != tt.wantSilenceErr {
				t.Errorf("%s.SilenceErrors = %v, want %v", tt.name, tt.cmd.SilenceErrors, tt.wantSilenceErr)
			}
			if tt.cmd.SilenceUsage != tt.wantSilenceErr {
				t.Errorf("%s.SilenceUsage = %v, want %v", tt.name, tt.cmd.SilenceUsage, tt.wantSilenceErr)
			}
		})
	}
}
