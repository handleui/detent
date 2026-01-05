package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu      sync.Mutex
	logFile *os.File
)

// Init initializes debug logging to the repo's .detent-debug.log file.
func Init(repoRoot string) error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		return nil // Already initialized
	}

	path := filepath.Clean(filepath.Join(repoRoot, ".detent-debug.log"))
	f, err := os.Create(path) //nolint:gosec // path is constructed from trusted repoRoot
	if err != nil {
		return err
	}
	logFile = f
	return nil
}

// Log writes a debug message.
func Log(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()

	if logFile == nil {
		return
	}
	_, _ = fmt.Fprintf(logFile, format+"\n", args...)
}

// Close closes the debug log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
