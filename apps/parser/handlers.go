package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/detentsh/core/ci"
	"github.com/detentsh/core/errors"
	"github.com/detentsh/core/extract"
	"github.com/detentsh/core/tools"
)

const (
	// SECURITY: Maximum request body size to prevent memory exhaustion DoS.
	// 10MB is sufficient for typical CI logs while preventing abuse.
	maxBodySize = 10 * 1024 * 1024

	// SECURITY: Maximum log string length within the JSON to prevent memory exhaustion.
	// This is separate from body size since JSON encoding overhead exists.
	maxLogsLength = 8 * 1024 * 1024
)

// ParseRequest is the request body for POST /parse.
type ParseRequest struct {
	Logs    string        `json:"logs"`
	Context *ParseContext `json:"context,omitempty"`
}

// ParseContext provides optional CI context for the parsing operation.
type ParseContext struct {
	Job      string `json:"job,omitempty"`
	Step     string `json:"step,omitempty"`
	BasePath string `json:"basePath,omitempty"`
}

// ParseResponse is the response body for POST /parse.
type ParseResponse struct {
	Errors []*errors.ExtractedError `json:"errors"`
	Stats  Stats                    `json:"stats"`
}

// Stats provides summary statistics for the parse result.
type Stats struct {
	Total    int `json:"total"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// HealthResponse is the response body for GET /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Parsers int    `json:"parsers"`
	Version string `json:"version"`
}

// ErrorResponse is the response body for error cases.
type ErrorResponse struct {
	Error string `json:"error"`
}

// passthroughParser is a ci.ContextParser that passes lines through unchanged.
// Used when parsing raw logs without CI-specific prefixes.
// This is a stateless singleton to avoid per-request allocations.
type passthroughParser struct{}

func (p *passthroughParser) ParseLine(line string) (*ci.LineContext, string, bool) {
	return nil, line, false
}

// sharedPassthroughParser is a singleton instance reused across all requests.
var sharedPassthroughParser = &passthroughParser{}

// Handler holds shared state for HTTP handlers.
type Handler struct {
	registry *tools.Registry
	version  string
	logger   *slog.Logger
}

// NewHandler creates a new Handler with the default parser registry.
func NewHandler(version string, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		registry: tools.DefaultRegistry(),
		version:  version,
		logger:   logger,
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// SecurityHeadersMiddleware adds security headers to all responses.
// SECURITY: These headers protect against common web vulnerabilities.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SECURITY: Prevent MIME type sniffing attacks.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// SECURITY: Prevent clickjacking attacks.
		w.Header().Set("X-Frame-Options", "DENY")
		// SECURITY: Enable XSS filter in older browsers.
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		// SECURITY: Disable caching for API responses to prevent sensitive data leakage.
		w.Header().Set("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware returns middleware that logs request details.
func (h *Handler) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		h.logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.status),
			slog.Duration("duration", time.Since(start)),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)
	})
}

// HandleHealth handles GET /health requests.
func (h *Handler) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	resp := HealthResponse{
		Status:  "ok",
		Parsers: len(h.registry.Parsers()),
		Version: h.version,
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleParse handles POST /parse requests.
func (h *Handler) HandleParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "method not allowed"})
		return
	}

	// SECURITY: Validate Content-Type to prevent content-type confusion attacks.
	// Accept missing Content-Type for curl convenience, but reject non-JSON types.
	// Use HasPrefix to handle charset suffixes like "application/json; charset=utf-8".
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, ErrorResponse{Error: "Content-Type must be application/json"})
		return
	}

	defer func() { _ = r.Body.Close() }()

	// SECURITY: Limit body size to prevent memory exhaustion DoS.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "failed to read request body"})
		return
	}

	// SECURITY: Check if the body was truncated (hit the limit).
	if len(body) == maxBodySize {
		writeJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{Error: "request body too large"})
		return
	}

	var req ParseRequest
	// SECURITY: Standard json.Unmarshal is safe - Go's encoding/json has built-in
	// protections against deeply nested structures (stack limit) and doesn't allow
	// duplicate keys to cause issues. No additional depth limiting needed.
	if err := json.Unmarshal(body, &req); err != nil {
		// SECURITY: Don't expose parsing details that could reveal internal structure.
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON"})
		return
	}

	if req.Logs == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "logs field is required"})
		return
	}

	// SECURITY: Validate logs length to prevent memory exhaustion from large strings.
	if len(req.Logs) > maxLogsLength {
		writeJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{Error: "logs field too large"})
		return
	}

	// Create a fresh extractor for this request (stateless)
	extractor := extract.NewExtractor(h.registry)

	// Use shared passthrough parser for raw logs (avoids per-request allocation)
	extracted := extractor.Extract(req.Logs, sharedPassthroughParser)

	// Apply severity inference
	errors.ApplySeverity(extracted)

	// Apply base path if provided
	if req.Context != nil && req.Context.BasePath != "" {
		for _, e := range extracted {
			if e.File != "" {
				e.File = makeRelative(e.File, req.Context.BasePath)
			}
		}
	}

	// Apply workflow context if provided
	if req.Context != nil && (req.Context.Job != "" || req.Context.Step != "") {
		for _, e := range extracted {
			if e.WorkflowContext == nil {
				e.WorkflowContext = &errors.WorkflowContext{
					Job:  req.Context.Job,
					Step: req.Context.Step,
				}
			}
		}
	}

	// Calculate stats
	stats := Stats{Total: len(extracted)}
	for _, e := range extracted {
		switch e.Severity {
		case "error":
			stats.Errors++
		case "warning":
			stats.Warnings++
		}
	}

	resp := ParseResponse{
		Errors: extracted,
		Stats:  stats,
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// makeRelative converts an absolute path to relative if it's under basePath.
func makeRelative(path, basePath string) string {
	if basePath == "" {
		return path
	}
	// Simple prefix stripping for the HTTP API
	if len(path) > len(basePath) && path[:len(basePath)] == basePath {
		rel := path[len(basePath):]
		if rel != "" && (rel[0] == '/' || rel[0] == '\\') {
			rel = rel[1:]
		}
		return rel
	}
	return path
}
