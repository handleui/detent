package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCompareVersions tests version comparison logic
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		latest        string
		wantLatest    string
		wantHasUpdate bool
	}{
		{
			name:          "newer version available",
			current:       "1.0.0",
			latest:        "1.1.0",
			wantLatest:    "v1.1.0",
			wantHasUpdate: true,
		},
		{
			name:          "major version update",
			current:       "1.9.9",
			latest:        "2.0.0",
			wantLatest:    "v2.0.0",
			wantHasUpdate: true,
		},
		{
			name:          "patch version update",
			current:       "1.0.0",
			latest:        "1.0.1",
			wantLatest:    "v1.0.1",
			wantHasUpdate: true,
		},
		{
			name:          "same version",
			current:       "1.0.0",
			latest:        "1.0.0",
			wantLatest:    "",
			wantHasUpdate: false,
		},
		{
			name:          "current is newer",
			current:       "2.0.0",
			latest:        "1.0.0",
			wantLatest:    "",
			wantHasUpdate: false,
		},
		{
			name:          "with v prefix on current",
			current:       "v1.0.0",
			latest:        "1.1.0",
			wantLatest:    "v1.1.0",
			wantHasUpdate: true,
		},
		{
			name:          "with v prefix on latest",
			current:       "1.0.0",
			latest:        "v1.1.0",
			wantLatest:    "v1.1.0",
			wantHasUpdate: true,
		},
		{
			name:          "with v prefix on both",
			current:       "v1.0.0",
			latest:        "v1.1.0",
			wantLatest:    "v1.1.0",
			wantHasUpdate: true,
		},
		{
			name:          "prerelease current vs stable latest",
			current:       "1.0.0-beta.1",
			latest:        "1.0.0",
			wantLatest:    "v1.0.0",
			wantHasUpdate: true,
		},
		{
			name:          "stable current vs prerelease latest",
			current:       "1.0.0",
			latest:        "1.1.0-beta.1",
			wantLatest:    "v1.1.0-beta.1",
			wantHasUpdate: true,
		},
		{
			name:          "prerelease vs newer prerelease",
			current:       "1.0.0-alpha.1",
			latest:        "1.0.0-beta.1",
			wantLatest:    "v1.0.0-beta.1",
			wantHasUpdate: true,
		},
		{
			name:          "empty latest returns no update",
			current:       "1.0.0",
			latest:        "",
			wantLatest:    "",
			wantHasUpdate: false,
		},
		{
			name:          "invalid current version",
			current:       "not-a-version",
			latest:        "1.0.0",
			wantLatest:    "",
			wantHasUpdate: false,
		},
		{
			name:          "invalid latest version",
			current:       "1.0.0",
			latest:        "not-a-version",
			wantLatest:    "",
			wantHasUpdate: false,
		},
		{
			name:          "both invalid",
			current:       "abc",
			latest:        "xyz",
			wantLatest:    "",
			wantHasUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLatest, gotHasUpdate := compareVersions(tt.current, tt.latest)
			if gotLatest != tt.wantLatest {
				t.Errorf("compareVersions(%q, %q) latestVersion = %q, want %q",
					tt.current, tt.latest, gotLatest, tt.wantLatest)
			}
			if gotHasUpdate != tt.wantHasUpdate {
				t.Errorf("compareVersions(%q, %q) hasUpdate = %v, want %v",
					tt.current, tt.latest, gotHasUpdate, tt.wantHasUpdate)
			}
		})
	}
}

// TestCheck_SpecialVersions tests Check behavior with special version strings
func TestCheck_SpecialVersions(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		wantLatest    string
		wantHasUpdate bool
	}{
		{
			name:          "empty version returns no update",
			current:       "",
			wantLatest:    "",
			wantHasUpdate: false,
		},
		{
			name:          "dev version returns no update",
			current:       "dev",
			wantLatest:    "",
			wantHasUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLatest, gotHasUpdate := Check(tt.current)
			if gotLatest != tt.wantLatest {
				t.Errorf("Check(%q) latestVersion = %q, want %q",
					tt.current, gotLatest, tt.wantLatest)
			}
			if gotHasUpdate != tt.wantHasUpdate {
				t.Errorf("Check(%q) hasUpdate = %v, want %v",
					tt.current, gotHasUpdate, tt.wantHasUpdate)
			}
		})
	}
}

// TestCheck_WithMockServer tests Check with mocked HTTP responses
func TestCheck_WithMockServer(t *testing.T) {
	tests := []struct {
		name           string
		responseCode   int
		responseBody   string
		currentVersion string
		wantLatest     string
		wantHasUpdate  bool
	}{
		{
			name:           "successful response with newer version",
			responseCode:   http.StatusOK,
			responseBody:   `{"latest": "2.0.0", "versions": ["2.0.0", "1.0.0"]}`,
			currentVersion: "1.0.0",
			wantLatest:     "v2.0.0",
			wantHasUpdate:  true,
		},
		{
			name:           "successful response with same version",
			responseCode:   http.StatusOK,
			responseBody:   `{"latest": "1.0.0", "versions": ["1.0.0"]}`,
			currentVersion: "1.0.0",
			wantLatest:     "",
			wantHasUpdate:  false,
		},
		{
			name:           "successful response with older version",
			responseCode:   http.StatusOK,
			responseBody:   `{"latest": "1.0.0", "versions": ["1.0.0"]}`,
			currentVersion: "2.0.0",
			wantLatest:     "",
			wantHasUpdate:  false,
		},
		{
			name:           "server error returns no update",
			responseCode:   http.StatusInternalServerError,
			responseBody:   `{"error": "server error"}`,
			currentVersion: "1.0.0",
			wantLatest:     "",
			wantHasUpdate:  false,
		},
		{
			name:           "not found error returns no update",
			responseCode:   http.StatusNotFound,
			responseBody:   `{"error": "not found"}`,
			currentVersion: "1.0.0",
			wantLatest:     "",
			wantHasUpdate:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv(DetentHomeEnv, tmpDir)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.responseCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			originalURL := manifestURL
			manifestURL = server.URL
			defer func() { manifestURL = originalURL }()

			gotLatest, gotHasUpdate := Check(tt.currentVersion)
			if gotLatest != tt.wantLatest {
				t.Errorf("Check(%q) latestVersion = %q, want %q",
					tt.currentVersion, gotLatest, tt.wantLatest)
			}
			if gotHasUpdate != tt.wantHasUpdate {
				t.Errorf("Check(%q) hasUpdate = %v, want %v",
					tt.currentVersion, gotHasUpdate, tt.wantHasUpdate)
			}
		})
	}
}

// TestCheck_MalformedResponses tests Check with malformed manifest responses
func TestCheck_MalformedResponses(t *testing.T) {
	tests := []struct {
		name         string
		responseBody string
	}{
		{
			name:         "invalid JSON",
			responseBody: `{"latest": "1.0.0"`,
		},
		{
			name:         "empty JSON object",
			responseBody: `{}`,
		},
		{
			name:         "empty latest field",
			responseBody: `{"latest": "", "versions": ["1.0.0"]}`,
		},
		{
			name:         "invalid semver in latest",
			responseBody: `{"latest": "not-semver", "versions": ["not-semver"]}`,
		},
		{
			name:         "null latest",
			responseBody: `{"latest": null}`,
		},
		{
			name:         "wrong type for latest",
			responseBody: `{"latest": 123}`,
		},
		{
			name:         "array instead of object",
			responseBody: `["1.0.0", "2.0.0"]`,
		},
		{
			name:         "completely empty response",
			responseBody: ``,
		},
		{
			name:         "truncated JSON",
			responseBody: `{"latest": "1.0.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv(DetentHomeEnv, tmpDir)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			originalURL := manifestURL
			manifestURL = server.URL
			defer func() { manifestURL = originalURL }()

			gotLatest, gotHasUpdate := Check("1.0.0")

			if gotHasUpdate {
				t.Errorf("Check() with malformed response should return hasUpdate=false, got true with latestVersion=%q", gotLatest)
			}
		})
	}
}

// TestCheck_CacheBehavior tests the caching mechanism
func TestCheck_CacheBehavior(t *testing.T) {
	t.Run("uses cached value within cache duration", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cacheData := cache{
			LastCheck:     time.Now(),
			LatestVersion: "2.0.0",
		}
		data, _ := json.Marshal(cacheData)
		cachePath := filepath.Join(tmpDir, cacheFile)
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatalf("Failed to write cache file: %v", err)
		}

		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requestCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"latest": "3.0.0"}`))
		}))
		defer server.Close()

		originalURL := manifestURL
		manifestURL = server.URL
		defer func() { manifestURL = originalURL }()

		gotLatest, gotHasUpdate := Check("1.0.0")

		if requestCount != 0 {
			t.Errorf("Expected 0 HTTP requests (cached), got %d", requestCount)
		}
		if gotLatest != "v2.0.0" {
			t.Errorf("Expected cached version v2.0.0, got %q", gotLatest)
		}
		if !gotHasUpdate {
			t.Error("Expected hasUpdate=true for cached version")
		}
	})

	t.Run("fetches new value when cache expired", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cacheData := cache{
			LastCheck:     time.Now().Add(-25 * time.Hour),
			LatestVersion: "2.0.0",
		}
		data, _ := json.Marshal(cacheData)
		cachePath := filepath.Join(tmpDir, cacheFile)
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatalf("Failed to write cache file: %v", err)
		}

		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requestCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"latest": "3.0.0"}`))
		}))
		defer server.Close()

		originalURL := manifestURL
		manifestURL = server.URL
		defer func() { manifestURL = originalURL }()

		gotLatest, gotHasUpdate := Check("1.0.0")

		if requestCount != 1 {
			t.Errorf("Expected 1 HTTP request (cache expired), got %d", requestCount)
		}
		if gotLatest != "v3.0.0" {
			t.Errorf("Expected fetched version v3.0.0, got %q", gotLatest)
		}
		if !gotHasUpdate {
			t.Error("Expected hasUpdate=true for fetched version")
		}
	})

	t.Run("fetches when no cache exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requestCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"latest": "2.0.0"}`))
		}))
		defer server.Close()

		originalURL := manifestURL
		manifestURL = server.URL
		defer func() { manifestURL = originalURL }()

		gotLatest, gotHasUpdate := Check("1.0.0")

		if requestCount != 1 {
			t.Errorf("Expected 1 HTTP request (no cache), got %d", requestCount)
		}
		if gotLatest != "v2.0.0" {
			t.Errorf("Expected fetched version v2.0.0, got %q", gotLatest)
		}
		if !gotHasUpdate {
			t.Error("Expected hasUpdate=true")
		}
	})

	t.Run("falls back to cache when fetch fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cacheData := cache{
			LastCheck:     time.Now().Add(-25 * time.Hour),
			LatestVersion: "2.0.0",
		}
		data, _ := json.Marshal(cacheData)
		cachePath := filepath.Join(tmpDir, cacheFile)
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatalf("Failed to write cache file: %v", err)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		originalURL := manifestURL
		manifestURL = server.URL
		defer func() { manifestURL = originalURL }()

		gotLatest, gotHasUpdate := Check("1.0.0")

		if gotLatest != "v2.0.0" {
			t.Errorf("Expected fallback to cached version v2.0.0, got %q", gotLatest)
		}
		if !gotHasUpdate {
			t.Error("Expected hasUpdate=true from cached version")
		}
	})

	t.Run("saves cache after successful fetch", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"latest": "2.0.0"}`))
		}))
		defer server.Close()

		originalURL := manifestURL
		manifestURL = server.URL
		defer func() { manifestURL = originalURL }()

		_, _ = Check("1.0.0")

		cachePath := filepath.Join(tmpDir, cacheFile)
		data, err := os.ReadFile(cachePath)
		if err != nil {
			t.Fatalf("Failed to read cache file: %v", err)
		}

		var savedCache cache
		if err := json.Unmarshal(data, &savedCache); err != nil {
			t.Fatalf("Failed to unmarshal cache: %v", err)
		}

		if savedCache.LatestVersion != "2.0.0" {
			t.Errorf("Cache LatestVersion = %q, want %q", savedCache.LatestVersion, "2.0.0")
		}
		if time.Since(savedCache.LastCheck) > time.Minute {
			t.Error("Cache LastCheck should be recent")
		}
	})
}

// TestCheck_CacheMalformed tests handling of malformed cache files
func TestCheck_CacheMalformed(t *testing.T) {
	tests := []struct {
		name         string
		cacheContent string
	}{
		{
			name:         "invalid JSON in cache",
			cacheContent: `{"lastCheck": "not-a-date"`,
		},
		{
			name:         "empty cache file",
			cacheContent: ``,
		},
		{
			name:         "null cache",
			cacheContent: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv(DetentHomeEnv, tmpDir)

			cachePath := filepath.Join(tmpDir, cacheFile)
			if err := os.WriteFile(cachePath, []byte(tt.cacheContent), 0o600); err != nil {
				t.Fatalf("Failed to write cache file: %v", err)
			}

			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestCount++
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"latest": "2.0.0"}`))
			}))
			defer server.Close()

			originalURL := manifestURL
			manifestURL = server.URL
			defer func() { manifestURL = originalURL }()

			gotLatest, gotHasUpdate := Check("1.0.0")

			if requestCount != 1 {
				t.Errorf("Expected 1 HTTP request (malformed cache), got %d", requestCount)
			}
			if gotLatest != "v2.0.0" {
				t.Errorf("Expected fetched version v2.0.0, got %q", gotLatest)
			}
			if !gotHasUpdate {
				t.Error("Expected hasUpdate=true")
			}
		})
	}
}

// TestClearCache tests the ClearCache function
func TestClearCache(t *testing.T) {
	t.Run("removes existing cache file", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cachePath := filepath.Join(tmpDir, cacheFile)
		if err := os.WriteFile(cachePath, []byte(`{"lastCheck": "2024-01-01T00:00:00Z"}`), 0o600); err != nil {
			t.Fatalf("Failed to write cache file: %v", err)
		}

		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			t.Fatal("Cache file should exist before test")
		}

		if err := ClearCache(); err != nil {
			t.Fatalf("ClearCache() error = %v", err)
		}

		if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
			t.Error("Cache file should be removed after ClearCache()")
		}
	})

	t.Run("returns nil when cache file does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		if err := ClearCache(); err != nil {
			t.Errorf("ClearCache() error = %v, want nil for non-existent file", err)
		}
	})

	t.Run("returns nil for empty detent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		if err := ClearCache(); err != nil {
			t.Errorf("ClearCache() error = %v, want nil", err)
		}
	})
}

// TestFetchLatestVersion tests the fetchLatestVersion function
func TestFetchLatestVersion(t *testing.T) {
	tests := []struct {
		name         string
		responseCode int
		responseBody string
		wantVersion  string
		wantErr      bool
	}{
		{
			name:         "successful fetch",
			responseCode: http.StatusOK,
			responseBody: `{"latest": "1.2.3", "versions": ["1.2.3"]}`,
			wantVersion:  "1.2.3",
			wantErr:      false,
		},
		{
			name:         "version with v prefix",
			responseCode: http.StatusOK,
			responseBody: `{"latest": "v1.2.3", "versions": ["v1.2.3"]}`,
			wantVersion:  "v1.2.3",
			wantErr:      false,
		},
		{
			name:         "server error",
			responseCode: http.StatusInternalServerError,
			responseBody: ``,
			wantVersion:  "",
			wantErr:      true,
		},
		{
			name:         "empty latest in manifest",
			responseCode: http.StatusOK,
			responseBody: `{"latest": ""}`,
			wantVersion:  "",
			wantErr:      true,
		},
		{
			name:         "invalid semver",
			responseCode: http.StatusOK,
			responseBody: `{"latest": "invalid"}`,
			wantVersion:  "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.responseCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			originalURL := manifestURL
			manifestURL = server.URL
			defer func() { manifestURL = originalURL }()

			gotVersion, err := fetchLatestVersion()

			if (err != nil) != tt.wantErr {
				t.Errorf("fetchLatestVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("fetchLatestVersion() = %q, want %q", gotVersion, tt.wantVersion)
			}
		})
	}
}

// TestFetchLatestVersion_Timeout tests that fetchLatestVersion respects timeout
func TestFetchLatestVersion_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"latest": "1.0.0"}`))
	}))
	defer server.Close()

	originalURL := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = originalURL }()

	start := time.Now()
	_, err := fetchLatestVersion()
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error")
	}

	// Should timeout around httpTimeout (5s), not wait the full 10s
	if duration > 7*time.Second {
		t.Errorf("Expected timeout around %v, but took %v", httpTimeout, duration)
	}
}

// TestLoadCache tests cache loading behavior
func TestLoadCache(t *testing.T) {
	t.Run("returns nil for non-existent cache", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		c := loadCache()
		if c != nil {
			t.Errorf("loadCache() = %v, want nil", c)
		}
	})

	t.Run("returns nil for invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		cachePath := filepath.Join(tmpDir, cacheFile)
		if err := os.WriteFile(cachePath, []byte(`invalid json`), 0o600); err != nil {
			t.Fatalf("Failed to write cache file: %v", err)
		}

		c := loadCache()
		if c != nil {
			t.Errorf("loadCache() = %v, want nil for invalid JSON", c)
		}
	})

	t.Run("loads valid cache", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		now := time.Now().Truncate(time.Second)
		cacheData := cache{
			LastCheck:     now,
			LatestVersion: "1.2.3",
		}
		data, _ := json.Marshal(cacheData)
		cachePath := filepath.Join(tmpDir, cacheFile)
		if err := os.WriteFile(cachePath, data, 0o600); err != nil {
			t.Fatalf("Failed to write cache file: %v", err)
		}

		c := loadCache()
		if c == nil {
			t.Fatal("loadCache() = nil, want non-nil")
		}
		if c.LatestVersion != "1.2.3" {
			t.Errorf("loadCache().LatestVersion = %q, want %q", c.LatestVersion, "1.2.3")
		}
	})
}

// TestSaveCache tests cache saving behavior
func TestSaveCache(t *testing.T) {
	t.Run("creates cache file", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(DetentHomeEnv, tmpDir)

		c := &cache{
			LastCheck:     time.Now(),
			LatestVersion: "1.2.3",
		}

		saveCache(c)

		cachePath := filepath.Join(tmpDir, cacheFile)
		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			t.Error("Cache file should be created")
		}
	})

	t.Run("creates detent directory if needed", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "detent")
		t.Setenv(DetentHomeEnv, nestedDir)

		c := &cache{
			LastCheck:     time.Now(),
			LatestVersion: "1.2.3",
		}

		saveCache(c)

		cachePath := filepath.Join(nestedDir, cacheFile)
		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			t.Error("Cache file should be created in nested directory")
		}
	})
}

// TestConstants tests the package constants have expected values
func TestConstants(t *testing.T) {
	if cacheDuration != 24*time.Hour {
		t.Errorf("cacheDuration = %v, want 24h", cacheDuration)
	}

	if httpTimeout != 5*time.Second {
		t.Errorf("httpTimeout = %v, want 5s", httpTimeout)
	}

	if maxResponseSize != 64*1024 {
		t.Errorf("maxResponseSize = %d, want 65536 (64KB)", maxResponseSize)
	}

	if cacheFile != "update-cache.json" {
		t.Errorf("cacheFile = %q, want %q", cacheFile, "update-cache.json")
	}
}

// TestCheck_NetworkError tests Check behavior when network is unavailable
func TestCheck_NetworkError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(DetentHomeEnv, tmpDir)

	originalURL := manifestURL
	manifestURL = "http://localhost:1"
	defer func() { manifestURL = originalURL }()

	gotLatest, gotHasUpdate := Check("1.0.0")

	if gotLatest != "" {
		t.Errorf("Check() with network error latestVersion = %q, want empty", gotLatest)
	}
	if gotHasUpdate {
		t.Error("Check() with network error hasUpdate = true, want false")
	}
}

// TestCheck_LargeResponse tests that large responses are handled safely
func TestCheck_LargeResponse(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(DetentHomeEnv, tmpDir)

	// Create a response that's large but still within 64KB limit
	// Each version entry is about 10 bytes, so 1000 entries ~= 10KB
	largeResponse := `{"latest": "1.0.0", "versions": [`
	for i := 0; i < 1000; i++ {
		if i > 0 {
			largeResponse += ","
		}
		largeResponse += `"1.0.0"`
	}
	largeResponse += `]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeResponse))
	}))
	defer server.Close()

	originalURL := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = originalURL }()

	gotLatest, gotHasUpdate := Check("0.9.0")

	if gotLatest != "v1.0.0" {
		t.Errorf("Check() latestVersion = %q, want v1.0.0", gotLatest)
	}
	if !gotHasUpdate {
		t.Error("Check() hasUpdate = false, want true")
	}
}

// TestCheck_OversizedResponse tests that responses over 64KB are truncated
func TestCheck_OversizedResponse(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(DetentHomeEnv, tmpDir)

	// Create a response larger than maxResponseSize (64KB)
	// This should cause a JSON decode error due to truncation
	largeResponse := `{"latest": "1.0.0", "versions": [`
	for i := 0; i < 10000; i++ {
		if i > 0 {
			largeResponse += ","
		}
		largeResponse += `"1.0.0"`
	}
	largeResponse += `]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeResponse))
	}))
	defer server.Close()

	originalURL := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = originalURL }()

	// Should return no update because response is truncated and invalid
	gotLatest, gotHasUpdate := Check("0.9.0")

	if gotHasUpdate {
		t.Errorf("Check() with oversized response should return hasUpdate=false, got latestVersion=%q", gotLatest)
	}
}

// BenchmarkCheck benchmarks the Check function with cached data
func BenchmarkCheck_Cached(b *testing.B) {
	tmpDir := b.TempDir()
	b.Setenv(DetentHomeEnv, tmpDir)

	cacheData := cache{
		LastCheck:     time.Now(),
		LatestVersion: "2.0.0",
	}
	data, _ := json.Marshal(cacheData)
	cachePath := filepath.Join(tmpDir, cacheFile)
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
		b.Fatalf("Failed to write cache file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Check("1.0.0")
	}
}

// BenchmarkCompareVersions benchmarks version comparison
func BenchmarkCompareVersions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = compareVersions("1.2.3", "2.0.0")
	}
}
