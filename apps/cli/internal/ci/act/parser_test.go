package act

import (
	"sort"
	"testing"

	"github.com/detent/cli/internal/ci"
)

func TestParser_ParseLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantEvent *ci.JobEvent
		wantOK    bool
	}{
		// Detent marker tests (primary, reliable)
		{
			name: "detent job-start",
			line: `::detent::job-start::build`,
			wantEvent: &ci.JobEvent{
				JobName: "build",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-start with prefix",
			line: `[CI/build] | ::detent::job-start::build`,
			wantEvent: &ci.JobEvent{
				JobName: "build",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end success",
			line: `::detent::job-end::test::success`,
			wantEvent: &ci.JobEvent{
				JobName: "test",
				Action:  "finish",
				Success: true,
			},
			wantOK: true,
		},
		{
			name: "detent job-end failure",
			line: `::detent::job-end::lint::failure`,
			wantEvent: &ci.JobEvent{
				JobName: "lint",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end cancelled",
			line: `::detent::job-end::deploy::cancelled`,
			wantEvent: &ci.JobEvent{
				JobName: "deploy",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name:      "detent manifest - no job event",
			line:      `::detent::manifest::build,test,lint`,
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "detent invalid marker",
			line:      `::detent::unknown::something`,
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "detent job-start empty job id",
			line:      `::detent::job-start::`,
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name: "detent job-end missing status - defaults to failure",
			line: `::detent::job-end::build`,
			wantEvent: &ci.JobEvent{
				JobName: "build",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end empty status - defaults to failure",
			line: `::detent::job-end::build::`,
			wantEvent: &ci.JobEvent{
				JobName: "build",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end unknown status - defaults to failure",
			line: `::detent::job-end::build::unknown_status`,
			wantEvent: &ci.JobEvent{
				JobName: "build",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name:      "detent job-start invalid job ID - shell injection",
			line:      `::detent::job-start::exploit` + "`whoami`",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "detent job-start job ID with spaces",
			line:      `::detent::job-start::job with spaces`,
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "detent job-start job ID starting with number",
			line:      `::detent::job-start::123invalid`,
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name: "detent job-start with whitespace trimmed",
			line: `::detent::job-start::  build  `,
			wantEvent: &ci.JobEvent{
				JobName: "build",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},

		// Emoji fallback tests (for reusable workflows)
		{
			name: "emoji job start - simple",
			line: "[CI/Release] 🚀  Start image",
			wantEvent: &ci.JobEvent{
				JobName: "Release",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job start - brackets in name",
			line: "[CI/[CLI] Lint] 🚀  Start image",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Lint",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job start - padded name",
			line: "[CI/[CLI] Lint      ] 🚀  Start",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Lint",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job finish - succeeded",
			line: "[CI/[CLI] Test] 🏁  Job succeeded",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Test",
				Action:  "finish",
				Success: true,
			},
			wantOK: true,
		},
		{
			name: "emoji job finish - failed",
			line: "[Release/Release    ] 🏁  Job failed",
			wantEvent: &ci.JobEvent{
				JobName: "Release",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job skipped - simple",
			line: "[CI/Deploy] ⏭️  Skipping job because a]",
			wantEvent: &ci.JobEvent{
				JobName: "Deploy",
				Action:  "skip",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job skipped - brackets in name",
			line: "[CI/[CLI] Release] ⏭️  Skipping job because \"needs\" condition not met",
			wantEvent: &ci.JobEvent{
				JobName: "[CLI] Release",
				Action:  "skip",
				Success: false,
			},
			wantOK: true,
		},
		{
			name:      "emoji step success - ignored",
			line:      "[CI/[CLI] Lint] ✅  Success",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "emoji step error - ignored",
			line:      "[CI/[CLI] Lint] ❌  Failed",
			wantEvent: nil,
			wantOK:    false,
		},
		{
			name:      "emoji step progress - ignored",
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
			parser := New() // Fresh parser for each test
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

func TestParser_Manifest(t *testing.T) {
	t.Run("parses manifest and stores expected jobs", func(t *testing.T) {
		parser := New()

		// Initially no manifest
		if parser.HasManifest() {
			t.Error("HasManifest() = true, want false before parsing manifest")
		}
		if jobs := parser.ExpectedJobs(); jobs != nil {
			t.Errorf("ExpectedJobs() = %v, want nil before parsing manifest", jobs)
		}

		// Parse manifest
		event, ok := parser.ParseLine("::detent::manifest::build,test,lint")
		if ok {
			t.Error("ParseLine() ok = true for manifest, want false (manifest is not a job event)")
		}
		if event != nil {
			t.Errorf("ParseLine() event = %+v, want nil for manifest", event)
		}

		// Now manifest should be stored
		if !parser.HasManifest() {
			t.Error("HasManifest() = false, want true after parsing manifest")
		}

		jobs := parser.ExpectedJobs()
		if jobs == nil {
			t.Fatal("ExpectedJobs() = nil, want job list")
		}

		sort.Strings(jobs) // Sort for comparison
		expected := []string{"build", "lint", "test"}
		if len(jobs) != len(expected) {
			t.Errorf("ExpectedJobs() len = %d, want %d", len(jobs), len(expected))
		}
		for i, job := range jobs {
			if job != expected[i] {
				t.Errorf("ExpectedJobs()[%d] = %q, want %q", i, job, expected[i])
			}
		}
	})

	t.Run("only first manifest is stored", func(t *testing.T) {
		parser := New()

		// Parse first manifest
		parser.ParseLine("::detent::manifest::job1,job2")

		// Parse second manifest (should be ignored)
		parser.ParseLine("::detent::manifest::job3,job4,job5")

		jobs := parser.ExpectedJobs()
		sort.Strings(jobs)

		// Should only have jobs from first manifest
		expected := []string{"job1", "job2"}
		if len(jobs) != len(expected) {
			t.Errorf("ExpectedJobs() len = %d, want %d (first manifest only)", len(jobs), len(expected))
		}
	})

	t.Run("handles empty manifest", func(t *testing.T) {
		parser := New()

		parser.ParseLine("::detent::manifest::")

		if !parser.HasManifest() {
			t.Error("HasManifest() = false, want true even for empty manifest")
		}

		jobs := parser.ExpectedJobs()
		if jobs != nil {
			t.Errorf("ExpectedJobs() = %v, want nil for empty manifest", jobs)
		}
	})

	t.Run("trims whitespace from job IDs", func(t *testing.T) {
		parser := New()

		parser.ParseLine("::detent::manifest:: build , test , lint ")

		jobs := parser.ExpectedJobs()
		sort.Strings(jobs)

		expected := []string{"build", "lint", "test"}
		if len(jobs) != len(expected) {
			t.Errorf("ExpectedJobs() len = %d, want %d", len(jobs), len(expected))
		}
	})
}

func TestParser_DetentMarkerPriority(t *testing.T) {
	t.Run("detent markers take priority over emoji", func(t *testing.T) {
		parser := New()

		// Line with both detent marker and emoji (edge case)
		line := `[CI/build] 🚀 ::detent::job-start::actual-job`
		event, ok := parser.ParseLine(line)

		if !ok {
			t.Fatal("ParseLine() ok = false, want true")
		}

		// Should use detent marker, not emoji
		if event.JobName != "actual-job" {
			t.Errorf("ParseLine() JobName = %q, want %q (from detent marker)", event.JobName, "actual-job")
		}
	})
}

func TestParser_ImplementsInterface(t *testing.T) {
	var _ ci.Parser = (*Parser)(nil)
}
