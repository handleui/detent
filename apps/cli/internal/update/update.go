package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/detent/cli/internal/util"
	"github.com/detent/cli/internal/persistence"
)

const (
	defaultManifestURL = "https://detent.sh/api/binaries/releases/manifest.json"
	installScript      = "https://detent.sh/install.sh"
	cacheFile          = "update-cache.json"
	cacheDuration      = 24 * time.Hour
	httpTimeout        = 5 * time.Second

	// maxResponseSize limits manifest response to prevent memory exhaustion
	maxResponseSize = 64 * 1024 // 64KB should be plenty for a version manifest
)

// manifestURL is the URL to fetch the manifest from.
// It can be overridden in tests to use a mock server.
var manifestURL = defaultManifestURL

type manifest struct {
	Latest   string   `json:"latest"`
	Versions []string `json:"versions"`
}

type cache struct {
	LastCheck     time.Time `json:"lastCheck"`
	LatestVersion string    `json:"latestVersion"`
}

func getCachePath() (string, error) {
	dir, err := persistence.GetDetentDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheFile), nil
}

func loadCache() *cache {
	path, err := getCachePath()
	if err != nil {
		return nil
	}

	// #nosec G304 - path is derived from user's home directory
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil
	}

	var c cache
	if unmarshalErr := json.Unmarshal(data, &c); unmarshalErr != nil {
		return nil
	}

	return &c
}

func saveCache(c *cache) {
	path, err := getCachePath()
	if err != nil {
		return
	}

	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0o700); mkdirErr != nil {
		return
	}

	data, marshalErr := json.Marshal(c)
	if marshalErr != nil {
		return
	}

	_ = os.WriteFile(path, data, 0o600)
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: httpTimeout}

	resp, err := client.Get(manifestURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Limit response size to prevent memory exhaustion from malicious/broken servers
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)

	var m manifest
	if decodeErr := json.NewDecoder(limitedReader).Decode(&m); decodeErr != nil {
		return "", decodeErr
	}

	// Validate that the version is valid semver before returning
	if m.Latest == "" {
		return "", errors.New("manifest contains empty latest version")
	}
	latest := strings.TrimPrefix(m.Latest, "v")
	if _, parseErr := semver.NewVersion(latest); parseErr != nil {
		return "", fmt.Errorf("invalid version in manifest: %w", parseErr)
	}

	return m.Latest, nil
}

// fetchLatestVersionWithRetry wraps fetchLatestVersion with retry logic.
func fetchLatestVersionWithRetry() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var result string
	err := util.Retry(ctx, func(_ context.Context) error {
		var fetchErr error
		result, fetchErr = fetchLatestVersion()
		return fetchErr
	},
		util.WithMaxAttempts(3),
		util.WithInitialDelay(500*time.Millisecond),
		util.WithMaxDelay(5*time.Second),
		util.WithBackoffMultiplier(2.0),
		util.WithJitterFactor(0.2),
	)

	return result, err
}

// Check returns the latest version and whether an update is available.
// Uses a 24h cache to avoid repeated network calls. Silent on errors.
func Check(currentVersion string) (latestVersion string, hasUpdate bool) {
	if currentVersion == "" || currentVersion == "dev" {
		return "", false
	}

	c := loadCache()

	if c != nil && time.Since(c.LastCheck) < cacheDuration {
		return compareVersions(currentVersion, c.LatestVersion)
	}

	latest, err := fetchLatestVersionWithRetry()
	if err != nil {
		if c != nil {
			return compareVersions(currentVersion, c.LatestVersion)
		}
		return "", false
	}

	saveCache(&cache{
		LastCheck:     time.Now(),
		LatestVersion: latest,
	})

	return compareVersions(currentVersion, latest)
}

func compareVersions(current, latest string) (string, bool) {
	if latest == "" {
		return "", false
	}

	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	currentSemver, err := semver.NewVersion(current)
	if err != nil {
		return "", false
	}

	latestSemver, err := semver.NewVersion(latest)
	if err != nil {
		return "", false
	}

	if latestSemver.GreaterThan(currentSemver) {
		return "v" + latest, true
	}

	return "", false
}

// Run executes the install script to update to the latest version.
func Run() error {
	// #nosec G204 - installScript is a hardcoded constant, not user input
	cmd := exec.Command("bash", "-c", "set -o pipefail; curl -fsSL "+installScript+" | bash")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// ClearCache removes the update cache file.
// Returns nil if the file doesn't exist.
func ClearCache() error {
	path, err := getCachePath()
	if err != nil {
		return err
	}
	if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}
	return nil
}
