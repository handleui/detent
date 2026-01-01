package cmd

import (
	"testing"
)

func TestPruneCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "prune command has correct use",
			wantUse: "prune",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if pruneCmd.Use != tt.wantUse {
				t.Errorf("pruneCmd.Use = %q, want %q", pruneCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestPruneCommandFlags(t *testing.T) {
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
			name:      "all flag exists",
			flagName:  "all",
			shorthand: "a",
			wantType:  "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := pruneCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Errorf("Flag %q not found", tt.flagName)
				return
			}

			if flag.Value.Type() != tt.wantType {
				t.Errorf("Flag %q has type %q, want %q", tt.flagName, flag.Value.Type(), tt.wantType)
			}

			if flag.Shorthand != tt.shorthand {
				t.Errorf("Flag %q has shorthand %q, want %q", tt.flagName, flag.Shorthand, tt.shorthand)
			}
		})
	}
}

func TestPruneCommandRunE(t *testing.T) {
	if pruneCmd.RunE == nil {
		t.Error("pruneCmd.RunE should be set")
	}
}

func TestPruneFlagDefaults(t *testing.T) {
	// Test that flag variables are initialized to false
	// We can verify this by checking the flag default values
	forceFlag := pruneCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("force flag not found")
	}

	allFlag := pruneCmd.Flags().Lookup("all")
	if allFlag == nil {
		t.Fatal("all flag not found")
	}

	// Default values should be "false" for bool flags
	if forceFlag.DefValue != "false" {
		t.Errorf("force flag default = %q, want %q", forceFlag.DefValue, "false")
	}
	if allFlag.DefValue != "false" {
		t.Errorf("all flag default = %q, want %q", allFlag.DefValue, "false")
	}
}
