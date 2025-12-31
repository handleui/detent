package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/detent/cli/internal/sentry"
	"github.com/detent/cli/internal/tools/eslint"
	"github.com/detent/cli/internal/tools/generic"
	"github.com/detent/cli/internal/tools/golang"
	"github.com/detent/cli/internal/tools/parser"
	"github.com/detent/cli/internal/tools/rust"
	"github.com/detent/cli/internal/tools/typescript"
)

type (
	// ToolParser is re-exported from parser package for convenience
	ToolParser = parser.ToolParser
	// ParseContext is re-exported from parser package for convenience
	ParseContext = parser.ParseContext
)

// Registry manages tool parsers and routes output to the appropriate parser.
// It maintains parsers in priority order and supports tool-aware selection.
type Registry struct {
	parsers []ToolParser          // Sorted by priority (descending)
	byID    map[string]ToolParser // Quick lookup by parser ID
	mu      sync.RWMutex          // Protects concurrent access
}

// NewRegistry creates a new parser registry
func NewRegistry() *Registry {
	return &Registry{
		parsers: make([]ToolParser, 0),
		byID:    make(map[string]ToolParser),
	}
}

// Register adds a parser to the registry.
// Parsers are automatically sorted by priority (highest first).
func (r *Registry) Register(p ToolParser) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.parsers = append(r.parsers, p)
	r.byID[p.ID()] = p

	// Sort by priority descending (highest priority first)
	sort.SliceStable(r.parsers, func(i, j int) bool {
		return r.parsers[i].Priority() > r.parsers[j].Priority()
	})
}

// Get returns a parser by ID, or nil if not found
func (r *Registry) Get(id string) ToolParser {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byID[id]
}

// FindParser returns the best matching parser for a line.
// If the context has a known tool, that parser is used directly.
// Otherwise, parsers are tried in priority order using confidence scoring.
func (r *Registry) FindParser(line string, ctx *ParseContext) ToolParser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Fast path: if tool is known from step context, use that parser directly
	if ctx != nil && ctx.Tool != "" {
		if p, ok := r.byID[ctx.Tool]; ok {
			return p
		}
	}

	// Find parser with highest confidence score
	var best ToolParser
	var bestScore float64

	for _, p := range r.parsers {
		score := p.CanParse(line, ctx)
		if score > bestScore {
			bestScore = score
			best = p
		}
	}

	return best
}

// IsNoise checks if any parser considers this line as noise.
// Returns true if any registered parser flags it as noise.
func (r *Registry) IsNoise(line string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.parsers {
		if p.IsNoise(line) {
			return true
		}
	}
	return false
}

// ResetAll resets the state of all registered parsers.
// Should be called between parsing different outputs.
func (r *Registry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.parsers {
		p.Reset()
	}
}

// Parsers returns a copy of all registered parsers in priority order
func (r *Registry) Parsers() []ToolParser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ToolParser, len(r.parsers))
	copy(result, r.parsers)
	return result
}

// DefaultRegistry returns a registry with all built-in parsers registered.
// Parsers are registered in priority order (most specific first).
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Register parsers in priority order (highest priority first)
	// Priority 90: Language-specific parsers with precise formats
	r.Register(golang.NewParser())
	r.Register(typescript.NewParser())

	// Priority 85: Tool-specific parsers (ESLint, Rust, linters)
	r.Register(eslint.NewParser())
	r.Register(rust.NewParser())

	// Future parsers to be added:
	// r.Register(python.NewParser())     // Priority ~85
	// r.Register(nodejs.NewParser())     // Priority ~80

	// Priority 10: Generic fallback parser (last resort, flags unknown patterns for Sentry)
	r.Register(generic.NewParser())

	return r
}

// ToolPattern represents a tool detection pattern with metadata.
type ToolPattern struct {
	Pattern     *regexp.Regexp
	ParserID    string
	DisplayName string // Human-readable name for error messages
}

// toolPatterns maps regex patterns to parser IDs for tool detection.
// Patterns are tried in order; more specific patterns should come first.
var toolPatterns = []ToolPattern{
	// Go tools
	{regexp.MustCompile(`(?:^|\s|/)golangci-lint\s`), "go", "golangci-lint"},
	{regexp.MustCompile(`(?:^|\s)go\s+(test|build|vet|run|install|mod|fmt|generate)\b`), "go", "go"},
	{regexp.MustCompile(`(?:^|\s)go\s+tool\s`), "go", "go tool"},
	{regexp.MustCompile(`(?:^|\s|/)staticcheck\b`), "go", "staticcheck"},
	{regexp.MustCompile(`(?:^|\s|/)govulncheck\b`), "go", "govulncheck"},

	// TypeScript/JavaScript type checking
	{regexp.MustCompile(`(?:^|\s|/)tsc\b`), "typescript", "tsc"},
	{regexp.MustCompile(`(?:^|\s)npx\s+tsc\b`), "typescript", "tsc"},
	{regexp.MustCompile(`(?:^|\s)bunx?\s+tsc\b`), "typescript", "tsc"},
	{regexp.MustCompile(`(?:^|\s)pnpm\s+.*tsc\b`), "typescript", "tsc"},
	{regexp.MustCompile(`(?:^|\s)yarn\s+.*tsc\b`), "typescript", "tsc"},

	// ESLint (supported)
	{regexp.MustCompile(`(?:^|\s|/)eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)npx\s+eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)bunx?\s+eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)pnpm\s+.*eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)yarn\s+.*eslint\b`), "eslint", "eslint"},

	// Biome (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)biome\s+(check|lint|format|ci)\b`), "biome", "biome"},
	{regexp.MustCompile(`(?:^|\s)npx\s+@biomejs/biome\b`), "biome", "biome"},
	{regexp.MustCompile(`(?:^|\s)bunx?\s+biome\b`), "biome", "biome"},

	// Prettier (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)prettier\b`), "prettier", "prettier"},
	{regexp.MustCompile(`(?:^|\s)npx\s+prettier\b`), "prettier", "prettier"},

	// Python tools (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)pytest\b`), "pytest", "pytest"},
	{regexp.MustCompile(`(?:^|\s)python\s+-m\s+pytest\b`), "pytest", "pytest"},
	{regexp.MustCompile(`(?:^|\s|/)mypy\b`), "mypy", "mypy"},
	{regexp.MustCompile(`(?:^|\s)python\s+-m\s+mypy\b`), "mypy", "mypy"},
	{regexp.MustCompile(`(?:^|\s|/)ruff\s+(check|format)?\b`), "ruff", "ruff"},
	{regexp.MustCompile(`(?:^|\s|/)pylint\b`), "pylint", "pylint"},
	{regexp.MustCompile(`(?:^|\s|/)flake8\b`), "flake8", "flake8"},
	{regexp.MustCompile(`(?:^|\s|/)black\b`), "black", "black"},
	{regexp.MustCompile(`(?:^|\s|/)pyright\b`), "pyright", "pyright"},
	{regexp.MustCompile(`(?:^|\s)python\s+-m\s+unittest\b`), "unittest", "unittest"},

	// Rust tools
	{regexp.MustCompile(`(?:^|\s)cargo\s+(test|build|check|clippy|run|fmt)\b`), "rust", "cargo"},
	{regexp.MustCompile(`(?:^|\s|/)rustc\b`), "rust", "rustc"},
	{regexp.MustCompile(`(?:^|\s|/)clippy-driver\b`), "rust", "clippy"},
	{regexp.MustCompile(`(?:^|\s|/)rustfmt\b`), "rust", "rustfmt"},

	// JavaScript test runners (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)jest\b`), "jest", "jest"},
	{regexp.MustCompile(`(?:^|\s)npx\s+jest\b`), "jest", "jest"},
	{regexp.MustCompile(`(?:^|\s|/)vitest\b`), "vitest", "vitest"},
	{regexp.MustCompile(`(?:^|\s)npx\s+vitest\b`), "vitest", "vitest"},
	{regexp.MustCompile(`(?:^|\s|/)mocha\b`), "mocha", "mocha"},
	{regexp.MustCompile(`(?:^|\s)npx\s+mocha\b`), "mocha", "mocha"},
	{regexp.MustCompile(`(?:^|\s|/)ava\b`), "ava", "ava"},
	{regexp.MustCompile(`(?:^|\s|/)playwright\s+test\b`), "playwright", "playwright"},
	{regexp.MustCompile(`(?:^|\s)npx\s+playwright\s+test\b`), "playwright", "playwright"},
	{regexp.MustCompile(`(?:^|\s|/)cypress\s+run\b`), "cypress", "cypress"},

	// Java/JVM tools (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)mvn\s`), "maven", "maven"},
	{regexp.MustCompile(`(?:^|\s|/)gradle\s`), "gradle", "gradle"},
	{regexp.MustCompile(`(?:^|\s|/)gradlew\s`), "gradle", "gradle"},
	{regexp.MustCompile(`(?:^|\s|/)javac\b`), "javac", "javac"},

	// Ruby tools (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)rubocop\b`), "rubocop", "rubocop"},
	{regexp.MustCompile(`(?:^|\s|/)rspec\b`), "rspec", "rspec"},
	{regexp.MustCompile(`(?:^|\s)bundle\s+exec\s+rspec\b`), "rspec", "rspec"},
	{regexp.MustCompile(`(?:^|\s|/)minitest\b`), "minitest", "minitest"},

	// PHP tools (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)phpunit\b`), "phpunit", "phpunit"},
	{regexp.MustCompile(`(?:^|\s|/)phpstan\b`), "phpstan", "phpstan"},
	{regexp.MustCompile(`(?:^|\s|/)psalm\b`), "psalm", "psalm"},

	// C/C++ tools (not yet supported)
	{regexp.MustCompile(`(?:^|\s|/)gcc\b`), "gcc", "gcc"},
	{regexp.MustCompile(`(?:^|\s|/)g\+\+\b`), "g++", "g++"},
	{regexp.MustCompile(`(?:^|\s|/)clang\b`), "clang", "clang"},
	{regexp.MustCompile(`(?:^|\s|/)clang\+\+\b`), "clang++", "clang++"},
	{regexp.MustCompile(`(?:^|\s)cmake\s`), "cmake", "cmake"},
	{regexp.MustCompile(`(?:^|\s)make\s`), "make", "make"},

	// .NET tools (not yet supported)
	{regexp.MustCompile(`(?:^|\s)dotnet\s+(build|test|run)\b`), "dotnet", "dotnet"},

	// Swift tools (not yet supported)
	{regexp.MustCompile(`(?:^|\s)swift\s+(build|test)\b`), "swift", "swift"},
	{regexp.MustCompile(`(?:^|\s)xcodebuild\s`), "xcodebuild", "xcodebuild"},

	// Terraform/Infrastructure (not yet supported)
	{regexp.MustCompile(`(?:^|\s)terraform\s+(plan|apply|validate)\b`), "terraform", "terraform"},
}

// DetectedTool represents a tool detected from a run command.
type DetectedTool struct {
	ID          string // Parser ID (e.g., "go", "typescript", "pytest")
	DisplayName string // Human-readable name (e.g., "golangci-lint", "tsc", "pytest")
	Supported   bool   // Whether we have a dedicated parser for this tool
}

// DetectToolFromRun analyzes a run command and returns the detected tool ID.
// Returns empty string if no known tool is detected.
// For detecting all tools in a command, use DetectAllToolsFromRun.
func DetectToolFromRun(run string) string {
	// Normalize: handle multi-line commands by checking each line
	lines := strings.Split(run, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Handle command chaining (&&, ||, ;)
		commands := splitCommands(line)
		for _, cmd := range commands {
			cmd = strings.TrimSpace(cmd)
			if cmd == "" {
				continue
			}
			for _, tp := range toolPatterns {
				if tp.Pattern.MatchString(cmd) {
					return tp.ParserID
				}
			}
		}
	}
	return ""
}

// DetectAllToolsFromRun analyzes a run command and returns all detected tools.
// This is useful for workflow validation to report all unsupported tools at once.
func DetectAllToolsFromRun(run string) []DetectedTool {
	seen := make(map[string]bool)
	var tools []DetectedTool

	// Normalize: handle multi-line commands by checking each line
	lines := strings.Split(run, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Handle command chaining (&&, ||, ;)
		commands := splitCommands(line)
		for _, cmd := range commands {
			cmd = strings.TrimSpace(cmd)
			if cmd == "" {
				continue
			}
			for _, tp := range toolPatterns {
				if tp.Pattern.MatchString(cmd) {
					// Avoid duplicates
					if !seen[tp.ParserID] {
						seen[tp.ParserID] = true
						tools = append(tools, DetectedTool{
							ID:          tp.ParserID,
							DisplayName: tp.DisplayName,
							Supported:   false, // Will be set by caller using registry
						})
					}
					break // Only match first pattern per command segment
				}
			}
		}
	}

	return tools
}

// splitCommands splits a command line by common shell operators.
// This handles cases like: "npm install && npm test" or "go build; go test"
func splitCommands(line string) []string {
	// Split by &&, ||, ; while preserving the command structure
	// We use a simple approach that handles most common cases
	var result []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(line); i++ {
		ch := line[i]

		// Handle quotes
		if (ch == '"' || ch == '\'') && (i == 0 || line[i-1] != '\\') {
			if !inQuote {
				inQuote = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuote = false
			}
			current.WriteByte(ch)
			continue
		}

		// Check for command separators only when not in quotes
		if !inQuote {
			// Check for && or ||
			if i < len(line)-1 {
				if (line[i] == '&' && line[i+1] == '&') || (line[i] == '|' && line[i+1] == '|') {
					if current.Len() > 0 {
						result = append(result, current.String())
						current.Reset()
					}
					i++ // Skip next character
					continue
				}
			}
			// Check for ;
			if ch == ';' {
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
				continue
			}
			// Check for | (pipe) - still a command separator
			if ch == '|' {
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
				continue
			}
		}

		current.WriteByte(ch)
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// HasDedicatedParser returns true if the registry has a non-generic parser for the given ID.
func (r *Registry) HasDedicatedParser(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.byID[id]
	if !ok {
		return false
	}
	// Generic parser is a fallback, not a "dedicated" parser
	return p.ID() != "generic"
}

// SupportedToolIDs returns a list of all registered parser IDs that have dedicated parsing.
// This excludes the generic fallback parser.
func (r *Registry) SupportedToolIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ids []string
	for id := range r.byID {
		if id != "generic" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// DetectAndCheckSupport detects a tool from a run command and checks if it's supported.
// Returns (toolID, isSupported). If no tool is detected, returns ("", true).
func (r *Registry) DetectAndCheckSupport(run string) (string, bool) {
	toolID := DetectToolFromRun(run)
	if toolID == "" {
		return "", true // No tool detected, assume OK
	}
	return toolID, r.HasDedicatedParser(toolID)
}

// DetectAllAndCheckSupport detects all tools from a run command and checks their support status.
// Returns a slice of DetectedTool with the Supported field populated.
func (r *Registry) DetectAllAndCheckSupport(run string) []DetectedTool {
	tools := DetectAllToolsFromRun(run)
	for i := range tools {
		tools[i].Supported = r.HasDedicatedParser(tools[i].ID)
	}
	return tools
}

// UnsupportedToolInfo contains information about an unsupported tool for error reporting.
type UnsupportedToolInfo struct {
	ToolID      string
	DisplayName string
	StepName    string
	JobID       string
}

// ReportUnsupportedTools sends information about unsupported tools to Sentry for monitoring.
// This helps prioritize which tool parsers to implement next based on actual usage.
func ReportUnsupportedTools(tools []UnsupportedToolInfo) {
	if len(tools) == 0 {
		return
	}

	// Group by tool ID to avoid duplicate reports
	toolCounts := make(map[string]int)
	for _, t := range tools {
		toolCounts[t.ToolID]++
	}

	// Create a summary message
	var toolNames []string
	for id, count := range toolCounts {
		if count > 1 {
			toolNames = append(toolNames, fmt.Sprintf("%s (x%d)", id, count))
		} else {
			toolNames = append(toolNames, id)
		}
	}
	sort.Strings(toolNames)

	// Add breadcrumb for context
	sentry.AddBreadcrumb("tools", fmt.Sprintf("detected %d unsupported tool(s)", len(tools)))

	// Set tag for filtering in Sentry dashboard
	sentry.SetTag("unsupported_tools", strings.Join(toolNames, ","))

	// Send message to Sentry (this is informational, not an error)
	sentry.CaptureMessage(fmt.Sprintf("Unsupported tools detected in workflow: %s", strings.Join(toolNames, ", ")))
}

// FormatUnsupportedToolsWarning creates a formatted warning message for unsupported tools.
func FormatUnsupportedToolsWarning(unsupportedTools []DetectedTool, supportedIDs []string) string {
	if len(unsupportedTools) == 0 {
		return ""
	}

	var toolNames []string
	for _, t := range unsupportedTools {
		toolNames = append(toolNames, t.DisplayName)
	}

	var sb strings.Builder
	if len(unsupportedTools) == 1 {
		sb.WriteString(fmt.Sprintf("Tool %q detected but not fully supported", toolNames[0]))
	} else {
		sb.WriteString(fmt.Sprintf("Tools %s detected but not fully supported", formatList(toolNames)))
	}
	sb.WriteString(". Errors will be captured but may not be fully structured.")

	if len(supportedIDs) > 0 {
		sb.WriteString(fmt.Sprintf(" Fully supported tools: %s.", strings.Join(supportedIDs, ", ")))
	}

	return sb.String()
}

// formatList formats a list of items as "a, b, and c" or "a and b".
func formatList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}
