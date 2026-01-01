package tools

import (
	"strings"
	"testing"
)

func TestDetectToolFromRun(t *testing.T) {
	tests := []struct {
		name     string
		run      string
		expected string
	}{
		// Go tools
		{
			name:     "go test",
			run:      "go test ./...",
			expected: "go",
		},
		{
			name:     "go build",
			run:      "go build -o app .",
			expected: "go",
		},
		{
			name:     "go vet",
			run:      "go vet ./...",
			expected: "go",
		},
		{
			name:     "golangci-lint",
			run:      "golangci-lint run ./...",
			expected: "go",
		},
		{
			name:     "golangci-lint with path",
			run:      "/usr/local/bin/golangci-lint run",
			expected: "go",
		},
		{
			name:     "staticcheck",
			run:      "staticcheck ./...",
			expected: "go",
		},
		{
			name:     "govulncheck",
			run:      "govulncheck ./...",
			expected: "go",
		},

		// TypeScript
		{
			name:     "tsc",
			run:      "tsc --noEmit",
			expected: "typescript",
		},
		{
			name:     "npx tsc",
			run:      "npx tsc --build",
			expected: "typescript",
		},
		{
			name:     "bun tsc",
			run:      "bun tsc --noEmit",
			expected: "typescript",
		},
		{
			name:     "bunx tsc",
			run:      "bunx tsc",
			expected: "typescript",
		},
		{
			name:     "yarn tsc",
			run:      "yarn tsc --noEmit",
			expected: "typescript",
		},

		// ESLint
		{
			name:     "eslint",
			run:      "eslint src/",
			expected: "eslint",
		},
		{
			name:     "npx eslint",
			run:      "npx eslint .",
			expected: "eslint",
		},
		{
			name:     "bunx eslint",
			run:      "bunx eslint src/",
			expected: "eslint",
		},

		// Rust tools
		{
			name:     "cargo test",
			run:      "cargo test",
			expected: "rust",
		},
		{
			name:     "cargo build",
			run:      "cargo build --release",
			expected: "rust",
		},
		{
			name:     "cargo clippy",
			run:      "cargo clippy -- -D warnings",
			expected: "rust",
		},
		{
			name:     "cargo fmt",
			run:      "cargo fmt --check",
			expected: "rust",
		},
		{
			name:     "rustc",
			run:      "rustc main.rs",
			expected: "rust",
		},

		// Multi-line commands
		{
			name: "multi-line with go test",
			run: `echo "Running tests"
go test ./...
echo "Done"`,
			expected: "go",
		},
		{
			name: "multi-line with comment",
			run: `# Run lint
eslint src/`,
			expected: "eslint",
		},

		// Chained commands
		{
			name:     "chained with &&",
			run:      "npm install && npm test",
			expected: "", // npm install has no pattern, npm test has no pattern
		},
		{
			name:     "chained go commands",
			run:      "go build ./... && go test ./...",
			expected: "go",
		},
		{
			name:     "chained with semicolon",
			run:      "go fmt ./...; go test ./...",
			expected: "go",
		},

		// No tool detected
		{
			name:     "echo only",
			run:      "echo 'Hello World'",
			expected: "",
		},
		{
			name:     "custom script",
			run:      "./scripts/build.sh",
			expected: "",
		},
		{
			name:     "npm install",
			run:      "npm install",
			expected: "",
		},
		{
			name:     "empty command",
			run:      "",
			expected: "",
		},
		{
			name:     "only comments",
			run:      "# This is a comment",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectToolFromRun(tt.run)
			if got != tt.expected {
				t.Errorf("DetectToolFromRun(%q) = %q, want %q", tt.run, got, tt.expected)
			}
		})
	}
}

func TestDetectAllToolsFromRun(t *testing.T) {
	tests := []struct {
		name        string
		run         string
		expectedIDs []string
	}{
		{
			name:        "single tool",
			run:         "go test ./...",
			expectedIDs: []string{"go"},
		},
		{
			name:        "chained same tool",
			run:         "go build ./... && go test ./...",
			expectedIDs: []string{"go"}, // Should dedupe
		},
		{
			name:        "multiple different tools",
			run:         "eslint src/ && tsc --noEmit",
			expectedIDs: []string{"eslint", "typescript"},
		},
		{
			name: "multi-line multiple tools",
			run: `eslint src/
tsc --noEmit
cargo test`,
			expectedIDs: []string{"eslint", "typescript", "rust"},
		},
		{
			name:        "piped commands",
			run:         "go test ./... | grep FAIL",
			expectedIDs: []string{"go"},
		},
		{
			name:        "no tools",
			run:         "echo 'hello' && npm install",
			expectedIDs: []string{},
		},
		{
			name:        "tools with quotes preserved",
			run:         `go test -run "TestFoo" ./...`,
			expectedIDs: []string{"go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectAllToolsFromRun(tt.run)
			gotIDs := make([]string, len(got))
			for i, t := range got {
				gotIDs[i] = t.ID
			}

			if len(gotIDs) != len(tt.expectedIDs) {
				t.Errorf("DetectAllToolsFromRun(%q) got %d tools %v, want %d tools %v",
					tt.run, len(gotIDs), gotIDs, len(tt.expectedIDs), tt.expectedIDs)
				return
			}

			for i, expectedID := range tt.expectedIDs {
				if gotIDs[i] != expectedID {
					t.Errorf("DetectAllToolsFromRun(%q)[%d] = %q, want %q",
						tt.run, i, gotIDs[i], expectedID)
				}
			}
		})
	}
}

func TestSplitCommands(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single command",
			input:    "go test ./...",
			expected: []string{"go test ./..."},
		},
		{
			name:     "double ampersand",
			input:    "go build && go test",
			expected: []string{"go build ", " go test"},
		},
		{
			name:     "semicolon",
			input:    "go build; go test",
			expected: []string{"go build", " go test"},
		},
		{
			name:     "pipe",
			input:    "go test | grep FAIL",
			expected: []string{"go test ", " grep FAIL"},
		},
		{
			name:     "double pipe (or)",
			input:    "go test || echo failed",
			expected: []string{"go test ", " echo failed"},
		},
		{
			name:     "quoted string with &&",
			input:    `echo "build && test" && go test`,
			expected: []string{`echo "build && test" `, ` go test`},
		},
		{
			name:     "single quoted string",
			input:    `echo 'build && test' && go test`,
			expected: []string{`echo 'build && test' `, ` go test`},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "only separators",
			input:    "&&||;",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCommands(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitCommands(%q) got %d parts %v, want %d parts %v",
					tt.input, len(got), got, len(tt.expected), tt.expected)
				return
			}
			for i, exp := range tt.expected {
				if got[i] != exp {
					t.Errorf("splitCommands(%q)[%d] = %q, want %q",
						tt.input, i, got[i], exp)
				}
			}
		})
	}
}

func TestRegistry_HasDedicatedParser(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		name     string
		id       string
		expected bool
	}{
		{
			name:     "go parser exists",
			id:       "go",
			expected: true,
		},
		{
			name:     "typescript parser exists",
			id:       "typescript",
			expected: true,
		},
		{
			name:     "generic parser not dedicated",
			id:       "generic",
			expected: false,
		},
		{
			name:     "eslint parser exists",
			id:       "eslint",
			expected: true,
		},
		{
			name:     "rust parser exists",
			id:       "rust",
			expected: true,
		},
		{
			name:     "unknown tool",
			id:       "unknown",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.HasDedicatedParser(tt.id)
			if got != tt.expected {
				t.Errorf("HasDedicatedParser(%q) = %v, want %v", tt.id, got, tt.expected)
			}
		})
	}
}

func TestRegistry_SupportedToolIDs(t *testing.T) {
	r := DefaultRegistry()
	ids := r.SupportedToolIDs()

	// Should contain go, typescript, eslint, and rust
	hasGo := false
	hasTS := false
	hasESLint := false
	hasRust := false
	hasGeneric := false

	for _, id := range ids {
		switch id {
		case "go":
			hasGo = true
		case "typescript":
			hasTS = true
		case "eslint":
			hasESLint = true
		case "rust":
			hasRust = true
		case "generic":
			hasGeneric = true
		}
	}

	if !hasGo {
		t.Error("SupportedToolIDs should contain 'go'")
	}
	if !hasTS {
		t.Error("SupportedToolIDs should contain 'typescript'")
	}
	if !hasESLint {
		t.Error("SupportedToolIDs should contain 'eslint'")
	}
	if !hasRust {
		t.Error("SupportedToolIDs should contain 'rust'")
	}
	if hasGeneric {
		t.Error("SupportedToolIDs should NOT contain 'generic'")
	}
}

func TestRegistry_DetectAndCheckSupport(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		name          string
		run           string
		wantTool      string
		wantSupported bool
	}{
		{
			name:          "supported go test",
			run:           "go test ./...",
			wantTool:      "go",
			wantSupported: true,
		},
		{
			name:          "supported typescript",
			run:           "tsc --noEmit",
			wantTool:      "typescript",
			wantSupported: true,
		},
		{
			name:          "supported eslint",
			run:           "eslint src/",
			wantTool:      "eslint",
			wantSupported: true,
		},
		{
			name:          "supported rust cargo",
			run:           "cargo test",
			wantTool:      "rust",
			wantSupported: true,
		},
		{
			name:          "supported rust clippy",
			run:           "cargo clippy",
			wantTool:      "rust",
			wantSupported: true,
		},
		{
			name:          "no tool detected",
			run:           "echo hello",
			wantTool:      "",
			wantSupported: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTool, gotSupported := r.DetectAndCheckSupport(tt.run)
			if gotTool != tt.wantTool {
				t.Errorf("DetectAndCheckSupport(%q) tool = %q, want %q", tt.run, gotTool, tt.wantTool)
			}
			if gotSupported != tt.wantSupported {
				t.Errorf("DetectAndCheckSupport(%q) supported = %v, want %v", tt.run, gotSupported, tt.wantSupported)
			}
		})
	}
}

func TestRegistry_DetectAllAndCheckSupport(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		name                 string
		run                  string
		wantToolCount        int
		wantUnsupportedCount int
	}{
		{
			name:                 "single supported tool",
			run:                  "go test ./...",
			wantToolCount:        1,
			wantUnsupportedCount: 0,
		},
		{
			name:                 "multiple supported tools",
			run:                  "eslint src/ && tsc --noEmit",
			wantToolCount:        2,
			wantUnsupportedCount: 0,
		},
		{
			name:                 "all four supported tools",
			run:                  "go test && tsc && eslint . && cargo build",
			wantToolCount:        4,
			wantUnsupportedCount: 0,
		},
		{
			name:                 "no tools",
			run:                  "echo hello",
			wantToolCount:        0,
			wantUnsupportedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := r.DetectAllAndCheckSupport(tt.run)
			if len(tools) != tt.wantToolCount {
				t.Errorf("DetectAllAndCheckSupport(%q) got %d tools, want %d",
					tt.run, len(tools), tt.wantToolCount)
			}

			unsupportedCount := 0
			for _, tool := range tools {
				if !tool.Supported {
					unsupportedCount++
				}
			}
			if unsupportedCount != tt.wantUnsupportedCount {
				t.Errorf("DetectAllAndCheckSupport(%q) got %d unsupported, want %d",
					tt.run, unsupportedCount, tt.wantUnsupportedCount)
			}
		})
	}
}

func TestRegistry_DetectTools(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		name          string
		run           string
		opts          DetectionOptions
		wantToolCount int
		wantFirstID   string
		wantSupported bool // for checking first tool when CheckSupport is true
	}{
		{
			name:          "detect all tools with support check",
			run:           "eslint src/ && tsc --noEmit",
			opts:          DetectionOptions{CheckSupport: true},
			wantToolCount: 2,
			wantFirstID:   "eslint",
			wantSupported: true,
		},
		{
			name:          "detect first only",
			run:           "eslint src/ && tsc --noEmit",
			opts:          DetectionOptions{FirstOnly: true},
			wantToolCount: 1,
			wantFirstID:   "eslint",
		},
		{
			name:          "detect first with support check",
			run:           "go test ./...",
			opts:          DetectionOptions{FirstOnly: true, CheckSupport: true},
			wantToolCount: 1,
			wantFirstID:   "go",
			wantSupported: true,
		},
		{
			name:          "no tools detected",
			run:           "echo hello",
			opts:          DetectionOptions{CheckSupport: true},
			wantToolCount: 0,
			wantFirstID:   "",
		},
		{
			name:          "multiple tools all supported",
			run:           "go test && tsc && eslint . && cargo build",
			opts:          DetectionOptions{CheckSupport: true},
			wantToolCount: 4,
			wantFirstID:   "go",
			wantSupported: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.DetectTools(tt.run, tt.opts)

			if len(result.Tools) != tt.wantToolCount {
				t.Errorf("DetectTools(%q) got %d tools, want %d",
					tt.run, len(result.Tools), tt.wantToolCount)
			}

			if result.FirstID() != tt.wantFirstID {
				t.Errorf("DetectTools(%q).FirstID() = %q, want %q",
					tt.run, result.FirstID(), tt.wantFirstID)
			}

			if tt.opts.CheckSupport && tt.wantToolCount > 0 {
				if result.First().Supported != tt.wantSupported {
					t.Errorf("DetectTools(%q).First().Supported = %v, want %v",
						tt.run, result.First().Supported, tt.wantSupported)
				}
			}
		})
	}
}

func TestDetectionResult_Methods(t *testing.T) {
	t.Run("empty result", func(t *testing.T) {
		result := DetectionResult{}

		if result.HasTools() {
			t.Error("empty result should not have tools")
		}
		if result.FirstID() != "" {
			t.Errorf("empty result FirstID() = %q, want empty", result.FirstID())
		}
		if result.First().ID != "" {
			t.Errorf("empty result First().ID = %q, want empty", result.First().ID)
		}
		if !result.AllSupported() {
			t.Error("empty result should report AllSupported as true")
		}
		if len(result.Unsupported()) != 0 {
			t.Errorf("empty result Unsupported() should be empty, got %d", len(result.Unsupported()))
		}
	})

	t.Run("single supported tool", func(t *testing.T) {
		result := DetectionResult{
			Tools: []DetectedTool{
				{ID: "go", DisplayName: "go", Supported: true},
			},
		}

		if !result.HasTools() {
			t.Error("result should have tools")
		}
		if result.FirstID() != "go" {
			t.Errorf("FirstID() = %q, want go", result.FirstID())
		}
		if !result.AllSupported() {
			t.Error("AllSupported() should be true")
		}
		if len(result.Unsupported()) != 0 {
			t.Errorf("Unsupported() should be empty, got %d", len(result.Unsupported()))
		}
	})

	t.Run("mixed supported and unsupported", func(t *testing.T) {
		result := DetectionResult{
			Tools: []DetectedTool{
				{ID: "go", DisplayName: "go", Supported: true},
				{ID: "pytest", DisplayName: "pytest", Supported: false},
				{ID: "eslint", DisplayName: "eslint", Supported: true},
			},
		}

		if !result.HasTools() {
			t.Error("result should have tools")
		}
		if result.AllSupported() {
			t.Error("AllSupported() should be false with unsupported tool")
		}

		unsupported := result.Unsupported()
		if len(unsupported) != 1 {
			t.Errorf("Unsupported() should have 1 tool, got %d", len(unsupported))
		}
		if unsupported[0].ID != "pytest" {
			t.Errorf("Unsupported()[0].ID = %q, want pytest", unsupported[0].ID)
		}
	})
}

func TestFormatUnsupportedToolsWarning(t *testing.T) {
	tests := []struct {
		name             string
		unsupportedTools []DetectedTool
		supportedIDs     []string
		wantContains     []string
		wantEmpty        bool
	}{
		{
			name:             "no unsupported tools",
			unsupportedTools: []DetectedTool{},
			supportedIDs:     []string{"go", "typescript"},
			wantEmpty:        true,
		},
		{
			name: "single unsupported tool",
			unsupportedTools: []DetectedTool{
				{ID: "pytest", DisplayName: "pytest", Supported: false},
			},
			supportedIDs: []string{"go", "typescript"},
			wantContains: []string{"pytest", "not fully supported", "go", "typescript"},
		},
		{
			name: "two unsupported tools",
			unsupportedTools: []DetectedTool{
				{ID: "pytest", DisplayName: "pytest", Supported: false},
				{ID: "cargo", DisplayName: "cargo", Supported: false},
			},
			supportedIDs: []string{"go"},
			wantContains: []string{"pytest", "cargo", "and", "not fully supported"},
		},
		{
			name: "three unsupported tools",
			unsupportedTools: []DetectedTool{
				{ID: "pytest", DisplayName: "pytest", Supported: false},
				{ID: "cargo", DisplayName: "cargo", Supported: false},
				{ID: "jest", DisplayName: "jest", Supported: false},
			},
			supportedIDs: []string{"go", "typescript"},
			wantContains: []string{"pytest", "cargo", "jest", ", and ", "not fully supported"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatUnsupportedToolsWarning(tt.unsupportedTools, tt.supportedIDs)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("FormatUnsupportedToolsWarning() = %q, want empty", got)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatUnsupportedToolsWarning() = %q, should contain %q", got, want)
				}
			}
		})
	}
}

func TestFormatList(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		expected string
	}{
		{
			name:     "empty list",
			items:    []string{},
			expected: "",
		},
		{
			name:     "single item",
			items:    []string{"apple"},
			expected: "apple",
		},
		{
			name:     "two items",
			items:    []string{"apple", "banana"},
			expected: "apple and banana",
		},
		{
			name:     "three items",
			items:    []string{"apple", "banana", "cherry"},
			expected: "apple, banana, and cherry",
		},
		{
			name:     "four items",
			items:    []string{"apple", "banana", "cherry", "date"},
			expected: "apple, banana, cherry, and date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatList(tt.items)
			if got != tt.expected {
				t.Errorf("formatList(%v) = %q, want %q", tt.items, got, tt.expected)
			}
		})
	}
}

func TestExtractFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		// Go file patterns
		{name: "go colon format", line: "main.go:10:5: undefined: foo", expected: ".go"},
		{name: "go with path", line: "/path/to/file.go:10:5: error", expected: ".go"},
		{name: "go relative path", line: "internal/pkg/file.go:25:10: message", expected: ".go"},

		// TypeScript file patterns
		{name: "ts paren format", line: "src/app.ts(5,10): error TS2322", expected: ".ts"},
		{name: "tsx paren format", line: "components/Button.tsx(12,5): error TS2339", expected: ".tsx"},
		{name: "ts colon format", line: "src/app.ts:5:10: error TS2322", expected: ".ts"},

		// Rust file patterns
		{name: "rs colon format", line: "src/main.rs:10:5: error[E0308]", expected: ".rs"},
		{name: "toml cargo", line: "Cargo.toml:15:1: error: invalid key", expected: ".toml"},

		// JS/JSX patterns (eslint)
		{name: "js colon format", line: "src/index.js:5:10: error", expected: ".js"},
		{name: "jsx colon format", line: "src/App.jsx:12:5: error", expected: ".jsx"},
		{name: "mjs colon format", line: "lib/utils.mjs:8:3: warning", expected: ".mjs"},
		{name: "cjs colon format", line: "config.cjs:3:1: error", expected: ".cjs"},

		// No extension cases
		{name: "no extension", line: "some random text without file path", expected: ""},
		{name: "dot without file", line: "error: something.went.wrong", expected: ""},
		{name: "unknown extension", line: "file.unknown:10:5: error", expected: ""},
		{name: "empty line", line: "", expected: ""},
		{name: "dot at end", line: "file.go", expected: ""},  // No : or ( after extension
		{name: "rust error header", line: "error[E0308]: mismatched types", expected: ""},

		// Case insensitivity
		{name: "uppercase GO", line: "main.GO:10:5: error", expected: ".go"},
		{name: "uppercase TS", line: "app.TS(5,10): error", expected: ".ts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFileExtension(tt.line)
			if got != tt.expected {
				t.Errorf("extractFileExtension(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}

func TestRegistry_FindParser_ExtensionFastPath(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		name             string
		line             string
		expectedParserID string
	}{
		// Extension-based fast path should select correct parsers
		{name: "go error", line: "main.go:10:5: undefined: foo", expectedParserID: "go"},
		{name: "ts error", line: "src/app.ts(5,10): error TS2322: message", expectedParserID: "typescript"},
		{name: "tsx error", line: "Button.tsx(12,5): error TS2339: Property", expectedParserID: "typescript"},

		// ESLint unix format: file.js:line:col: message [severity/rule]
		{name: "js error eslint unix", line: "src/file.js:10:5: 'foo' is not defined [error/no-undef]", expectedParserID: "eslint"},

		// Lines without file extensions should use slow path and match based on content
		{name: "rust error header", line: "error[E0308]: mismatched types", expectedParserID: "rust"},

		// Rust location arrow (extension fast path should work after header sets inError)
		// Note: This tests that even though .rs extension matches rust parser, it still
		// needs to verify CanParse > 0, which only happens when in multi-line state
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := r.FindParser(tt.line, nil)
			if p == nil {
				t.Fatalf("FindParser(%q) returned nil", tt.line)
			}
			if p.ID() != tt.expectedParserID {
				t.Errorf("FindParser(%q) = %q, want %q", tt.line, p.ID(), tt.expectedParserID)
			}
		})
	}
}

func BenchmarkRegistry_FindParser_WithExtension(b *testing.B) {
	r := DefaultRegistry()

	// Lines with file extensions that parsers recognize (should use fast path)
	linesWithExt := []string{
		"main.go:10:5: undefined: foo",
		"src/app.ts(5,10): error TS2322: Type 'string' is not assignable to type 'number'.",
		"src/file.js:10:5: 'foo' is not defined [error/no-undef]",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range linesWithExt {
			r.FindParser(line, nil)
		}
	}
}

func BenchmarkRegistry_FindParser_WithoutExtension(b *testing.B) {
	r := DefaultRegistry()

	// Lines without file extensions (should use slow path)
	linesWithoutExt := []string{
		"error[E0308]: mismatched types",
		"  8:11  error  Missing semicolon  semi",
		"✖ 2 problems (1 error, 1 warning)",
		"some unknown error format",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range linesWithoutExt {
			r.FindParser(line, nil)
		}
	}
}

func BenchmarkRegistry_IsNoise(b *testing.B) {
	r := DefaultRegistry()

	// Representative sample of lines - mix of noise and real errors
	lines := []string{
		"=== RUN   TestFoo",
		"--- PASS: TestBar (0.00s)",
		"main.go:10:5: undefined: foo",
		"   Compiling myproject v0.1.0",
		"src/app.ts(5,10): error TS2322: Type 'string' is not assignable to type 'number'.",
		"  8:11  error  Missing semicolon  semi",
		"✖ 2 problems (1 error, 1 warning)",
		"error[E0308]: mismatched types",
		"    at Object.<anonymous> (/path/to/file.js:10:5)",
		"",
		"        deeply indented code",
		"Downloading dependencies...",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, line := range lines {
			r.IsNoise(line)
		}
	}
}

func TestRegistry_IsNoise(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		name    string
		line    string
		isNoise bool
	}{
		// Empty/whitespace
		{name: "empty line", line: "", isNoise: true},
		{name: "whitespace only", line: "   \t  ", isNoise: true},

		// Go test noise
		{name: "go test run", line: "=== RUN   TestFoo", isNoise: true},
		{name: "go test pass", line: "--- PASS: TestFoo (0.00s)", isNoise: true},
		{name: "go test skip", line: "--- SKIP: TestFoo (0.00s)", isNoise: true},
		{name: "go overall pass", line: "PASS", isNoise: true},
		{name: "go package pass", line: "ok  	github.com/foo/bar	0.123s", isNoise: true},
		{name: "go no test files", line: "?   	github.com/foo/bar	[no test files]", isNoise: true},

		// Rust/Cargo noise
		{name: "cargo compiling", line: "   Compiling myproject v0.1.0", isNoise: true},
		{name: "cargo finished", line: "   Finished dev [unoptimized + debuginfo] target(s)", isNoise: true},
		{name: "cargo running", line: "     Running `target/debug/myproject`", isNoise: true},
		{name: "rust test pass", line: "test tests::it_works ... ok", isNoise: true},
		{name: "rust test result", line: "test result: ok. 5 passed; 0 failed", isNoise: true},

		// TypeScript noise
		{name: "ts found errors", line: "Found 3 errors.", isNoise: true},
		{name: "ts version", line: "Version 5.0.4", isNoise: true},
		{name: "ts watch mode", line: "Starting compilation in watch mode...", isNoise: true},
		{name: "ts pretty pipe", line: "  12 |   const x = 1;", isNoise: true},

		// ESLint noise
		{name: "eslint problems", line: "✖ 2 problems (1 error, 1 warning)", isNoise: true},
		{name: "eslint pass", line: "✓ All files pass linting", isNoise: true},
		{name: "eslint fixable", line: "  1 error and 1 warning potentially fixable with the `--fix` option.", isNoise: true},

		// Generic CI noise
		{name: "success checkmark", line: "✓ Build succeeded", isNoise: true},
		{name: "downloading", line: "Downloading dependencies...", isNoise: true},
		{name: "github action", line: "::group::Setup Node.js", isNoise: true},
		{name: "stack trace at", line: "    at Object.<anonymous> (/path/to/file.js:10:5)", isNoise: true},
		{name: "indented content", line: "        deeply indented code", isNoise: true},

		// Real errors (should NOT be noise)
		{name: "go error", line: "main.go:10:5: undefined: foo", isNoise: false},
		{name: "ts error", line: "src/app.ts(5,10): error TS2322: Type 'string' is not assignable to type 'number'.", isNoise: false},
		{name: "eslint error stylish", line: "  8:11  error  Missing semicolon  semi", isNoise: false},
		{name: "rust error header", line: "error[E0308]: mismatched types", isNoise: false},
		{name: "go test fail", line: "--- FAIL: TestFoo (0.00s)", isNoise: false},

		// Edge cases
		{name: "ansi colored noise", line: "\x1b[32m=== RUN   TestFoo\x1b[0m", isNoise: true},
		{name: "ansi colored error", line: "\x1b[31mmain.go:10:5: undefined: foo\x1b[0m", isNoise: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.IsNoise(tt.line)
			if got != tt.isNoise {
				t.Errorf("IsNoise(%q) = %v, want %v", tt.line, got, tt.isNoise)
			}
		})
	}
}
