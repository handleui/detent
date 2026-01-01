//go:build windows

package git

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// checkTmpDiskSpace validates that the temporary directory has sufficient space.
// Returns an error if disk usage exceeds maxTmpDiskUsagePct.
func checkTmpDiskSpace() error {
	tmpDir := os.TempDir()

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	tmpDirPtr, err := windows.UTF16PtrFromString(tmpDir)
	if err != nil {
		return nil // Can't check, allow operation
	}

	if err := windows.GetDiskFreeSpaceEx(tmpDirPtr, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return fmt.Errorf("checking disk space in %s: %w", tmpDir, err)
	}

	if totalBytes == 0 {
		return nil // Can't calculate, allow operation
	}

	usedPct := float64(totalBytes-freeBytesAvailable) / float64(totalBytes) * 100

	if usedPct > maxTmpDiskUsagePct {
		return fmt.Errorf("insufficient disk space in %s: %.1f%% used (max %d%%)",
			tmpDir, usedPct, maxTmpDiskUsagePct)
	}

	return nil
}
