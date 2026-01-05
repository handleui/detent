package actbin

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DetentDir returns the path to the detent data directory (~/.detent).
func DetentDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".detent"), nil
}

// BinDir returns the path to the detent bin directory (~/.detent/bin).
func BinDir() (string, error) {
	detentDir, err := DetentDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(detentDir, "bin"), nil
}

// ActPath returns the path to the versioned act binary.
// The binary is stored as act-{version} to allow multiple versions to coexist.
func ActPath() (string, error) {
	binDir, err := BinDir()
	if err != nil {
		return "", err
	}
	binaryName := fmt.Sprintf("act-%s", Version)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return filepath.Join(binDir, binaryName), nil
}

// IsInstalled checks if the bundled act binary exists at the correct version.
func IsInstalled() bool {
	actPath, err := ActPath()
	if err != nil {
		return false
	}
	info, err := os.Stat(actPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// EnsureBinDir creates the bin directory if it doesn't exist.
func EnsureBinDir() error {
	binDir, err := BinDir()
	if err != nil {
		return err
	}
	//nolint:gosec // bin directory needs 755 for executable access
	return os.MkdirAll(binDir, 0o755)
}

// platformAssetName returns the GitHub release asset name for the current platform.
// Returns names like "act_Darwin_arm64.tar.gz" or "act_Windows_x86_64.zip".
func platformAssetName() (string, error) {
	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "Darwin"
	case "linux":
		osName = "Linux"
	case "windows":
		osName = "Windows"
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	var archName string
	switch runtime.GOARCH {
	case "amd64":
		archName = "x86_64"
	case "arm64":
		archName = "arm64"
	case "arm":
		archName = "armv7"
	case "386":
		archName = "i386"
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("act_%s_%s.%s", osName, archName, ext), nil
}

// DownloadURL returns the GitHub release download URL for the current platform.
func DownloadURL() (string, error) {
	assetName, err := platformAssetName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"https://github.com/nektos/act/releases/download/v%s/%s",
		Version,
		assetName,
	), nil
}
