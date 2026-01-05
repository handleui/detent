package cmd

import (
	"testing"
)

func TestVersion(t *testing.T) {
	tests := []struct {
		name         string
		initialValue string
		wantDefault  bool
	}{
		{
			name:         "default version",
			initialValue: "dev",
			wantDefault:  true,
		},
		{
			name:         "custom version",
			initialValue: "1.2.3",
			wantDefault:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original value
			original := Version
			defer func() { Version = original }()

			// Set test value
			Version = tt.initialValue

			if tt.wantDefault {
				if Version != "dev" {
					t.Errorf("Version = %q, want %q", Version, "dev")
				}
			} else {
				if Version != tt.initialValue {
					t.Errorf("Version = %q, want %q", Version, tt.initialValue)
				}
			}
		})
	}
}
