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

// GetFileInfo retrieves file metadata (size, mod time)
func GetFileInfo(path string) (size, modTime int64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat file: %w", err)
	}

	return info.Size(), info.ModTime().Unix(), nil
}

// BuildScannedFile creates a ScannedFile record with hash and metadata
func BuildScannedFile(path string, errorCount, warningCount int) (*ScannedFile, error) {
	hash, info, err := ComputeFileHashWithInfo(path)
	if err != nil {
		return nil, err
	}

	return &ScannedFile{
		Path:         path,
		Hash:         hash,
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		ErrorCount:   errorCount,
		WarningCount: warningCount,
	}, nil
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

// ComputeCodebaseStateHash creates a deterministic hash representing the exact state
// of a codebase. This enables deduplication across cloud agents and runs.
//
// For clean repos: just the commit SHA
// For dirty repos: commit SHA + sorted dirty file paths with their content hashes
//
// dirtyFileHashes is a map of relative file paths to their SHA256 content hashes.
func ComputeCodebaseStateHash(commitSHA string, dirtyFileHashes map[string]string) string {
	if len(dirtyFileHashes) == 0 {
		// Clean state: just hash the commit SHA for consistency
		hash := sha256.Sum256([]byte(commitSHA))
		return hex.EncodeToString(hash[:])
	}

	// Sort file paths for deterministic ordering
	paths := make([]string, 0, len(dirtyFileHashes))
	for path := range dirtyFileHashes {
		paths = append(paths, path)
	}
	sortStrings(paths)

	// Build deterministic string: commit::path1:hash1::path2:hash2...
	var buf strings.Builder
	buf.WriteString(commitSHA)
	for _, path := range paths {
		buf.WriteString("::")
		buf.WriteString(path)
		buf.WriteString(":")
		buf.WriteString(dirtyFileHashes[path])
	}

	hash := sha256.Sum256([]byte(buf.String()))
	return hex.EncodeToString(hash[:])
}

// sortStrings sorts a slice of strings in place (simple insertion sort for small slices)
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
