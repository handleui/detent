package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/handleui/detent/packages/core/tools/eslint"
	"github.com/handleui/detent/packages/core/tools/generic"
	"github.com/handleui/detent/packages/core/tools/golang"
	"github.com/handleui/detent/packages/core/tools/parser"
	"github.com/handleui/detent/packages/core/tools/rust"
	"github.com/handleui/detent/packages/core/tools/typescript"
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
	byExt   map[string]ToolParser // Extension-based index for fast path
	mu      sync.RWMutex          // Protects concurrent access

	// Optimized noise detection - consolidated from all parsers
	noiseChecker *noiseChecker
}

// NewRegistry creates a new parser registry
func NewRegistry() *Registry {
	return &Registry{
		parsers: make([]ToolParser, 0),
		byID:    make(map[string]ToolParser),
		byExt:   make(map[string]ToolParser),
	}
}

// extensionToParserID maps file extensions to parser IDs for fast path lookup.
// This enables O(1) parser selection when a line contains a file path with a known extension.
var extensionToParserID = map[string]string{
	".go":   "go",
	".ts":   "typescript",
	".tsx":  "typescript",
	".js":   "eslint", // JS errors more likely from linter than tsc
	".jsx":  "eslint",
	".mjs":  "eslint",
	".cjs":  "eslint",
	".rs":   "rust",
	".toml": "rust", // Cargo.toml errors
}

// Register adds a parser to the registry.
// Parsers are automatically sorted by priority (highest first).
// Also populates the extension-based index for fast path lookup.
func (r *Registry) Register(p ToolParser) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.parsers = append(r.parsers, p)
	r.byID[p.ID()] = p

	// Populate extension index for this parser
	for ext, parserID := range extensionToParserID {
		if parserID == p.ID() {
			r.byExt[ext] = p
		}
	}

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
// Otherwise, tries extension-based lookup first, then falls back to
// priority-ordered confidence scoring.
func (r *Registry) FindParser(line string, ctx *ParseContext) ToolParser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Fast path 1: if tool is known from step context, use that parser directly
	if ctx != nil && ctx.Tool != "" {
		if p, ok := r.byID[ctx.Tool]; ok {
			return p
		}
	}

	// Fast path 2: try extension-based lookup for lines with file paths
	if ext := extractFileExtension(line); ext != "" {
		if p, ok := r.byExt[ext]; ok {
			// Verify the parser actually wants this line (non-zero confidence)
			if p.CanParse(line, ctx) > 0 {
				return p
			}
		}
	}

	// Slow path: find parser with highest confidence score
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

// extractFileExtension extracts a file extension from a line containing a file path.
// Returns the extension (e.g., ".go", ".ts") or empty string if none found.
// Looks for common error format patterns: file.ext:line:col, file.ext(line,col), etc.
func extractFileExtension(line string) string {
	// Common patterns for file paths in error messages:
	// - path/file.go:10:5: message
	// - path/file.ts(10,5): message
	// - path/file.rs:10: message
	// - /absolute/path/file.go:10:5: message

	// Find potential file path by looking for extension followed by : or (
	// This is a simple heuristic that avoids regex for performance
	for i := 0; i < len(line); i++ {
		if line[i] == '.' {
			// Found a dot, extract potential extension
			extEnd := i + 1
			for extEnd < len(line) && isExtChar(line[extEnd]) {
				extEnd++
			}
			// Check if followed by : or ( (common in error formats)
			if extEnd < len(line) && (line[extEnd] == ':' || line[extEnd] == '(') {
				ext := strings.ToLower(line[i:extEnd])
				if _, ok := extensionToParserID[ext]; ok {
					return ext
				}
			}
		}
	}
	return ""
}

// isExtChar returns true if c is a valid file extension character
func isExtChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// IsNoise checks if the line is noise using optimized consolidated patterns.
// Returns true if the line matches any noise pattern from any registered parser.
// Performance: O(1) prefix checks + O(patterns) regex checks vs O(parsers * patterns).
func (r *Registry) IsNoise(line string) bool {
	r.mu.RLock()
	checker := r.noiseChecker
	r.mu.RUnlock()

	if checker == nil {
		return false
	}

	return checker.IsNoise(line)
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

	// Initialize optimized noise checker by collecting patterns from all registered parsers.
	// This must be called AFTER all parsers are registered to collect their patterns.
	r.noiseChecker = newNoiseCheckerFromParsers(r.parsers)

	return r
}

// ToolPattern represents a tool detection pattern with metadata.
type ToolPattern struct {
	Pattern     *regexp.Regexp
	ParserID    string
	DisplayName string // Human-readable name for error messages
}

// toolPatterns maps regex patterns to parser IDs for tool detection.
// Only patterns for tools with implemented parsers are included.
// Add new patterns when implementing new parsers.
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

	// ESLint
	{regexp.MustCompile(`(?:^|\s|/)eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)npx\s+eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)bunx?\s+eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)pnpm\s+.*eslint\b`), "eslint", "eslint"},
	{regexp.MustCompile(`(?:^|\s)yarn\s+.*eslint\b`), "eslint", "eslint"},

	// Rust tools
	{regexp.MustCompile(`(?:^|\s)cargo\s+(test|build|check|clippy|run|fmt)\b`), "rust", "cargo"},
	{regexp.MustCompile(`(?:^|\s|/)rustc\b`), "rust", "rustc"},
	{regexp.MustCompile(`(?:^|\s|/)clippy-driver\b`), "rust", "clippy"},
	{regexp.MustCompile(`(?:^|\s|/)rustfmt\b`), "rust", "rustfmt"},
}

// DetectedTool represents a tool detected from a run command.
type DetectedTool struct {
	ID          string // Parser ID (e.g., "go", "typescript", "pytest")
	DisplayName string // Human-readable name (e.g., "golangci-lint", "tsc", "pytest")
	Supported   bool   // Whether we have a dedicated parser for this tool
}

// DetectionOptions configures tool detection behavior.
type DetectionOptions struct {
	// FirstOnly returns only the first detected tool (default: false, returns all)
	FirstOnly bool
	// CheckSupport populates the Supported field in results (default: false)
	// Requires a Registry instance to check against
	CheckSupport bool
}

// DetectionResult contains the results of tool detection.
type DetectionResult struct {
	// Tools contains all detected tools (or just the first if FirstOnly was set)
	Tools []DetectedTool
}

// First returns the first detected tool, or an empty DetectedTool if none found.
func (r DetectionResult) First() DetectedTool {
	if len(r.Tools) == 0 {
		return DetectedTool{}
	}
	return r.Tools[0]
}

// FirstID returns the ID of the first detected tool, or empty string if none found.
func (r DetectionResult) FirstID() string {
	if len(r.Tools) == 0 {
		return ""
	}
	return r.Tools[0].ID
}

// HasTools returns true if any tools were detected.
func (r DetectionResult) HasTools() bool {
	return len(r.Tools) > 0
}

// Unsupported returns only the unsupported tools from the result.
func (r DetectionResult) Unsupported() []DetectedTool {
	var result []DetectedTool
	for _, t := range r.Tools {
		if !t.Supported {
			result = append(result, t)
		}
	}
	return result
}

// AllSupported returns true if all detected tools are supported (or if no tools detected).
func (r DetectionResult) AllSupported() bool {
	for _, t := range r.Tools {
		if !t.Supported {
			return false
		}
	}
	return true
}

// DetectTools analyzes a run command and returns detected tools with configurable options.
// This is the primary tool detection method that consolidates all detection functionality.
//
// Usage examples:
//
//	// Detect all tools with support check
//	result := registry.DetectTools(run, DetectionOptions{CheckSupport: true})
//	for _, tool := range result.Tools { ... }
//
//	// Detect first tool only
//	result := registry.DetectTools(run, DetectionOptions{FirstOnly: true})
//	toolID := result.FirstID()
//
//	// Check for unsupported tools
//	result := registry.DetectTools(run, DetectionOptions{CheckSupport: true})
//	unsupported := result.Unsupported()
func (r *Registry) DetectTools(run string, opts DetectionOptions) DetectionResult {
	return detectToolsInternal(run, opts, r)
}

// detectToolsInternal is the core detection logic that can work with or without a registry.
func detectToolsInternal(run string, opts DetectionOptions, registry *Registry) DetectionResult {
	seen := make(map[string]bool)
	var detectedTools []DetectedTool

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

						tool := DetectedTool{
							ID:          tp.ParserID,
							DisplayName: tp.DisplayName,
							Supported:   false,
						}

						// Check support if registry is available and option is set
						if opts.CheckSupport && registry != nil {
							tool.Supported = registry.HasDedicatedParser(tp.ParserID)
						}

						detectedTools = append(detectedTools, tool)

						// Early return if only first tool is needed
						if opts.FirstOnly {
							return DetectionResult{Tools: detectedTools}
						}
					}
					break // Only match first pattern per command segment
				}
			}
		}
	}

	return DetectionResult{Tools: detectedTools}
}

// DetectToolFromRun analyzes a run command and returns the detected tool ID.
// Returns empty string if no known tool is detected.
//
// Deprecated: Use Registry.DetectTools with DetectionOptions{FirstOnly: true} instead.
func DetectToolFromRun(run string) string {
	result := detectToolsInternal(run, DetectionOptions{FirstOnly: true}, nil)
	return result.FirstID()
}

// DetectAllToolsFromRun analyzes a run command and returns all detected tools.
// This is useful for workflow validation to report all unsupported tools at once.
//
// Deprecated: Use Registry.DetectTools instead for full functionality including support checks.
func DetectAllToolsFromRun(run string) []DetectedTool {
	result := detectToolsInternal(run, DetectionOptions{}, nil)
	return result.Tools
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
//
// Deprecated: Use Registry.DetectTools with DetectionOptions{FirstOnly: true, CheckSupport: true} instead.
func (r *Registry) DetectAndCheckSupport(run string) (string, bool) {
	result := r.DetectTools(run, DetectionOptions{FirstOnly: true, CheckSupport: true})
	if !result.HasTools() {
		return "", true // No tool detected, assume OK
	}
	first := result.First()
	return first.ID, first.Supported
}

// DetectAllAndCheckSupport detects all tools from a run command and checks their support status.
// Returns a slice of DetectedTool with the Supported field populated.
//
// Deprecated: Use Registry.DetectTools with DetectionOptions{CheckSupport: true} instead.
func (r *Registry) DetectAllAndCheckSupport(run string) []DetectedTool {
	result := r.DetectTools(run, DetectionOptions{CheckSupport: true})
	return result.Tools
}

// UnsupportedToolInfo contains information about an unsupported tool for error reporting.
type UnsupportedToolInfo struct {
	ToolID      string
	DisplayName string
	StepName    string
	JobID       string
}

// UnsupportedToolsReporter is a callback for reporting unsupported tools.
// This allows CLI to inject telemetry (e.g., Sentry) without core depending on it.
// The callback receives a summary of tool names (e.g., ["go (x2)", "pytest"]).
type UnsupportedToolsReporter func(toolSummary []string)

// DefaultUnsupportedToolsReporter is the default reporter (no-op).
// Can be overridden by CLI to inject Sentry or other telemetry.
var DefaultUnsupportedToolsReporter UnsupportedToolsReporter

// ReportUnsupportedTools sends information about unsupported tools for monitoring.
// This helps prioritize which tool parsers to implement next based on actual usage.
func ReportUnsupportedTools(tools []UnsupportedToolInfo) {
	if len(tools) == 0 || DefaultUnsupportedToolsReporter == nil {
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

	DefaultUnsupportedToolsReporter(toolNames)
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

// noiseChecker provides optimized noise detection using consolidated patterns.
// Instead of calling IsNoise on every parser for every line, this consolidates
// all noise patterns and applies fast checks before expensive regex operations.
//
// Performance optimization:
//   - Fast prefix checks for common noise patterns (O(1) string operations)
//   - Consolidated regex patterns checked once instead of per-parser
//   - ANSI escape code stripping done once per line via parser.StripANSI
//
// The patterns are collected from all registered parsers that implement
// the NoisePatternProvider interface, eliminating the need for duplicate
// hardcoded patterns in the registry.
type noiseChecker struct {
	// fastPrefixes are common prefixes that indicate noise (checked first)
	fastPrefixes []string

	// fastContains are common substrings that indicate noise
	fastContains []string

	// regexPatterns are consolidated regex patterns from all parsers
	regexPatterns []*regexp.Regexp
}

// newNoiseCheckerFromParsers creates an optimized noise checker by collecting
// patterns from all registered parsers that implement NoisePatternProvider.
// This eliminates the need for duplicate hardcoded patterns in the registry.
func newNoiseCheckerFromParsers(parsers []ToolParser) *noiseChecker {
	nc := &noiseChecker{}

	// Use maps to deduplicate patterns
	prefixSet := make(map[string]struct{})
	containsSet := make(map[string]struct{})
	regexSet := make(map[string]*regexp.Regexp)

	// Collect patterns from all parsers that implement NoisePatternProvider
	for _, p := range parsers {
		if provider, ok := p.(parser.NoisePatternProvider); ok {
			patterns := provider.NoisePatterns()

			// Collect fast prefixes (lowercase for case-insensitive matching)
			for _, prefix := range patterns.FastPrefixes {
				lower := strings.ToLower(prefix)
				if _, exists := prefixSet[lower]; !exists {
					prefixSet[lower] = struct{}{}
					nc.fastPrefixes = append(nc.fastPrefixes, lower)
				}
			}

			// Collect fast contains (lowercase for case-insensitive matching)
			for _, contains := range patterns.FastContains {
				lower := strings.ToLower(contains)
				if _, exists := containsSet[lower]; !exists {
					containsSet[lower] = struct{}{}
					nc.fastContains = append(nc.fastContains, lower)
				}
			}

			// Collect regex patterns (deduplicate by string representation)
			for _, re := range patterns.Regex {
				patternStr := re.String()
				if _, exists := regexSet[patternStr]; !exists {
					regexSet[patternStr] = re
					nc.regexPatterns = append(nc.regexPatterns, re)
				}
			}
		}
	}

	return nc
}

// IsNoise checks if the line is noise using optimized consolidated checks.
func (nc *noiseChecker) IsNoise(line string) bool {
	// Strip ANSI codes once for all checks using canonical implementation
	stripped := parser.StripANSI(line)

	// Fast path: empty or whitespace-only lines are noise
	trimmed := strings.TrimSpace(stripped)
	if trimmed == "" {
		return true
	}

	// Fast prefix checks (case-insensitive)
	lowerTrimmed := strings.ToLower(trimmed)
	for _, prefix := range nc.fastPrefixes {
		if strings.HasPrefix(lowerTrimmed, prefix) {
			return true
		}
	}

	// Fast substring checks (case-insensitive)
	lowerStripped := strings.ToLower(stripped)
	for _, substr := range nc.fastContains {
		if strings.Contains(lowerStripped, substr) {
			return true
		}
	}

	// Regex patterns (most expensive, checked last)
	for _, pattern := range nc.regexPatterns {
		if pattern.MatchString(stripped) {
			return true
		}
	}

	return false
}
