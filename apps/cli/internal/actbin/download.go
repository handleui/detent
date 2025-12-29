package actbin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ProgressFunc is called during download with bytes downloaded and total bytes.
// Total may be -1 if content-length is not provided.
type ProgressFunc func(downloaded, total int64)

// ErrUnsupportedPlatform indicates the current OS/arch combination is not supported.
var ErrUnsupportedPlatform = errors.New("unsupported platform")

// ErrDownloadFailed indicates the download could not be completed.
var ErrDownloadFailed = errors.New("download failed")

// ErrExtractionFailed indicates the archive could not be extracted.
var ErrExtractionFailed = errors.New("extraction failed")

// maxDownloadSize is the maximum allowed download size (100 MB).
// The act binary is typically ~50 MB, so 100 MB provides headroom.
const maxDownloadSize = 100 * 1024 * 1024

// maxExtractedSize is the maximum allowed extracted binary size (200 MB).
// This prevents zip bomb attacks where a small archive expands to huge files.
const maxExtractedSize = 200 * 1024 * 1024

// EnsureInstalled checks if act is installed and downloads it if not.
// The onProgress callback is called periodically during download.
func EnsureInstalled(ctx context.Context, onProgress ProgressFunc) error {
	if IsInstalled() {
		return nil
	}
	return Download(ctx, onProgress)
}

// Download downloads and installs the act binary for the current platform.
func Download(ctx context.Context, onProgress ProgressFunc) error {
	url, urlErr := DownloadURL()
	if urlErr != nil {
		return fmt.Errorf("%w: %w", ErrUnsupportedPlatform, urlErr)
	}

	if binErr := EnsureBinDir(); binErr != nil {
		return fmt.Errorf("creating bin directory: %w", binErr)
	}

	actPath, pathErr := ActPath()
	if pathErr != nil {
		return pathErr
	}

	tempFile, tempErr := os.CreateTemp(filepath.Dir(actPath), "act-download-*")
	if tempErr != nil {
		return fmt.Errorf("creating temp file: %w", tempErr)
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()
	defer func() { _ = os.Remove(tempPath) }()

	if dlErr := downloadFile(ctx, url, tempPath, onProgress); dlErr != nil {
		return dlErr
	}

	if extErr := extractActBinary(tempPath, actPath); extErr != nil {
		return extErr
	}

	//nolint:gosec // executable needs 755 permissions
	if chmodErr := os.Chmod(actPath, 0o755); chmodErr != nil {
		return fmt.Errorf("setting executable permissions: %w", chmodErr)
	}

	return nil
}

// downloadFile downloads a URL to a local file with progress reporting.
func downloadFile(ctx context.Context, url, destPath string, onProgress ProgressFunc) error {
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if reqErr != nil {
		return fmt.Errorf("%w: creating request: %w", ErrDownloadFailed, reqErr)
	}

	resp, respErr := http.DefaultClient.Do(req)
	if respErr != nil {
		return fmt.Errorf("%w: %w", ErrDownloadFailed, respErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", ErrDownloadFailed, resp.StatusCode)
	}

	//nolint:gosec // destPath is from controlled internal path construction
	out, createErr := os.Create(destPath)
	if createErr != nil {
		return fmt.Errorf("%w: creating file: %w", ErrDownloadFailed, createErr)
	}
	defer func() { _ = out.Close() }()

	// Limit download size to prevent DoS from malicious server responses
	limitedBody := io.LimitReader(resp.Body, maxDownloadSize+1)

	var reader io.Reader
	if onProgress != nil {
		reader = &progressReader{
			reader:     limitedBody,
			total:      resp.ContentLength,
			onProgress: onProgress,
		}
	} else {
		reader = limitedBody
	}

	written, copyErr := io.Copy(out, reader)
	if copyErr != nil {
		return fmt.Errorf("%w: writing file: %w", ErrDownloadFailed, copyErr)
	}

	if written > maxDownloadSize {
		return fmt.Errorf("%w: download exceeds maximum size of %d bytes", ErrDownloadFailed, maxDownloadSize)
	}

	// Sync to disk to catch disk full errors before closing
	if syncErr := out.Sync(); syncErr != nil {
		return fmt.Errorf("%w: syncing file to disk: %w", ErrDownloadFailed, syncErr)
	}

	return nil
}

// progressReader wraps an io.Reader to report download progress.
type progressReader struct {
	reader     io.Reader
	downloaded int64
	total      int64
	onProgress ProgressFunc
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.onProgress != nil {
		pr.onProgress(pr.downloaded, pr.total)
	}
	return n, err
}

// extractActBinary extracts the act binary from a tar.gz or zip archive.
func extractActBinary(archivePath, destPath string) error {
	if runtime.GOOS == "windows" {
		return extractFromZip(archivePath, destPath)
	}
	return extractFromTarGz(archivePath, destPath)
}

// extractFromTarGz extracts the act binary from a tar.gz archive.
func extractFromTarGz(archivePath, destPath string) error {
	//nolint:gosec // archivePath is from controlled temp file path
	file, openErr := os.Open(archivePath)
	if openErr != nil {
		return fmt.Errorf("%w: opening archive: %w", ErrExtractionFailed, openErr)
	}
	defer func() { _ = file.Close() }()

	gzReader, gzErr := gzip.NewReader(file)
	if gzErr != nil {
		return fmt.Errorf("%w: creating gzip reader: %w", ErrExtractionFailed, gzErr)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	for {
		header, headerErr := tarReader.Next()
		if errors.Is(headerErr, io.EOF) {
			break
		}
		if headerErr != nil {
			return fmt.Errorf("%w: reading tar: %w", ErrExtractionFailed, headerErr)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		name := filepath.Base(header.Name)
		if name == "act" || name == "act.exe" {
			// Limit extracted size to prevent zip bombs
			limitedReader := io.LimitReader(tarReader, maxExtractedSize+1)
			return writeExecutable(limitedReader, destPath, maxExtractedSize)
		}
	}

	return fmt.Errorf("%w: act binary not found in archive", ErrExtractionFailed)
}

// extractFromZip extracts the act binary from a zip archive.
func extractFromZip(archivePath, destPath string) error {
	reader, openErr := zip.OpenReader(archivePath)
	if openErr != nil {
		return fmt.Errorf("%w: opening zip: %w", ErrExtractionFailed, openErr)
	}
	defer func() { _ = reader.Close() }()

	for _, file := range reader.File {
		name := filepath.Base(file.Name)
		if strings.EqualFold(name, "act.exe") || strings.EqualFold(name, "act") {
			rc, rcErr := file.Open()
			if rcErr != nil {
				return fmt.Errorf("%w: opening file in zip: %w", ErrExtractionFailed, rcErr)
			}
			// Limit extracted size to prevent zip bombs
			limitedReader := io.LimitReader(rc, maxExtractedSize+1)
			writeErr := writeExecutable(limitedReader, destPath, maxExtractedSize)
			_ = rc.Close()
			return writeErr
		}
	}

	return fmt.Errorf("%w: act binary not found in archive", ErrExtractionFailed)
}

// writeExecutable writes the binary content to a file atomically.
// maxSize is the maximum allowed file size; exceeding it returns an error.
func writeExecutable(r io.Reader, destPath string, maxSize int64) error {
	// Use process-unique temp file to avoid conflicts with concurrent downloads
	tempFile, createErr := os.CreateTemp(filepath.Dir(destPath), ".act-*.tmp")
	if createErr != nil {
		return fmt.Errorf("%w: creating output file: %w", ErrExtractionFailed, createErr)
	}
	tempPath := tempFile.Name()

	written, copyErr := io.Copy(tempFile, r)
	if copyErr != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: writing binary: %w", ErrExtractionFailed, copyErr)
	}

	if written > maxSize {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: extracted file exceeds maximum size of %d bytes", ErrExtractionFailed, maxSize)
	}

	// Sync to disk to catch disk full errors before closing
	if syncErr := tempFile.Sync(); syncErr != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: syncing file to disk: %w", ErrExtractionFailed, syncErr)
	}

	if closeErr := tempFile.Close(); closeErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: closing file: %w", ErrExtractionFailed, closeErr)
	}

	if renameErr := os.Rename(tempPath, destPath); renameErr != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: renaming file: %w", ErrExtractionFailed, renameErr)
	}

	return nil
}
