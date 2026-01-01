package golang

import (
	"regexp"
)

// Go-specific regex patterns for error extraction.
// Patterns use lazy quantifiers (.+?) where possible to prevent ReDoS.
var (
	// goErrorPattern matches Go compiler and linter errors: file.go:123:45: message
	// Group 1: file path (must end in .go)
	// Group 2: line number
	// Group 3: column number
	// Group 4: message (trimmed)
	goErrorPattern = regexp.MustCompile(`^([^\s:]+\.go):(\d+):(\d+):\s*(.+?)\s*$`)

	// goErrorNoColPattern matches Go compiler errors without column: file.go:123: message
	// Some errors (like import cycle) don't include column numbers
	// Group 1: file path
	// Group 2: line number
	// Group 3: message
	goErrorNoColPattern = regexp.MustCompile(`^([^\s:]+\.go):(\d+):\s*(.+?)\s*$`)

	// goTestFailPattern matches test failure markers: --- FAIL: TestName (0.00s)
	// Group 1: test name
	goTestFailPattern = regexp.MustCompile(`^---\s+FAIL:\s+(\S+)`)

	// goPanicPattern matches the start of a panic: panic: message
	goPanicPattern = regexp.MustCompile(`^panic:\s*(.+)$`)

	// goGoroutinePattern matches goroutine headers in stack traces
	// Example: "goroutine 1 [running]:"
	goGoroutinePattern = regexp.MustCompile(`^goroutine\s+\d+\s+\[`)

	// goStackFramePattern matches stack frame lines in Go stack traces
	// Matches function calls: "main.foo(0x0)" or "main.(*Type).Method(...)" or "created by main.foo"
	// Matches file locations: "        /path/to/file.go:10 +0x25"
	// Also matches "created by" lines that precede goroutine creation points
	goStackFramePattern = regexp.MustCompile(`^\S+\([^)]*\)\s*$|^\s+\S+\.go:\d+|^created by\s+`)

	// goStackFilePattern extracts file and line from stack trace file lines
	// Group 1: file path
	// Group 2: line number
	goStackFilePattern = regexp.MustCompile(`^\s+(\S+\.go):(\d+)`)

	// goBuildConstraintPattern matches build constraint errors
	// Example: "// +build linux" or "//go:build linux"
	goBuildConstraintPattern = regexp.MustCompile(`(?i)build constraints? exclude|no (?:buildable )?go (?:source )?files`)

	// goImportCyclePattern matches import cycle errors
	// Example: "import cycle not allowed"
	goImportCyclePattern = regexp.MustCompile(`import cycle not allowed`)

	// goModuleErrorPattern matches go.mod related errors
	// Examples:
	//   - "go: example.com/pkg@v1.0.0: invalid version"
	//   - "go: module example.com/pkg: not a module"
	//   - "go.mod:3: invalid go version"
	goModuleErrorPattern = regexp.MustCompile(`^go(?:\.mod)?(?::\d+)?:\s*(.+)$`)


	// golangciLintRulePattern extracts the rule name from golangci-lint output
	// Example: "ineffectual assignment to err (ineffassign)"
	// Example: "SA4006: this value of `lastFile` is never used (staticcheck)"
	// Group 1: message (everything before the rule)
	// Group 2: rule name in parentheses
	golangciLintRulePattern = regexp.MustCompile(`^(.+?)\s+\((\w+)\)\s*$`)

	// golangciLintCodePattern extracts static analysis codes from various linters
	// Staticcheck: SA4006, SA1000, SA5000, etc. (SA = static analysis)
	// Simple: S1000, S1001, etc. (code simplifications)
	// Stylecheck: ST1000, ST1001, etc. (style issues)
	// Quickfix: QF1000, QF1001, etc. (quick fixes)
	// Errcheck/gocritic/gosec: G101, G102, etc. (security)
	// Example: "SA4006: this value of `lastFile` is never used"
	// Example: "G101: Potential hardcoded credentials"
	// Group 1: code (e.g., "SA4006", "G101", "ST1000")
	// Group 2: message after the code
	golangciLintCodePattern = regexp.MustCompile(`^([A-Z]+\d+):\s*(.+)$`)

	// testOutputPattern matches indented test output (continuation of test failure)
	// Go test output is typically indented with tabs or spaces
	testOutputPattern = regexp.MustCompile(`^\s{4,}`)

	// testFileLinePattern matches test output file:line references
	// Example: "    file_test.go:25: expected 1, got 2"
	// Group 1: file path
	// Group 2: line number
	// Group 3: message
	testFileLinePattern = regexp.MustCompile(`^\s+([^\s:]+\.go):(\d+):\s*(.+)$`)

	// noisePatterns are lines that should be skipped as noise
	noisePatterns = []*regexp.Regexp{
		regexp.MustCompile(`^=== RUN\s+`),      // Test start
		regexp.MustCompile(`^=== PAUSE\s+`),    // Test pause
		regexp.MustCompile(`^=== CONT\s+`),     // Test continue
		regexp.MustCompile(`^=== NAME\s+`),     // Test name (Go 1.20+)
		regexp.MustCompile(`^--- PASS:`),       // Test pass
		regexp.MustCompile(`^--- SKIP:`),       // Test skip
		regexp.MustCompile(`^PASS$`),           // Overall pass
		regexp.MustCompile(`^ok\s+`),           // Package pass
		regexp.MustCompile(`^\?\s+`),           // No test files
		regexp.MustCompile(`^FAIL\s+\S+\s+\d`), // Package fail summary (FAIL package 0.123s)
		regexp.MustCompile(`^#\s+`),            // go build package header (# package/path)
		regexp.MustCompile(`^go:\s+`),          // go tool messages (go: downloading, go: finding)
		regexp.MustCompile(`^level=`),          // golangci-lint debug output
		regexp.MustCompile(`^Running\s+`),      // golangci-lint progress
		regexp.MustCompile(`^Issues:\s*\d+`),   // golangci-lint summary
		regexp.MustCompile(`^coverage:`),       // go test coverage output
		regexp.MustCompile(`^\s+---\s+PASS:`),  // Subtest pass
	}

	// KnownLinters maps linter names to their default severity level.
	// Based on golangci-lint linter configuration:
	// https://golangci-lint.run/usage/linters/
	KnownLinters = map[string]string{
		// Error-level linters (bugs, security issues, correctness)
		"gosec":             "error",
		"staticcheck":       "error",
		"govet":             "error",
		"errcheck":          "error",
		"ineffassign":       "error",
		"typecheck":         "error",
		"bodyclose":         "error",
		"nilerr":            "error",
		"nilnil":            "error",
		"sqlclosecheck":     "error",
		"rowserrcheck":      "error",
		"makezero":          "error",
		"durationcheck":     "error",
		"exportloopref":     "error",
		"noctx":             "error",
		"exhaustive":        "error",
		"asasalint":         "error",
		"bidichk":           "error",
		"contextcheck":      "error",
		"errchkjson":        "error",
		"execinquery":       "error",
		"gomoddirectives":   "error",
		"goprintffuncname":  "error",
		"musttag":           "error",
		"nosprintfhostport": "error",
		"reassign":          "error",
		"vet":               "error", // Alias for govet
		"unused":            "error", // Unused code is often a bug
		"deadcode":          "error", // Dead code (deprecated, merged into unused)
		"structcheck":       "error", // Struct field check (deprecated)
		"varcheck":          "error", // Variable check (deprecated)
		"copyloopvar":       "error", // Loop variable copy issues (Go 1.22+)
		"intrange":          "error", // Integer range issues
		"zerologlint":       "error", // Zerolog linter
		"spancheck":         "error", // OpenTelemetry span check
		"protogetter":       "error", // Protobuf getter check
		"perfsprint":        "error", // Performance sprint issues
		"nilnesserr":        "error", // nil + error check (govet)
		"fatcontext":        "error", // Context.WithValue issues
		"sloglint":          "error", // slog linter
		"recvcheck":         "error", // Receiver check

		// Warning-level linters (style, complexity, suggestions)
		"gocritic":          "warning",
		"gocyclo":           "warning",
		"gocognit":          "warning",
		"funlen":            "warning",
		"lll":               "warning",
		"nestif":            "warning",
		"godox":             "warning",
		"gofmt":             "warning",
		"goimports":         "warning",
		"misspell":          "warning",
		"whitespace":        "warning",
		"wsl":               "warning",
		"nlreturn":          "warning",
		"dogsled":           "warning",
		"dupl":              "warning",
		"golint":            "warning", // Deprecated, use revive
		"stylecheck":        "warning",
		"unconvert":         "warning",
		"unparam":           "warning",
		"nakedret":          "warning",
		"prealloc":          "warning",
		"goconst":           "warning",
		"gomnd":             "warning", // Deprecated, use mnd
		"mnd":               "warning", // Magic number detector
		"revive":            "warning",
		"forbidigo":         "warning",
		"depguard":          "warning",
		"godot":             "warning",
		"err113":            "warning", // Formerly goerr113
		"goerr113":          "warning", // Deprecated alias for err113
		"wrapcheck":         "warning",
		"errorlint":         "warning",
		"forcetypeassert":   "warning",
		"ifshort":           "warning", // Deprecated
		"varnamelen":        "warning",
		"ireturn":           "warning",
		"exhaustruct":       "warning",
		"nonamedreturns":    "warning",
		"maintidx":          "warning",
		"cyclop":            "warning",
		"gochecknoglobals":  "warning",
		"gochecknoinits":    "warning",
		"testpackage":       "warning",
		"paralleltest":      "warning",
		"tparallel":         "warning",
		"thelper":           "warning",
		"containedctx":      "warning",
		"usestdlibvars":     "warning",
		"loggercheck":       "warning", // Alias: logrlint
		"logrlint":          "warning", // Deprecated alias for loggercheck
		"decorder":          "warning",
		"errname":           "warning",
		"grouper":           "warning",
		"importas":          "warning", //nolint:misspell // importas is a real linter name
		"interfacebloat":    "warning",
		"nolintlint":        "warning",
		"nosnakecase":       "warning", // Deprecated
		"predeclared":       "warning",
		"promlinter":        "warning",
		"tagliatelle":       "warning",
		"tenv":              "warning",
		"testableexamples":  "warning",
		"wastedassign":      "warning",
		// Additional linters
		"ascicheck":         "warning", // ASCII identifier check (typo variant)
		"asciicheck":        "warning", // ASCII identifier check
		"canonicalheader":   "warning", // HTTP header canonicalization
		"dupword":           "warning", // Duplicate word check
		"gci":               "warning", // Go import ordering
		"ginkgolinter":      "warning", // Ginkgo test linter
		"gocheckcompilerdirectives": "warning",
		"gochecksumtype":    "warning", // Sum type exhaustiveness
		"goheader":          "warning", // File header check
		"gomodguard":        "warning", // Module guard
		"gosimple":          "warning", // Merged into staticcheck
		"gosmopolitan":      "warning", // i18n checks
		"inamedparam":       "warning", // Interface named params
		"interfacer":        "warning", // Deprecated
		"mirror":            "warning", // Mirror linter
		"nargs":             "warning", // Number of arguments
		"tagalign":          "warning", // Struct tag alignment
		"testifylint":       "warning", // Testify linter
	}

	// CodePrefixSeverity maps staticcheck/gosec code prefixes to severity.
	// SA = staticcheck (static analysis bugs)
	// S = simple (code simplification suggestions)
	// ST = stylecheck (style issues)
	// QF = quickfix (automated fixes available)
	// G = gosec (security issues)
	CodePrefixSeverity = map[string]string{
		"SA": "error",   // Static analysis bugs are errors
		"S":  "warning", // Simplification suggestions are warnings
		"ST": "warning", // Style issues are warnings
		"QF": "warning", // Quickfix suggestions are warnings
		"G":  "error",   // Security issues are errors
	}
)
