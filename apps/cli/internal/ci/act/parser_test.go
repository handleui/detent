package act

import (
	"testing"

	"github.com/detent/cli/internal/ci"
)

func TestParser_ParseLine(t *testing.T) {
	parser := New()

	tests := []struct {
		name      string
		line      string
		wantEvent *ci.JobEvent
		wantOK    bool
	}{
		{
			name: "job start - simple",
			line: "[CI/Release] 🚀  Start image",
			wantEvent: &ci.JobEvent{
				JobName: "Release",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "job start - brackets in name",
			line: "[CI/[CLI] Lint] 🚀  Start image",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Lint",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "job start - padded name",
			line: "[CI/[CLI] Lint      ] 🚀  Start",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Lint",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "job finish - succeeded",
			line: "[CI/[CLI] Test] 🏁  Job succeeded",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Test",
				Action:  "finish",
				Success: true,
			},
			wantOK: true,
		},
		{
			name: "job finish - failed",
			line: "[Release/Release    ] 🏁  Job failed",
			wantEvent: &ci.JobEvent{
				JobName: "Release",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "job skipped - simple",
			line: "[CI/Deploy] ⏭️  Skipping job because a]",
			wantEvent: &ci.JobEvent{
				JobName: "Deploy",
				Action:  "skip",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "job skipped - brackets in name",
			line: "[CI/[CLI] Release] ⏭️  Skipping job because \"needs\" condition not met",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Release",
				Action:  "skip",
				Success: false,
			},
			wantOK: true,
		},
		{
			name:      "step success - ignored",
			line:      "[CI/[CLI] Lint] ✅  Success",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "step error - ignored",
			line:      "[CI/[CLI] Lint] ❌  Failed",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "step progress - ignored",
			line:      "[CI/[CLI] Test] ⭐  Run Main test",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "debug line - no emoji",
			line:      "[DEBUG] some debug output",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "empty line",
			line:      "",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "no bracket",
			line:      "some random output",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "time log line",
			line:      `time="2025-01-01T00:00:00" level=debug msg="test"`,
			wantEvent: nil,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEvent, gotOK := parser.ParseLine(tt.line)

			if gotOK != tt.wantOK {
				t.Errorf("ParseLine() ok = %v, want %v", gotOK, tt.wantOK)
				return
			}

			if tt.wantEvent == nil {
				if gotEvent != nil {
					t.Errorf("ParseLine() event = %+v, want nil", gotEvent)
				}
				return
			}

			if gotEvent == nil {
				t.Errorf("ParseLine() event = nil, want %+v", tt.wantEvent)
				return
			}

			if gotEvent.JobName != tt.wantEvent.JobName {
				t.Errorf("ParseLine() JobName = %q, want %q", gotEvent.JobName, tt.wantEvent.JobName)
			}
			if gotEvent.Action != tt.wantEvent.Action {
				t.Errorf("ParseLine() Action = %q, want %q", gotEvent.Action, tt.wantEvent.Action)
			}
			if gotEvent.Success != tt.wantEvent.Success {
				t.Errorf("ParseLine() Success = %v, want %v", gotEvent.Success, tt.wantEvent.Success)
			}
		})
	}
}

func TestParser_ImplementsInterface(t *testing.T) {
	var _ ci.Parser = (*Parser)(nil)
}
