package tools

import (
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

		// ESLint (supported)
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

		// Biome (not yet supported)
		{
			name:     "biome check",
			run:      "biome check .",
			expected: "biome",
		},
		{
			name:     "biome lint",
			run:      "biome lint src/",
			expected: "biome",
		},

		// Prettier (not yet supported)
		{
			name:     "prettier",
			run:      "prettier --check .",
			expected: "prettier",
		},

		// Python tools (not yet supported)
		{
			name:     "pytest",
			run:      "pytest tests/",
			expected: "pytest",
		},
		{
			name:     "python -m pytest",
			run:      "python -m pytest -v",
			expected: "pytest",
		},
		{
			name:     "mypy",
			run:      "mypy src/",
			expected: "mypy",
		},
		{
			name:     "python -m mypy",
			run:      "python -m mypy src/",
			expected: "mypy",
		},
		{
			name:     "ruff check",
			run:      "ruff check .",
			expected: "ruff",
		},
		{
			name:     "ruff format",
			run:      "ruff format --check .",
			expected: "ruff",
		},
		{
			name:     "black",
			run:      "black --check .",
			expected: "black",
		},
		{
			name:     "pyright",
			run:      "pyright src/",
			expected: "pyright",
		},

		// Rust tools (supported via rust parser)
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

		// JavaScript test runners (not yet supported)
		{
			name:     "jest",
			run:      "jest --coverage",
			expected: "jest",
		},
		{
			name:     "vitest",
			run:      "vitest run",
			expected: "vitest",
		},
		{
			name:     "playwright test",
			run:      "playwright test",
			expected: "playwright",
		},
		{
			name:     "cypress run",
			run:      "cypress run",
			expected: "cypress",
		},

		// Java tools (not yet supported)
		{
			name:     "maven",
			run:      "mvn test",
			expected: "maven",
		},
		{
			name:     "gradle",
			run:      "gradle build",
			expected: "gradle",
		},
		{
			name:     "gradlew",
			run:      "./gradlew test",
			expected: "gradle",
		},

		// Ruby tools (not yet supported)
		{
			name:     "rubocop",
			run:      "rubocop",
			expected: "rubocop",
		},
		{
			name:     "rspec",
			run:      "rspec spec/",
			expected: "rspec",
		},
		{
			name:     "bundle exec rspec",
			run:      "bundle exec rspec",
			expected: "rspec",
		},

		// C/C++ tools (not yet supported)
		{
			name:     "gcc",
			run:      "gcc -o main main.c",
			expected: "gcc",
		},
		{
			name:     "clang",
			run:      "clang -o main main.c",
			expected: "clang",
		},
		{
			name:     "cmake",
			run:      "cmake --build .",
			expected: "cmake",
		},

		// .NET tools (not yet supported)
		{
			name:     "dotnet build",
			run:      "dotnet build",
			expected: "dotnet",
		},
		{
			name:     "dotnet test",
			run:      "dotnet test",
			expected: "dotnet",
		},

		// Swift tools (not yet supported)
		{
			name:     "swift build",
			run:      "swift build",
			expected: "swift",
		},
		{
			name:     "xcodebuild",
			run:      "xcodebuild test",
			expected: "xcodebuild",
		},

		// Infrastructure tools (not yet supported)
		{
			name:     "terraform plan",
			run:      "terraform plan",
			expected: "terraform",
		},
		{
			name:     "terraform validate",
			run:      "terraform validate",
			expected: "terraform",
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
jest --coverage`,
			expectedIDs: []string{"eslint", "typescript", "jest"},
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
			name:     "pytest not registered",
			id:       "pytest",
			expected: false,
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
			name:          "unsupported pytest",
			run:           "pytest tests/",
			wantTool:      "pytest",
			wantSupported: false,
		},
		{
			name:          "unsupported biome",
			run:           "biome check .",
			wantTool:      "biome",
			wantSupported: false,
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
		name                  string
		run                   string
		wantToolCount         int
		wantUnsupportedCount  int
	}{
		{
			name:                  "single supported tool",
			run:                   "go test ./...",
			wantToolCount:         1,
			wantUnsupportedCount:  0,
		},
		{
			name:                  "single unsupported tool",
			run:                   "pytest tests/",
			wantToolCount:         1,
			wantUnsupportedCount:  1,
		},
		{
			name:                  "mixed supported and unsupported",
			run:                   "eslint src/ && pytest tests/",
			wantToolCount:         2,
			wantUnsupportedCount:  1, // pytest unsupported
		},
		{
			name:                  "multiple unsupported tools",
			run:                   "pytest tests/ && jest --coverage",
			wantToolCount:         2,
			wantUnsupportedCount:  2,
		},
		{
			name:                  "no tools",
			run:                   "echo hello",
			wantToolCount:         0,
			wantUnsupportedCount:  0,
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
				if !contains(got, want) {
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

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
