package actbin

import (
	"runtime"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
	if !strings.HasPrefix(Version, "0.") {
		t.Errorf("Version should start with '0.', got %q", Version)
	}
}

func TestDetentDir(t *testing.T) {
	dir, err := DetentDir()
	if err != nil {
		t.Fatalf("DetentDir() error: %v", err)
	}
	if !strings.HasSuffix(dir, ".detent") {
		t.Errorf("DetentDir() = %q, want suffix '.detent'", dir)
	}
}

func TestBinDir(t *testing.T) {
	dir, err := BinDir()
	if err != nil {
		t.Fatalf("BinDir() error: %v", err)
	}
	if !strings.Contains(dir, ".detent") {
		t.Errorf("BinDir() = %q, want to contain '.detent'", dir)
	}
	if !strings.HasSuffix(dir, "bin") {
		t.Errorf("BinDir() = %q, want suffix 'bin'", dir)
	}
}

func TestActPath(t *testing.T) {
	path, err := ActPath()
	if err != nil {
		t.Fatalf("ActPath() error: %v", err)
	}
	if !strings.Contains(path, Version) {
		t.Errorf("ActPath() = %q, want to contain version %q", path, Version)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(path, ".exe") {
		t.Errorf("ActPath() on Windows should end with .exe, got %q", path)
	}
}

func TestDownloadURL(t *testing.T) {
	url, err := DownloadURL()
	if err != nil {
		t.Fatalf("DownloadURL() error: %v", err)
	}

	if !strings.Contains(url, Version) {
		t.Errorf("DownloadURL() = %q, want to contain version %q", url, Version)
	}
	if !strings.HasPrefix(url, "https://github.com/nektos/act/releases/download/") {
		t.Errorf("DownloadURL() = %q, want GitHub releases prefix", url)
	}

	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(url, "Darwin") {
			t.Errorf("DownloadURL() on macOS should contain 'Darwin', got %q", url)
		}
	case "linux":
		if !strings.Contains(url, "Linux") {
			t.Errorf("DownloadURL() on Linux should contain 'Linux', got %q", url)
		}
	case "windows":
		if !strings.Contains(url, "Windows") {
			t.Errorf("DownloadURL() on Windows should contain 'Windows', got %q", url)
		}
		if !strings.HasSuffix(url, ".zip") {
			t.Errorf("DownloadURL() on Windows should end with .zip, got %q", url)
		}
	}
}

func TestPlatformAssetName(t *testing.T) {
	name, err := platformAssetName()
	if err != nil {
		t.Fatalf("platformAssetName() error: %v", err)
	}

	if !strings.HasPrefix(name, "act_") {
		t.Errorf("platformAssetName() = %q, want prefix 'act_'", name)
	}

	switch runtime.GOOS {
	case "windows":
		if !strings.HasSuffix(name, ".zip") {
			t.Errorf("platformAssetName() on Windows should end with .zip, got %q", name)
		}
	default:
		if !strings.HasSuffix(name, ".tar.gz") {
			t.Errorf("platformAssetName() should end with .tar.gz, got %q", name)
		}
	}
}

func TestIsInstalled(t *testing.T) {
	installed := IsInstalled()
	t.Logf("IsInstalled() = %v (expected to be false if act not downloaded yet)", installed)
}
