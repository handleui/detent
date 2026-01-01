package act

import (
	"sort"
	"testing"

	"github.com/detentsh/core/ci"
)

func TestParser_ParseLineJobEvent(t *testing.T) {
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
				JobID:   "build",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-start with prefix",
			line: `[CI/build] | ::detent::job-start::build`,
			wantEvent: &ci.JobEvent{
				JobID:   "build",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end success",
			line: `::detent::job-end::test::success`,
			wantEvent: &ci.JobEvent{
				JobID:   "test",
				Action:  "finish",
				Success: true,
			},
			wantOK: true,
		},
		{
			name: "detent job-end failure",
			line: `::detent::job-end::lint::failure`,
			wantEvent: &ci.JobEvent{
				JobID:   "lint",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end cancelled",
			line: `::detent::job-end::deploy::cancelled`,
			wantEvent: &ci.JobEvent{
				JobID:   "deploy",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
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
				JobID:   "build",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end empty status - defaults to failure",
			line: `::detent::job-end::build::`,
			wantEvent: &ci.JobEvent{
				JobID:   "build",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "detent job-end unknown status - defaults to failure",
			line: `::detent::job-end::build::unknown_status`,
			wantEvent: &ci.JobEvent{
				JobID:   "build",
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
				JobID:   "build",
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
				JobID:   "Release",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job start - brackets in name",
			line: "[CI/[CLI] Lint] 🚀  Start image",
			wantEvent: &ci.JobEvent{
				JobID:   "[CLI] Lint",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job start - padded name",
			line: "[CI/[CLI] Lint      ] 🚀  Start",
			wantEvent: &ci.JobEvent{
				JobID:   "[CLI] Lint",
				Action:  "start",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job finish - succeeded",
			line: "[CI/[CLI] Test] 🏁  Job succeeded",
			wantEvent: &ci.JobEvent{
				JobID:   "[CLI] Test",
				Action:  "finish",
				Success: true,
			},
			wantOK: true,
		},
		{
			name: "emoji job finish - failed",
			line: "[Release/Release    ] 🏁  Job failed",
			wantEvent: &ci.JobEvent{
				JobID:   "Release",
				Action:  "finish",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job skipped - simple",
			line: "[CI/Deploy] ⏭️  Skipping job because a]",
			wantEvent: &ci.JobEvent{
				JobID:   "Deploy",
				Action:  "skip",
				Success: false,
			},
			wantOK: true,
		},
		{
			name: "emoji job skipped - brackets in name",
			line: "[CI/[CLI] Release] ⏭️  Skipping job because \"needs\" condition not met",
			wantEvent: &ci.JobEvent{
				JobID:   "[CLI] Release",
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
			gotEvent, gotOK := parser.ParseLineJobEvent(tt.line)

			if gotOK != tt.wantOK {
				t.Errorf("ParseLineJobEvent() ok = %v, want %v", gotOK, tt.wantOK)
				return
			}

			if tt.wantEvent == nil {
				if gotEvent != nil {
					t.Errorf("ParseLineJobEvent() event = %+v, want nil", gotEvent)
				}
				return
			}

			if gotEvent == nil {
				t.Errorf("ParseLineJobEvent() event = nil, want %+v", tt.wantEvent)
				return
			}

			if gotEvent.JobID != tt.wantEvent.JobID {
				t.Errorf("ParseLineJobEvent() JobID = %q, want %q", gotEvent.JobID, tt.wantEvent.JobID)
			}
			if gotEvent.Action != tt.wantEvent.Action {
				t.Errorf("ParseLineJobEvent() Action = %q, want %q", gotEvent.Action, tt.wantEvent.Action)
			}
			if gotEvent.Success != tt.wantEvent.Success {
				t.Errorf("ParseLineJobEvent() Success = %v, want %v", gotEvent.Success, tt.wantEvent.Success)
			}
		})
	}
}

func TestParser_Manifest(t *testing.T) {
	t.Run("parses v1 manifest and stores expected jobs", func(t *testing.T) {
		parser := New()

		// Initially no manifest
		if parser.HasManifest() {
			t.Error("HasManifest() = true, want false before parsing manifest")
		}
		if jobs := parser.ExpectedJobs(); jobs != nil {
			t.Errorf("ExpectedJobs() = %v, want nil before parsing manifest", jobs)
		}

		// Parse manifest - returns ManifestEvent
		event, ok := parser.ParseLine("::detent::manifest::build,test,lint")
		if !ok {
			t.Error("ParseLine() ok = false for manifest, want true")
		}
		manifestEvent, isManifest := event.(*ci.ManifestEvent)
		if !isManifest {
			t.Fatalf("ParseLine() event type = %T, want *ci.ManifestEvent", event)
		}
		if manifestEvent.Manifest == nil {
			t.Fatal("ManifestEvent.Manifest = nil, want non-nil")
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

	t.Run("parses v2 manifest with steps", func(t *testing.T) {
		parser := New()

		// V2 manifest with full job/step info
		v2Manifest := `::detent::manifest::v2::{"v":2,"jobs":[{"id":"build","name":"Build","steps":["Checkout","Install","Build"]},{"id":"deploy","name":"Deploy","uses":"org/repo/.github/workflows/deploy.yml@main"}]}`
		event, ok := parser.ParseLine(v2Manifest)

		if !ok {
			t.Error("ParseLine() ok = false for v2 manifest, want true")
		}

		manifestEvent, isManifest := event.(*ci.ManifestEvent)
		if !isManifest {
			t.Fatalf("ParseLine() event type = %T, want *ci.ManifestEvent", event)
		}

		manifest := manifestEvent.Manifest
		if manifest == nil {
			t.Fatal("ManifestEvent.Manifest = nil, want non-nil")
		}

		if manifest.Version != 2 {
			t.Errorf("Manifest.Version = %d, want 2", manifest.Version)
		}

		if len(manifest.Jobs) != 2 {
			t.Fatalf("Manifest.Jobs len = %d, want 2", len(manifest.Jobs))
		}

		// Check first job (regular with steps)
		job0 := manifest.Jobs[0]
		if job0.ID != "build" {
			t.Errorf("Jobs[0].ID = %q, want %q", job0.ID, "build")
		}
		if job0.Name != "Build" {
			t.Errorf("Jobs[0].Name = %q, want %q", job0.Name, "Build")
		}
		if len(job0.Steps) != 3 {
			t.Errorf("Jobs[0].Steps len = %d, want 3", len(job0.Steps))
		}
		if job0.Uses != "" {
			t.Errorf("Jobs[0].Uses = %q, want empty", job0.Uses)
		}

		// Check second job (reusable workflow)
		job1 := manifest.Jobs[1]
		if job1.ID != "deploy" {
			t.Errorf("Jobs[1].ID = %q, want %q", job1.ID, "deploy")
		}
		if job1.Uses == "" {
			t.Error("Jobs[1].Uses = empty, want reusable workflow ref")
		}
		if len(job1.Steps) != 0 {
			t.Errorf("Jobs[1].Steps len = %d, want 0 for reusable workflow", len(job1.Steps))
		}
	})

	t.Run("parses v2 manifest with base64 encoding", func(t *testing.T) {
		parser := New()

		// V2 manifest with base64-encoded JSON (security-hardened format)
		// Base64 of: {"v":2,"jobs":[{"id":"test","name":"Test","steps":["Checkout","Run tests"]}]}
		b64Manifest := `::detent::manifest::v2::b64::eyJ2IjoyLCJqb2JzIjpbeyJpZCI6InRlc3QiLCJuYW1lIjoiVGVzdCIsInN0ZXBzIjpbIkNoZWNrb3V0IiwiUnVuIHRlc3RzIl19XX0=`
		event, ok := parser.ParseLine(b64Manifest)

		if !ok {
			t.Error("ParseLine() ok = false for v2 base64 manifest, want true")
		}

		manifestEvent, isManifest := event.(*ci.ManifestEvent)
		if !isManifest {
			t.Fatalf("ParseLine() event type = %T, want *ci.ManifestEvent", event)
		}

		manifest := manifestEvent.Manifest
		if manifest == nil {
			t.Fatal("ManifestEvent.Manifest = nil, want non-nil")
		}

		if manifest.Version != 2 {
			t.Errorf("Manifest.Version = %d, want 2", manifest.Version)
		}

		if len(manifest.Jobs) != 1 {
			t.Fatalf("Manifest.Jobs len = %d, want 1", len(manifest.Jobs))
		}

		job0 := manifest.Jobs[0]
		if job0.ID != "test" {
			t.Errorf("Jobs[0].ID = %q, want %q", job0.ID, "test")
		}
		if job0.Name != "Test" {
			t.Errorf("Jobs[0].Name = %q, want %q", job0.Name, "Test")
		}
		if len(job0.Steps) != 2 {
			t.Errorf("Jobs[0].Steps len = %d, want 2", len(job0.Steps))
		}
	})

	t.Run("rejects invalid base64 in manifest", func(t *testing.T) {
		parser := New()

		// Invalid base64 (contains invalid character !)
		invalidB64Manifest := `::detent::manifest::v2::b64::!!!invalid-base64!!!`
		_, ok := parser.ParseLine(invalidB64Manifest)

		if ok {
			t.Error("ParseLine() ok = true for invalid base64, want false")
		}
	})

	t.Run("only first manifest is stored", func(t *testing.T) {
		parser := New()

		// Parse first manifest
		parser.ParseLine("::detent::manifest::job1,job2")

		// Parse second manifest (should be ignored)
		event, ok := parser.ParseLine("::detent::manifest::job3,job4,job5")
		if ok {
			t.Error("ParseLine() ok = true for second manifest, want false (ignored)")
		}
		if event != nil {
			t.Errorf("ParseLine() event = %+v, want nil for second manifest", event)
		}

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

func TestParser_StepEvents(t *testing.T) {
	t.Run("parses step-start event", func(t *testing.T) {
		parser := New()

		event, ok := parser.ParseLine("::detent::step-start::build::0::Checkout")
		if !ok {
			t.Error("ParseLine() ok = false, want true")
		}

		stepEvent, isStep := event.(*ci.StepEvent)
		if !isStep {
			t.Fatalf("ParseLine() event type = %T, want *ci.StepEvent", event)
		}

		if stepEvent.JobID != "build" {
			t.Errorf("StepEvent.JobID = %q, want %q", stepEvent.JobID, "build")
		}
		if stepEvent.StepIdx != 0 {
			t.Errorf("StepEvent.StepIdx = %d, want 0", stepEvent.StepIdx)
		}
		if stepEvent.StepName != "Checkout" {
			t.Errorf("StepEvent.StepName = %q, want %q", stepEvent.StepName, "Checkout")
		}
	})

	t.Run("parses step-start with special characters in name", func(t *testing.T) {
		parser := New()

		event, ok := parser.ParseLine("::detent::step-start::test::2::Run npm test && lint")
		if !ok {
			t.Error("ParseLine() ok = false, want true")
		}

		stepEvent := event.(*ci.StepEvent)
		if stepEvent.StepName != "Run npm test && lint" {
			t.Errorf("StepEvent.StepName = %q, want %q", stepEvent.StepName, "Run npm test && lint")
		}
	})

	t.Run("rejects invalid job ID in step event", func(t *testing.T) {
		parser := New()

		_, ok := parser.ParseLine("::detent::step-start::invalid job::0::Step")
		if ok {
			t.Error("ParseLine() ok = true for invalid job ID, want false")
		}
	})

	t.Run("rejects invalid step index", func(t *testing.T) {
		parser := New()

		_, ok := parser.ParseLine("::detent::step-start::build::abc::Step")
		if ok {
			t.Error("ParseLine() ok = true for invalid step index, want false")
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

		jobEvent, isJob := event.(*ci.JobEvent)
		if !isJob {
			t.Fatalf("ParseLine() event type = %T, want *ci.JobEvent", event)
		}

		// Should use detent marker, not emoji
		if jobEvent.JobID != "actual-job" {
			t.Errorf("ParseLine() JobID = %q, want %q (from detent marker)", jobEvent.JobID, "actual-job")
		}
	})
}
