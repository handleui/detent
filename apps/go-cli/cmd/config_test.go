package cmd

import (
	"testing"
)

func TestConfigCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "config command has correct use",
			wantUse: "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if configCmd.Use != tt.wantUse {
				t.Errorf("configCmd.Use = %q, want %q", configCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestConfigCommandSubcommands(t *testing.T) {
	expectedCommands := []string{
		"show",
		"reset",
		"path",
		"schema",
	}

	commands := configCmd.Commands()
	commandMap := make(map[string]bool)
	for _, cmd := range commands {
		commandMap[cmd.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !commandMap[expected] {
			t.Errorf("Expected subcommand %q not found in config command", expected)
		}
	}

	if len(commands) != len(expectedCommands) {
		t.Errorf("configCmd has %d subcommands, want %d", len(commands), len(expectedCommands))
	}
}

func TestConfigShowCommand(t *testing.T) {
	if configShowCmd.Use != "show" {
		t.Errorf("configShowCmd.Use = %q, want %q", configShowCmd.Use, "show")
	}
	if configShowCmd.RunE == nil {
		t.Error("configShowCmd.RunE should be set")
	}
}

func TestConfigResetCommand(t *testing.T) {
	if configResetCmd.Use != "reset" {
		t.Errorf("configResetCmd.Use = %q, want %q", configResetCmd.Use, "reset")
	}
	if configResetCmd.RunE == nil {
		t.Error("configResetCmd.RunE should be set")
	}

	// Check for force flag
	flag := configResetCmd.Flags().Lookup("force")
	if flag == nil {
		t.Error("configResetCmd should have 'force' flag")
		return
	}
	if flag.Shorthand != "f" {
		t.Errorf("force flag shorthand = %q, want %q", flag.Shorthand, "f")
	}
	if flag.Value.Type() != "bool" {
		t.Errorf("force flag type = %q, want %q", flag.Value.Type(), "bool")
	}
}

func TestConfigPathCommand(t *testing.T) {
	if configPathCmd.Use != "path" {
		t.Errorf("configPathCmd.Use = %q, want %q", configPathCmd.Use, "path")
	}
	if configPathCmd.RunE == nil {
		t.Error("configPathCmd.RunE should be set")
	}
}

func TestConfigSchemaCommand(t *testing.T) {
	if configSchemaCmd.Use != "schema" {
		t.Errorf("configSchemaCmd.Use = %q, want %q", configSchemaCmd.Use, "schema")
	}
	if configSchemaCmd.RunE == nil {
		t.Error("configSchemaCmd.RunE should be set")
	}

	// Check that it accepts no arguments
	if configSchemaCmd.Args == nil {
		t.Error("configSchemaCmd.Args should be set to NoArgs")
	}
}

func TestConfigInteractiveCommand(t *testing.T) {
	if configCmd.RunE == nil {
		t.Error("configCmd.RunE should be set for interactive mode")
	}
}

func TestForceResetFlag(t *testing.T) {
	// Test that the forceReset variable exists and is a bool
	// We can't directly test the variable's value as it's package-level
	// but we can verify the flag is properly bound
	flag := configResetCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("force flag not found")
	}

	// Set the flag to test it's writable
	err := flag.Value.Set("true")
	if err != nil {
		t.Errorf("Failed to set force flag: %v", err)
	}
}
