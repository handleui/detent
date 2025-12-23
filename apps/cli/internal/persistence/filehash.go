package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// ComputeFileHash calculates the SHA256 hash of a file
func ComputeFileHash(path string) (string, error) {
	file, err := os.Open(path) // #nosec G304 - path is from user's repository, expected behavior
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to compute hash: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
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
	hash, err := ComputeFileHash(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
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
