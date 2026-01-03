package cmd

import (
	"testing"

	"github.com/detent/cli/internal/persistence"
)

func TestCleanCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "clean command has correct use",
			wantUse: "clean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cleanCmd.Use != tt.wantUse {
				t.Errorf("cleanCmd.Use = %q, want %q", cleanCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestCleanCommandFlags(t *testing.T) {
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
		{
			name:      "retention flag exists",
			flagName:  "retention",
			shorthand: "r",
			wantType:  "int",
		},
		{
			name:      "dry-run flag exists",
			flagName:  "dry-run",
			shorthand: "",
			wantType:  "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := cleanCmd.Flags().Lookup(tt.flagName)
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

func TestCleanCommandRunE(t *testing.T) {
	if cleanCmd.RunE == nil {
		t.Error("cleanCmd.RunE should be set")
	}
}

func TestCleanFlagDefaults(t *testing.T) {
	forceFlag := cleanCmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Fatal("force flag not found")
	}

	allFlag := cleanCmd.Flags().Lookup("all")
	if allFlag == nil {
		t.Fatal("all flag not found")
	}

	retentionFlag := cleanCmd.Flags().Lookup("retention")
	if retentionFlag == nil {
		t.Fatal("retention flag not found")
	}

	dryRunFlag := cleanCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("dry-run flag not found")
	}

	if forceFlag.DefValue != "false" {
		t.Errorf("force flag default = %q, want %q", forceFlag.DefValue, "false")
	}
	if allFlag.DefValue != "false" {
		t.Errorf("all flag default = %q, want %q", allFlag.DefValue, "false")
	}
	if retentionFlag.DefValue != "30" {
		t.Errorf("retention flag default = %q, want %q", retentionFlag.DefValue, "30")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("dry-run flag default = %q, want %q", dryRunFlag.DefValue, "false")
	}
}

func TestProcessCleanDB(t *testing.T) {
	originalProcessCleanDB := processCleanDB
	defer func() { processCleanDB = originalProcessCleanDB }()

	called := false
	processCleanDB = func(_ string, _ int, _ bool) (*persistence.GCStats, error) {
		called = true
		return &persistence.GCStats{
			RunsDeleted:   5,
			ErrorsDeleted: 10,
		}, nil
	}

	stats, err := processCleanDB("/fake/path.db", 30, false)
	if err != nil {
		t.Errorf("processCleanDB returned error: %v", err)
	}
	if !called {
		t.Error("processCleanDB was not called")
	}
	if stats.RunsDeleted != 5 {
		t.Errorf("RunsDeleted = %d, want 5", stats.RunsDeleted)
	}
	if stats.ErrorsDeleted != 10 {
		t.Errorf("ErrorsDeleted = %d, want 10", stats.ErrorsDeleted)
	}
}
