package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// ComputeFileHash calculates the SHA256 hash of a file
func ComputeFileHash(path string) (string, error) {
	hash, _, err := ComputeFileHashWithInfo(path)
	return hash, err
}

// ComputeFileHashWithInfo calculates the SHA256 hash of a file and returns file info
func ComputeFileHashWithInfo(path string) (string, os.FileInfo, error) {
	file, err := os.Open(path) // #nosec G304 - path is from user's repository, expected behavior
	if err != nil {
		return "", nil, fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Get file info while file is open
	info, err := file.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("failed to stat file: %w", err)
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", nil, fmt.Errorf("failed to compute hash: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), info, nil
}

// lineColPattern matches file:line:col patterns for normalization
var lineColPattern = regexp.MustCompile(`:\d+:\d+`)

// ComputeContentHash creates a normalized hash of an error message for deduplication.
// Normalizes by lowercasing, trimming whitespace, and removing file:line:col patterns.
func ComputeContentHash(message string) string {
	// Normalize: lowercase, trim whitespace, remove line:col numbers
	normalized := strings.ToLower(strings.TrimSpace(message))
	normalized = lineColPattern.ReplaceAllString(normalized, "")

	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}
