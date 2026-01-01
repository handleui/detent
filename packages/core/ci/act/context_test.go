package act

import (
	"testing"

	"github.com/detentsh/core/ci"
)

func TestContextParser_ParseLine(t *testing.T) {
	parser := NewContextParser()

	tests := []struct {
		name            string
		line            string
		wantContext     *ci.LineContext
		wantCleanedLine string
		wantSkip        bool
	}{
		{
			name: "line with context and pipe",
			line: "[CI/build] | error: something",
			wantContext: &ci.LineContext{
				Job: "CI/build",
			},
			wantCleanedLine: "error: something",
			wantSkip:        false,
		},
		{
			name: "line with context no pipe",
			line: "[CI/build] some output",
			wantContext: &ci.LineContext{
				Job: "CI/build",
			},
			wantCleanedLine: "some output",
			wantSkip:        false,
		},
		{
			name: "line with context extra spacing",
			line: "[CI/build]   |   message with spaces",
			wantContext: &ci.LineContext{
				Job: "CI/build",
			},
			wantCleanedLine: "message with spaces",
			wantSkip:        false,
		},
		{
			name:            "line without context",
			line:            "just a regular line",
			wantContext:     nil,
			wantCleanedLine: "just a regular line",
			wantSkip:        false,
		},
		{
			name: "debug noise - nil only",
			line: "<nil>",
			wantContext: &ci.LineContext{
				IsNoise: true,
			},
			wantCleanedLine: "",
			wantSkip:        true,
		},
		{
			name: "debug noise - job strategy nil",
			line: "Job.Strategy: <nil>",
			wantContext: &ci.LineContext{
				IsNoise: true,
			},
			wantCleanedLine: "",
			wantSkip:        true,
		},
		{
			name: "debug noise - level debug with nil",
			line: "level=debug msg=something <nil>",
			wantContext: &ci.LineContext{
				IsNoise: true,
			},
			wantCleanedLine: "",
			wantSkip:        true,
		},
		{
			name: "debug noise - time log with nil",
			line: `time="2025-01-01T00:00:00" level=debug msg="test" <nil>`,
			wantContext: &ci.LineContext{
				IsNoise: true,
			},
			wantCleanedLine: "",
			wantSkip:        true,
		},
		{
			name:            "empty line",
			line:            "",
			wantContext:     nil,
			wantCleanedLine: "",
			wantSkip:        false,
		},
		{
			name: "complex job name with brackets",
			line: "[CI/[CLI] Lint] | running linter",
			wantContext: &ci.LineContext{
				Job: "CI/[CLI",
			},
			wantCleanedLine: "Lint] | running linter",
			wantSkip:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContext, gotCleanedLine, gotSkip := parser.ParseLine(tt.line)

			if gotSkip != tt.wantSkip {
				t.Errorf("ParseLine() skip = %v, want %v", gotSkip, tt.wantSkip)
				return
			}

			if gotCleanedLine != tt.wantCleanedLine {
				t.Errorf("ParseLine() cleanedLine = %q, want %q", gotCleanedLine, tt.wantCleanedLine)
			}

			if tt.wantContext == nil {
				if gotContext != nil {
					t.Errorf("ParseLine() context = %+v, want nil", gotContext)
				}
				return
			}

			if gotContext == nil {
				t.Errorf("ParseLine() context = nil, want %+v", tt.wantContext)
				return
			}

			if gotContext.Job != tt.wantContext.Job {
				t.Errorf("ParseLine() context.Job = %q, want %q", gotContext.Job, tt.wantContext.Job)
			}
			if gotContext.IsNoise != tt.wantContext.IsNoise {
				t.Errorf("ParseLine() context.IsNoise = %v, want %v", gotContext.IsNoise, tt.wantContext.IsNoise)
			}
		})
	}
}

func TestContextParser_ImplementsInterface(t *testing.T) {
	var _ ci.ContextParser = (*ContextParser)(nil)
}
