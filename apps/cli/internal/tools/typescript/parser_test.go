package typescript

import (
	"testing"

	"github.com/detent/cli/internal/errors"
	"github.com/detent/cli/internal/tools/parser"
)

func TestParser_ID(t *testing.T) {
	p := NewParser()
	if got := p.ID(); got != "typescript" {
		t.Errorf("ID() = %q, want %q", got, "typescript")
	}
}

func TestParser_Priority(t *testing.T) {
	p := NewParser()
	if got := p.Priority(); got != 90 {
		t.Errorf("Priority() = %d, want %d", got, 90)
	}
}

func TestParser_CanParse(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	tests := []struct {
		name     string
		line     string
		expected float64
	}{
		{
			name:     "standard TypeScript error with code",
			line:     "src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.",
			expected: 0.95,
		},
		{
			name:     "TypeScript error in .tsx file",
			line:     "components/Button.tsx(25,10): error TS2749: 'Props' refers to a value, but is being used as a type here.",
			expected: 0.95,
		},
		{
			name:     "TypeScript error without error code",
			line:     "src/index.ts(1,1): Cannot find module 'missing-package'.",
			expected: 0.95,
		},
		{
			name:     "TypeScript error with implicit any",
			line:     "lib/utils.ts(15,3): error TS7006: Parameter 'x' implicitly has an 'any' type.",
			expected: 0.95,
		},
		{
			name:     "Go error (should not match)",
			line:     "main.go:25:10: undefined: foo",
			expected: 0,
		},
		{
			name:     "ESLint error (should not match)",
			line:     "  10:5  error  Unexpected var, use let or const instead  no-var",
			expected: 0,
		},
		{
			name:     "generic error (should not match)",
			line:     "Error: something went wrong",
			expected: 0,
		},
		{
			name:     "empty line",
			line:     "",
			expected: 0,
		},
		{
			name:     "plain text",
			line:     "Building project...",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.CanParse(tt.line, ctx)
			if got != tt.expected {
				t.Errorf("CanParse(%q) = %v, want %v", tt.line, got, tt.expected)
			}
		})
	}
}

func TestParser_Parse(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	tests := []struct {
		name     string
		line     string
		expected *errors.ExtractedError
	}{
		{
			name: "standard TypeScript error with code",
			line: "src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.",
			expected: &errors.ExtractedError{
				Message:  "Type 'string' is not assignable to type 'number'.",
				File:     "src/app.ts",
				Line:     10,
				Column:   5,
				RuleID:   "TS2322",
				Severity: "error",
				Category: errors.CategoryTypeCheck,
				Source:   errors.SourceTypeScript,
				Raw:      "src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.",
			},
		},
		{
			name: "TypeScript error in .tsx file with suggestion",
			line: "components/Button.tsx(25,10): error TS2749: 'Props' refers to a value, but is being used as a type here. Did you mean 'typeof Props'?",
			expected: &errors.ExtractedError{
				Message:  "'Props' refers to a value, but is being used as a type here. Did you mean 'typeof Props'?",
				File:     "components/Button.tsx",
				Line:     25,
				Column:   10,
				RuleID:   "TS2749",
				Severity: "error",
				Category: errors.CategoryTypeCheck,
				Source:   errors.SourceTypeScript,
				Raw:      "components/Button.tsx(25,10): error TS2749: 'Props' refers to a value, but is being used as a type here. Did you mean 'typeof Props'?",
			},
		},
		{
			name: "TypeScript implicit any error",
			line: "lib/utils.ts(15,3): error TS7006: Parameter 'x' implicitly has an 'any' type.",
			expected: &errors.ExtractedError{
				Message:  "Parameter 'x' implicitly has an 'any' type.",
				File:     "lib/utils.ts",
				Line:     15,
				Column:   3,
				RuleID:   "TS7006",
				Severity: "error",
				Category: errors.CategoryTypeCheck,
				Source:   errors.SourceTypeScript,
				Raw:      "lib/utils.ts(15,3): error TS7006: Parameter 'x' implicitly has an 'any' type.",
			},
		},
		{
			name: "TypeScript error without error code",
			line: "src/index.ts(1,1): Cannot find module 'missing-package'.",
			expected: &errors.ExtractedError{
				Message:  "Cannot find module 'missing-package'.",
				File:     "src/index.ts",
				Line:     1,
				Column:   1,
				RuleID:   "",
				Severity: "error",
				Category: errors.CategoryTypeCheck,
				Source:   errors.SourceTypeScript,
				Raw:      "src/index.ts(1,1): Cannot find module 'missing-package'.",
			},
		},
		{
			name: "TypeScript error with nested path",
			line: "src/components/forms/Input.tsx(42,8): error TS2339: Property 'value' does not exist on type 'Props'.",
			expected: &errors.ExtractedError{
				Message:  "Property 'value' does not exist on type 'Props'.",
				File:     "src/components/forms/Input.tsx",
				Line:     42,
				Column:   8,
				RuleID:   "TS2339",
				Severity: "error",
				Category: errors.CategoryTypeCheck,
				Source:   errors.SourceTypeScript,
				Raw:      "src/components/forms/Input.tsx(42,8): error TS2339: Property 'value' does not exist on type 'Props'.",
			},
		},
		{
			name:     "Go error (should not parse)",
			line:     "main.go:25:10: undefined: foo",
			expected: nil,
		},
		{
			name:     "ESLint error (should not parse)",
			line:     "  10:5  error  Unexpected var, use let or const instead  no-var",
			expected: nil,
		},
		{
			name:     "empty line",
			line:     "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.line, ctx)

			if tt.expected == nil {
				if got != nil {
					t.Errorf("Parse(%q) = %+v, want nil", tt.line, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("Parse(%q) = nil, want %+v", tt.line, tt.expected)
			}

			if got.Message != tt.expected.Message {
				t.Errorf("Message = %q, want %q", got.Message, tt.expected.Message)
			}
			if got.File != tt.expected.File {
				t.Errorf("File = %q, want %q", got.File, tt.expected.File)
			}
			if got.Line != tt.expected.Line {
				t.Errorf("Line = %d, want %d", got.Line, tt.expected.Line)
			}
			if got.Column != tt.expected.Column {
				t.Errorf("Column = %d, want %d", got.Column, tt.expected.Column)
			}
			if got.RuleID != tt.expected.RuleID {
				t.Errorf("RuleID = %q, want %q", got.RuleID, tt.expected.RuleID)
			}
			if got.Category != tt.expected.Category {
				t.Errorf("Category = %q, want %q", got.Category, tt.expected.Category)
			}
			if got.Source != tt.expected.Source {
				t.Errorf("Source = %q, want %q", got.Source, tt.expected.Source)
			}
			if got.Raw != tt.expected.Raw {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.expected.Raw)
			}
			if got.Severity != tt.expected.Severity {
				t.Errorf("Severity = %q, want %q", got.Severity, tt.expected.Severity)
			}
		})
	}
}

func TestParser_IsNoise(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "TypeScript error is not noise",
			line: "src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.",
			want: false,
		},
		{
			name: "empty line is noise",
			line: "",
			want: true,
		},
		{
			name: "whitespace only line is noise",
			line: "   ",
			want: true,
		},
		{
			name: "tsc watch mode starting",
			line: "Starting compilation in watch mode...",
			want: true,
		},
		{
			name: "tsc file change detected",
			line: "File change detected. Starting incremental compilation...",
			want: true,
		},
		{
			name: "tsc error summary",
			line: "Found 3 errors. Watching for file changes.",
			want: true,
		},
		{
			name: "tsc version output",
			line: "Version 5.3.2",
			want: true,
		},
		{
			name: "tsc informational message",
			line: "message TS6194: Found 0 errors. Watching for file changes.",
			want: true,
		},
		{
			name: "tsc timestamp prefix",
			line: "[10:30:45 AM] File change detected.",
			want: true,
		},
		{
			name: "build message with path is noise (tsc build mode)",
			line: "Building project '/path/to/tsconfig.json'...",
			want: true,
		},
		{
			name: "generic build message is not noise",
			line: "Building application...",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.IsNoise(tt.line); got != tt.want {
				t.Errorf("IsNoise(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestParser_SupportsMultiLine(t *testing.T) {
	p := NewParser()
	if got := p.SupportsMultiLine(); got {
		t.Error("SupportsMultiLine() = true, want false")
	}
}

func TestParser_ContinueMultiLine(t *testing.T) {
	p := NewParser()
	if got := p.ContinueMultiLine("any line", nil); got {
		t.Error("ContinueMultiLine() = true, want false")
	}
}

func TestParser_FinishMultiLine(t *testing.T) {
	p := NewParser()
	if got := p.FinishMultiLine(nil); got != nil {
		t.Errorf("FinishMultiLine() = %+v, want nil", got)
	}
}

func TestParser_Reset(t *testing.T) {
	p := NewParser()
	// Reset should not panic
	p.Reset()
}

func TestParser_InterfaceCompliance(t *testing.T) {
	// This test verifies the compile-time interface check works
	var _ parser.ToolParser = (*Parser)(nil)
}

func TestParser_WorkflowContext(t *testing.T) {
	p := NewParser()

	ctx := &parser.ParseContext{
		Job:  "build-job",
		Step: "Type check",
		WorkflowContext: &errors.WorkflowContext{
			Job:  "build-job",
			Step: "Type check",
		},
	}

	err := p.Parse("src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.", ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.WorkflowContext == nil {
		t.Fatal("expected WorkflowContext to be set")
	}

	if err.WorkflowContext.Job != "build-job" {
		t.Errorf("WorkflowContext.Job = %q, want %q", err.WorkflowContext.Job, "build-job")
	}

	if err.WorkflowContext.Step != "Type check" {
		t.Errorf("WorkflowContext.Step = %q, want %q", err.WorkflowContext.Step, "Type check")
	}
}

func TestParser_WorkflowContextNil(t *testing.T) {
	p := NewParser()

	// Test with nil context
	err := p.Parse("src/app.ts(10,5): error TS2322: Type error.", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.WorkflowContext != nil {
		t.Error("expected WorkflowContext to be nil when context is nil")
	}

	// Test with context but nil WorkflowContext
	ctx := &parser.ParseContext{
		Job:  "build-job",
		Step: "Type check",
	}

	err = p.Parse("src/app.ts(10,5): error TS2322: Type error.", ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.WorkflowContext != nil {
		t.Error("expected WorkflowContext to be nil when ctx.WorkflowContext is nil")
	}
}

func TestParser_RealWorldSamples(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Real-world TypeScript error samples
	samples := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{
			name:    "type assignment error",
			line:    "src/app.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'.",
			wantErr: true,
		},
		{
			name:    "value used as type",
			line:    "components/Button.tsx(25,10): error TS2749: 'Props' refers to a value, but is being used as a type here. Did you mean 'typeof Props'?",
			wantErr: true,
		},
		{
			name:    "implicit any parameter",
			line:    "lib/utils.ts(15,3): error TS7006: Parameter 'x' implicitly has an 'any' type.",
			wantErr: true,
		},
		{
			name:    "missing module without error code",
			line:    "src/index.ts(1,1): Cannot find module 'missing-package'.",
			wantErr: true,
		},
		{
			name:    "property does not exist",
			line:    "src/api/client.ts(88,15): error TS2339: Property 'then' does not exist on type 'void'.",
			wantErr: true,
		},
		{
			name:    "argument not assignable",
			line:    "src/hooks/useData.ts(12,24): error TS2345: Argument of type 'string' is not assignable to parameter of type 'number'.",
			wantErr: true,
		},
		{
			name:    "missing return type",
			line:    "src/utils/format.ts(5,1): error TS7030: Not all code paths return a value.",
			wantErr: true,
		},
		{
			name:    "duplicate identifier",
			line:    "src/types/index.ts(10,10): error TS2300: Duplicate identifier 'User'.",
			wantErr: true,
		},
	}

	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			got := p.Parse(s.line, ctx)
			if s.wantErr && got == nil {
				t.Errorf("Parse(%q) = nil, want error", s.line)
			}
			if !s.wantErr && got != nil {
				t.Errorf("Parse(%q) = %+v, want nil", s.line, got)
			}
		})
	}
}

func TestParser_EdgeCases(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	tests := []struct {
		name           string
		line           string
		wantNil        bool
		expectedFile   string
		expectedLine   int
		expectedColumn int
		expectedRuleID string
	}{
		{
			name:           "file with dots in path",
			line:           "src/v2.0/app.ts(10,5): error TS2322: Type error.",
			wantNil:        false,
			expectedFile:   "src/v2.0/app.ts",
			expectedLine:   10,
			expectedColumn: 5,
			expectedRuleID: "TS2322",
		},
		{
			name:           "file with dashes",
			line:           "src/my-component.tsx(5,1): error TS2304: Cannot find name 'foo'.",
			wantNil:        false,
			expectedFile:   "src/my-component.tsx",
			expectedLine:   5,
			expectedColumn: 1,
			expectedRuleID: "TS2304",
		},
		{
			name:           "file with underscores",
			line:           "src/my_module.ts(100,50): error TS1005: ';' expected.",
			wantNil:        false,
			expectedFile:   "src/my_module.ts",
			expectedLine:   100,
			expectedColumn: 50,
			expectedRuleID: "TS1005",
		},
		{
			name:           "large line and column numbers",
			line:           "src/app.ts(9999,999): error TS2322: Type mismatch.",
			wantNil:        false,
			expectedFile:   "src/app.ts",
			expectedLine:   9999,
			expectedColumn: 999,
			expectedRuleID: "TS2322",
		},
		{
			name:           "absolute Unix path",
			line:           "/home/user/project/src/app.ts(10,5): error TS2322: Type error.",
			wantNil:        false,
			expectedFile:   "/home/user/project/src/app.ts",
			expectedLine:   10,
			expectedColumn: 5,
			expectedRuleID: "TS2322",
		},
		{
			name:           "Windows path with drive letter",
			line:           "C:\\Users\\project\\src\\app.ts(10,5): error TS2322: Type error.",
			wantNil:        false,
			expectedFile:   "C:\\Users\\project\\src\\app.ts",
			expectedLine:   10,
			expectedColumn: 5,
			expectedRuleID: "TS2322",
		},
		{
			name:    "invalid - .js file",
			line:    "src/app.js(10,5): error TS2322: Type error.",
			wantNil: true,
		},
		{
			name:    "invalid - missing parentheses",
			line:    "src/app.ts:10:5: error TS2322: Type error.",
			wantNil: true,
		},
		{
			name:    "invalid - colon separator instead of comma",
			line:    "src/app.ts(10:5): error TS2322: Type error.",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.line, ctx)

			if tt.wantNil {
				if got != nil {
					t.Errorf("Parse(%q) = %+v, want nil", tt.line, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("Parse(%q) = nil, want non-nil", tt.line)
			}

			if got.File != tt.expectedFile {
				t.Errorf("File = %q, want %q", got.File, tt.expectedFile)
			}
			if got.Line != tt.expectedLine {
				t.Errorf("Line = %d, want %d", got.Line, tt.expectedLine)
			}
			if got.Column != tt.expectedColumn {
				t.Errorf("Column = %d, want %d", got.Column, tt.expectedColumn)
			}
			if got.RuleID != tt.expectedRuleID {
				t.Errorf("RuleID = %q, want %q", got.RuleID, tt.expectedRuleID)
			}
		})
	}
}

func TestParser_TSErrorCategories(t *testing.T) {
	// Verify TSErrorCategories has valid category values
	validCategories := map[string]bool{
		"syntax":      true,
		"type":        true,
		"module":      true,
		"declaration": true,
		"config":      true,
		"build":       true,
		"strict":      true,
		"jsx":         true,
		"compiler":    true,
		"advanced":    true,
		"semantic":    true,
	}

	for prefix, category := range TSErrorCategories {
		if !validCategories[category] {
			t.Errorf("TSErrorCategories[%q] = %q is not a valid category", prefix, category)
		}
	}

	// Verify expected prefixes exist (including new ones)
	expectedPrefixes := []string{"TS1", "TS2", "TS3", "TS4", "TS5", "TS6", "TS7", "TS8", "TS9", "TS17", "TS18"}
	for _, prefix := range expectedPrefixes {
		if _, ok := TSErrorCategories[prefix]; !ok {
			t.Errorf("TSErrorCategories missing expected prefix %q", prefix)
		}
	}
}

func TestParser_CommonTSErrors(t *testing.T) {
	// Verify some common TypeScript error codes are present
	commonCodes := []string{
		"TS2322", // Type is not assignable
		"TS2339", // Property does not exist
		"TS2304", // Cannot find name
		"TS2345", // Argument type is not assignable
		"TS7006", // Parameter implicitly has any type
		"TS1005", // Token expected
	}

	for _, code := range commonCodes {
		if _, ok := CommonTSErrors[code]; !ok {
			t.Errorf("CommonTSErrors missing expected code %q", code)
		}
	}

	// Verify all descriptions are non-empty
	for code, desc := range CommonTSErrors {
		if desc == "" {
			t.Errorf("CommonTSErrors[%q] has empty description", code)
		}
	}
}

func TestParser_AllCommonErrorCodes(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Test that we can parse errors with all common TypeScript error codes
	for code, desc := range CommonTSErrors {
		line := "src/app.ts(10,5): error " + code + ": " + desc + "."
		err := p.Parse(line, ctx)
		if err == nil {
			t.Errorf("Failed to parse error with code %q: %q", code, line)
		} else if err.RuleID != code {
			t.Errorf("RuleID = %q, want %q for line %q", err.RuleID, code, line)
		}
	}
}

func TestParser_DeclarationFiles(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	tests := []struct {
		name         string
		line         string
		expectedFile string
		wantNil      bool
	}{
		{
			name:         "declaration file .d.ts",
			line:         "types/index.d.ts(5,10): error TS2304: Cannot find name 'Foo'.",
			expectedFile: "types/index.d.ts",
			wantNil:      false,
		},
		{
			name:         "declaration file .d.tsx",
			line:         "types/jsx.d.tsx(10,1): error TS2322: Type error in declaration.",
			expectedFile: "types/jsx.d.tsx",
			wantNil:      false,
		},
		{
			name:         "node_modules declaration file",
			line:         "node_modules/@types/react/index.d.ts(100,5): error TS2707: Generic type issue.",
			expectedFile: "node_modules/@types/react/index.d.ts",
			wantNil:      false,
		},
		{
			name:         "absolute path declaration file",
			line:         "/home/user/project/types/global.d.ts(1,1): error TS2300: Duplicate identifier.",
			expectedFile: "/home/user/project/types/global.d.ts",
			wantNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.line, ctx)
			if tt.wantNil {
				if got != nil {
					t.Errorf("Parse(%q) = %+v, want nil", tt.line, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Parse(%q) = nil, want non-nil", tt.line)
			}
			if got.File != tt.expectedFile {
				t.Errorf("File = %q, want %q", got.File, tt.expectedFile)
			}
		})
	}
}

func TestParser_ESModuleFiles(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	tests := []struct {
		name         string
		line         string
		expectedFile string
		wantNil      bool
	}{
		{
			name:         "ES module .mts file",
			line:         "src/module.mts(10,5): error TS2322: Type error.",
			expectedFile: "src/module.mts",
			wantNil:      false,
		},
		{
			name:         "CommonJS module .cts file",
			line:         "src/legacy.cts(20,10): error TS2339: Property does not exist.",
			expectedFile: "src/legacy.cts",
			wantNil:      false,
		},
		{
			name:         "ES module JSX .mtsx file",
			line:         "src/component.mtsx(5,1): error TS2786: Component error.",
			expectedFile: "src/component.mtsx",
			wantNil:      false,
		},
		{
			name:         "CommonJS JSX .ctsx file",
			line:         "src/legacy-component.ctsx(15,3): error TS2304: Cannot find name.",
			expectedFile: "src/legacy-component.ctsx",
			wantNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.line, ctx)
			if tt.wantNil {
				if got != nil {
					t.Errorf("Parse(%q) = %+v, want nil", tt.line, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Parse(%q) = nil, want non-nil", tt.line)
			}
			if got.File != tt.expectedFile {
				t.Errorf("File = %q, want %q", got.File, tt.expectedFile)
			}
		})
	}
}

func TestParser_PrettyOutput(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Test that ANSI escape codes are stripped before parsing
	tests := []struct {
		name         string
		line         string
		expectedFile string
		expectedCode string
		wantNil      bool
	}{
		{
			name:         "error with ANSI color codes",
			line:         "\x1b[31msrc/app.ts(10,5): error TS2322: Type error.\x1b[0m",
			expectedFile: "src/app.ts",
			expectedCode: "TS2322",
			wantNil:      false,
		},
		{
			name:         "error with bold ANSI codes",
			line:         "\x1b[1msrc/index.ts(1,1): error TS2304: Cannot find name.\x1b[22m",
			expectedFile: "src/index.ts",
			expectedCode: "TS2304",
			wantNil:      false,
		},
		{
			name:         "error with multiple ANSI codes",
			line:         "\x1b[36m\x1b[1mcomponents/Button.tsx(25,10):\x1b[0m error \x1b[31mTS2749\x1b[0m: Value used as type.",
			expectedFile: "components/Button.tsx",
			expectedCode: "TS2749",
			wantNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test CanParse with ANSI codes
			confidence := p.CanParse(tt.line, ctx)
			if tt.wantNil {
				if confidence > 0 {
					t.Errorf("CanParse(%q) = %v, want 0", tt.line, confidence)
				}
				return
			}
			if confidence != 0.95 {
				t.Errorf("CanParse(%q) = %v, want 0.95", tt.line, confidence)
			}

			// Test Parse with ANSI codes
			got := p.Parse(tt.line, ctx)
			if got == nil {
				t.Fatalf("Parse(%q) = nil, want non-nil", tt.line)
			}
			if got.File != tt.expectedFile {
				t.Errorf("File = %q, want %q", got.File, tt.expectedFile)
			}
			if got.RuleID != tt.expectedCode {
				t.Errorf("RuleID = %q, want %q", got.RuleID, tt.expectedCode)
			}
		})
	}
}

func TestParser_BuildModeNoise(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "projects in build list",
			line: "Projects in this build:",
			want: true,
		},
		{
			name: "project entry in build",
			line: "    * packages/core/tsconfig.json",
			want: true,
		},
		{
			name: "building project message",
			line: "Building project '/path/to/tsconfig.json'...",
			want: true,
		},
		{
			name: "project out of date",
			line: "Project 'packages/ui' is out of date because output file 'dist/index.js' does not exist",
			want: true,
		},
		{
			name: "project up to date",
			line: "Project 'packages/core' is up to date because newest input 'src/index.ts' is older than output 'dist/index.js'",
			want: true,
		},
		{
			name: "updating timestamps",
			line: "Updating output timestamps of project '/path/to/tsconfig.json'...",
			want: true,
		},
		{
			name: "skipping build",
			line: "Skipping build of project '/path/to/tsconfig.json' because output file 'dist/index.js' is newer than input file 'src/index.ts'",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.IsNoise(tt.line); got != tt.want {
				t.Errorf("IsNoise(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestParser_PrettyOutputNoise(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "pretty line number with pipe",
			line: "  10 | const x: number = 'string';",
			want: true,
		},
		{
			name: "pretty continuation with pipe",
			line: "     |        ^",
			want: true,
		},
		{
			name: "error underline tildes",
			line: "                      ~~~~~~~~",
			want: true,
		},
		{
			name: "pretty line number with ANSI",
			line: "\x1b[90m  10 |\x1b[0m const x = 'hello';",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.IsNoise(tt.line); got != tt.want {
				t.Errorf("IsNoise(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "red text",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "red text",
		},
		{
			name:     "bold text",
			input:    "\x1b[1mbold\x1b[22m",
			expected: "bold",
		},
		{
			name:     "multiple codes",
			input:    "\x1b[1m\x1b[31merror\x1b[0m: message",
			expected: "error: message",
		},
		{
			name:     "complex formatting",
			input:    "\x1b[36msrc/app.ts\x1b[0m:\x1b[33m10\x1b[0m:\x1b[33m5\x1b[0m - \x1b[91merror\x1b[0m \x1b[90mTS2322\x1b[0m: Type error",
			expected: "src/app.ts:10:5 - error TS2322: Type error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.input)
			if got != tt.expected {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParser_ProjectReferenceErrors(t *testing.T) {
	p := NewParser()
	ctx := &parser.ParseContext{}

	// Test project reference related error codes
	tests := []struct {
		name         string
		line         string
		expectedCode string
		wantNil      bool
	}{
		{
			name:         "referenced project composite error",
			line:         "tsconfig.json(1,1): error TS6310: Referenced project 'packages/core' must have setting 'composite': true.",
			expectedCode: "TS6310",
			wantNil:      true, // tsconfig.json doesn't match .ts/.tsx pattern
		},
		{
			name:         "project reference error in ts file",
			line:         "src/index.ts(1,1): error TS6307: File is specified in reference but not in files array.",
			expectedCode: "TS6307",
			wantNil:      false,
		},
		{
			name:         "file not under rootDir",
			line:         "src/utils/helper.ts(1,1): error TS6059: File '/path/to/file.ts' is not under 'rootDir'.",
			expectedCode: "TS6059",
			wantNil:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.line, ctx)
			if tt.wantNil {
				if got != nil {
					t.Errorf("Parse(%q) = %+v, want nil", tt.line, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Parse(%q) = nil, want non-nil", tt.line)
			}
			if got.RuleID != tt.expectedCode {
				t.Errorf("RuleID = %q, want %q", got.RuleID, tt.expectedCode)
			}
		})
	}
}
