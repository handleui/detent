//go:build unix

package git

import (
	"fmt"
	"os"
	"syscall"
)

// checkTmpDiskSpace validates that the temporary directory has sufficient space.
// Returns an error if disk usage exceeds maxTmpDiskUsagePct.
func checkTmpDiskSpace() error {
	tmpDir := os.TempDir()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(tmpDir, &stat); err != nil {
		return fmt.Errorf("checking disk space in %s: %w", tmpDir, err)
	}

	// Calculate usage percentage
	// Use Bavail (available to non-root) for more accurate free space
	totalBlocks := stat.Blocks
	availBlocks := stat.Bavail
	if totalBlocks == 0 {
		return nil // Can't calculate, allow operation
	}

	usedPct := float64(totalBlocks-availBlocks) / float64(totalBlocks) * 100

	if usedPct > maxTmpDiskUsagePct {
		return fmt.Errorf("insufficient disk space in %s: %.1f%% used (max %d%%)",
			tmpDir, usedPct, maxTmpDiskUsagePct)
	}

	return nil
}
