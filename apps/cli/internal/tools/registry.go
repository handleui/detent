package tools

import (
	"sort"
	"sync"

	"github.com/detent/cli/internal/tools/generic"
	"github.com/detent/cli/internal/tools/golang"
	"github.com/detent/cli/internal/tools/parser"
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

	// Future parsers to be added:
	// r.Register(eslint.NewParser())     // Priority ~85
	// r.Register(python.NewParser())     // Priority ~85
	// r.Register(rust.NewParser())       // Priority ~85
	// r.Register(nodejs.NewParser())     // Priority ~80

	// Priority 10: Generic fallback parser (last resort, flags unknown patterns for Sentry)
	r.Register(generic.NewParser())

	return r
}
