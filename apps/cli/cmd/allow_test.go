package cmd

import (
	"testing"
)

func TestAllowCommand(t *testing.T) {
	tests := []struct {
		name    string
		wantUse string
	}{
		{
			name:    "allow command has correct use",
			wantUse: "allow [command]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if allowCmd.Use != tt.wantUse {
				t.Errorf("allowCmd.Use = %q, want %q", allowCmd.Use, tt.wantUse)
			}
		})
	}
}

func TestAllowCommandFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		shorthand string
		wantType  string
	}{
		{
			name:      "list flag exists",
			flagName:  "list",
			shorthand: "l",
			wantType:  "bool",
		},
		{
			name:      "remove flag exists",
			flagName:  "remove",
			shorthand: "r",
			wantType:  "bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := allowCmd.Flags().Lookup(tt.flagName)
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

func TestAllowCommandArgs(t *testing.T) {
	// Test that allow command accepts maximum 1 argument
	if allowCmd.Args == nil {
		t.Error("allowCmd.Args should be set")
		return
	}

	// cobra.MaximumNArgs(1) allows 0 or 1 args
	testCases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "no args",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "one arg",
			args:    []string{"bun test"},
			wantErr: false,
		},
		{
			name:    "two args",
			args:    []string{"bun", "test"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := allowCmd.Args(allowCmd, tc.args)
			if (err != nil) != tc.wantErr {
				t.Errorf("allowCmd.Args() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestAllowCommandSilenceSettings(t *testing.T) {
	if !allowCmd.SilenceUsage {
		t.Error("allowCmd.SilenceUsage should be true")
	}
	if !allowCmd.SilenceErrors {
		t.Error("allowCmd.SilenceErrors should be true")
	}
}

func TestAllowCommandRunE(t *testing.T) {
	if allowCmd.RunE == nil {
		t.Error("allowCmd.RunE should be set")
	}
}
