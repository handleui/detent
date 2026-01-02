package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	handler := NewHandler("1.0.0", nil)

	t.Run("returns 200 with valid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
		w := httptest.NewRecorder()

		handler.HandleHealth(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Status != "ok" {
			t.Errorf("expected status 'ok', got %q", resp.Status)
		}

		if resp.Version != "1.0.0" {
			t.Errorf("expected version '1.0.0', got %q", resp.Version)
		}
	})

	t.Run("reports correct parser count", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
		w := httptest.NewRecorder()

		handler.HandleHealth(w, req)

		var resp HealthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Should have at least the default parsers (Go, TS, Python, ESLint, Rust, Generic)
		if resp.Parsers < 5 {
			t.Errorf("expected at least 5 parsers, got %d", resp.Parsers)
		}
	})
}

func TestParseEndpoint_ValidInput(t *testing.T) {
	handler := NewHandler("1.0.0", nil)

	t.Run("Go compile error", func(t *testing.T) {
		logs := `main.go:10:5: undefined: foo
main.go:15:10: cannot convert x (type int) to type string`

		body, _ := json.Marshal(ParseRequest{Logs: logs})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Errors) != 2 {
			t.Errorf("expected 2 errors, got %d", len(resp.Errors))
		}

		// Check first error
		if resp.Errors[0].File != "main.go" {
			t.Errorf("expected file 'main.go', got %q", resp.Errors[0].File)
		}
		if resp.Errors[0].Line != 10 {
			t.Errorf("expected line 10, got %d", resp.Errors[0].Line)
		}
		if resp.Errors[0].Source != "go" {
			t.Errorf("expected source 'go', got %q", resp.Errors[0].Source)
		}
	})

	t.Run("TypeScript error", func(t *testing.T) {
		logs := `src/index.ts(5,10): error TS2304: Cannot find name 'foo'.`

		body, _ := json.Marshal(ParseRequest{Logs: logs})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Errors) == 0 {
			t.Fatal("expected at least 1 error")
		}

		found := false
		for _, e := range resp.Errors {
			if e.Source == "typescript" && strings.Contains(e.File, "index.ts") {
				found = true
				if e.RuleID != "TS2304" {
					t.Errorf("expected rule_id 'TS2304', got %q", e.RuleID)
				}
			}
		}
		if !found {
			t.Error("expected to find TypeScript error")
		}
	})

	t.Run("ESLint error with rule ID (unix format)", func(t *testing.T) {
		// ESLint unix format: file:line:col: message [severity/rule-id]
		logs := `src/file.js:10:5: Unexpected var, use let or const instead [error/no-var]`

		body, _ := json.Marshal(ParseRequest{Logs: logs})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		found := false
		for _, e := range resp.Errors {
			if e.RuleID == "no-var" {
				found = true
				if e.Source != "eslint" {
					t.Errorf("expected source 'eslint', got %q", e.Source)
				}
				if e.Line != 10 {
					t.Errorf("expected line 10, got %d", e.Line)
				}
				if e.Column != 5 {
					t.Errorf("expected column 5, got %d", e.Column)
				}
			}
		}
		if !found {
			t.Error("expected to find ESLint error with rule 'no-var'")
		}
	})

	t.Run("multiple errors returns correct stats", func(t *testing.T) {
		logs := `main.go:1:1: error one
main.go:2:1: error two
main.go:3:1: error three`

		body, _ := json.Marshal(ParseRequest{Logs: logs})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Stats.Total != len(resp.Errors) {
			t.Errorf("stats.total (%d) should match errors length (%d)", resp.Stats.Total, len(resp.Errors))
		}
	})
}

func TestParseEndpoint_EdgeCases(t *testing.T) {
	handler := NewHandler("1.0.0", nil)

	t.Run("empty logs returns empty errors", func(t *testing.T) {
		body, _ := json.Marshal(ParseRequest{Logs: "   \n\n   "})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Errors) != 0 {
			t.Errorf("expected 0 errors, got %d", len(resp.Errors))
		}

		if resp.Stats.Total != 0 || resp.Stats.Errors != 0 || resp.Stats.Warnings != 0 {
			t.Errorf("expected all stats to be 0, got total=%d, errors=%d, warnings=%d",
				resp.Stats.Total, resp.Stats.Errors, resp.Stats.Warnings)
		}
	})

	t.Run("missing logs field returns 400", func(t *testing.T) {
		body := []byte(`{"context": {"job": "build"}}`)
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Error != "logs field is required" {
			t.Errorf("expected error 'logs field is required', got %q", resp.Error)
		}
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {
		body := []byte(`{not valid json}`)
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Error != "invalid JSON" {
			t.Errorf("expected error 'invalid JSON', got %q", resp.Error)
		}
	})

	t.Run("large input handles gracefully", func(t *testing.T) {
		// Generate 1MB of log-like content
		var sb strings.Builder
		line := "main.go:1:1: this is a test error message\n"
		for sb.Len() < 1024*1024 {
			sb.WriteString(line)
		}

		body, _ := json.Marshal(ParseRequest{Logs: sb.String()})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		// Should succeed (200) even with large input
		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Should have extracted errors
		if len(resp.Errors) == 0 {
			t.Error("expected to extract errors from large input")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/parse", http.NoBody)
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status 405, got %d", w.Code)
		}

		// Verify Allow header is set per RFC 7231
		allow := w.Header().Get("Allow")
		if allow != http.MethodPost {
			t.Errorf("expected Allow header 'POST', got %q", allow)
		}
	})

	t.Run("invalid Content-Type returns 415", func(t *testing.T) {
		body, _ := json.Marshal(ParseRequest{Logs: "main.go:1:1: error"})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("expected status 415, got %d", w.Code)
		}

		var resp ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Error != "Content-Type must be application/json" {
			t.Errorf("expected error about Content-Type, got %q", resp.Error)
		}
	})

	t.Run("missing Content-Type is allowed", func(t *testing.T) {
		body, _ := json.Marshal(ParseRequest{Logs: "main.go:1:1: error"})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("Content-Type with charset is allowed", func(t *testing.T) {
		body, _ := json.Marshal(ParseRequest{Logs: "main.go:1:1: error"})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("response has correct Content-Type", func(t *testing.T) {
		body, _ := json.Marshal(ParseRequest{Logs: "main.go:1:1: error"})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", contentType)
		}
	})
}

func TestParseEndpoint_Context(t *testing.T) {
	handler := NewHandler("1.0.0", nil)

	t.Run("context passed applies to errors", func(t *testing.T) {
		logs := `main.go:10:5: undefined: foo`

		body, _ := json.Marshal(ParseRequest{
			Logs: logs,
			Context: &ParseContext{
				Job:  "build",
				Step: "compile",
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Errors) == 0 {
			t.Fatal("expected at least 1 error")
		}

		// Check that workflow context was applied
		for _, e := range resp.Errors {
			if e.WorkflowContext == nil {
				t.Error("expected workflow context to be set")
				continue
			}
			if e.WorkflowContext.Job != "build" {
				t.Errorf("expected job 'build', got %q", e.WorkflowContext.Job)
			}
			if e.WorkflowContext.Step != "compile" {
				t.Errorf("expected step 'compile', got %q", e.WorkflowContext.Step)
			}
		}
	})

	t.Run("no context works without error", func(t *testing.T) {
		logs := `main.go:10:5: undefined: foo`

		body, _ := json.Marshal(ParseRequest{Logs: logs})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("basePath makes paths relative", func(t *testing.T) {
		logs := `/workspace/src/main.go:10:5: undefined: foo`

		body, _ := json.Marshal(ParseRequest{
			Logs: logs,
			Context: &ParseContext{
				BasePath: "/workspace",
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/parse", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.HandleParse(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp ParseResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp.Errors) == 0 {
			t.Fatal("expected at least 1 error")
		}

		// Check that path was made relative
		for _, e := range resp.Errors {
			if strings.HasPrefix(e.File, "/workspace") {
				t.Errorf("expected relative path, got %q", e.File)
			}
		}
	})
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	t.Run("adds security headers", func(t *testing.T) {
		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := SecurityHeadersMiddleware(innerHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		headers := map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"X-XSS-Protection":       "1; mode=block",
			"Cache-Control":          "no-store",
		}

		for header, expected := range headers {
			if got := w.Header().Get(header); got != expected {
				t.Errorf("expected %s header %q, got %q", header, expected, got)
			}
		}
	})
}

func TestLoggingMiddleware(t *testing.T) {
	handler := NewHandler("1.0.0", nil)

	t.Run("logs requests and captures status", func(t *testing.T) {
		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		wrapped := handler.LoggingMiddleware(innerHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d", w.Code)
		}
	})

	t.Run("preserves default status when WriteHeader not called", func(t *testing.T) {
		innerHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		})

		wrapped := handler.LoggingMiddleware(innerHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})
}

func TestMakeRelative(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		basePath string
		want     string
	}{
		{"empty basePath", "/foo/bar.go", "", "/foo/bar.go"},
		{"matching prefix", "/workspace/src/main.go", "/workspace", "src/main.go"},
		{"non-matching prefix", "/other/path/main.go", "/workspace", "/other/path/main.go"},
		{"exact match", "/workspace", "/workspace", "/workspace"},
		{"trailing slash base", "/workspace/src/main.go", "/workspace/", "src/main.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeRelative(tt.path, tt.basePath)
			if got != tt.want {
				t.Errorf("makeRelative(%q, %q) = %q, want %q", tt.path, tt.basePath, got, tt.want)
			}
		})
	}
}
