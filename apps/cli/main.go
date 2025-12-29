package main

import (
	"fmt"
	"os"

	"github.com/detent/cli/cmd"
	"github.com/detent/cli/internal/sentry"
)

func main() {
	os.Exit(run())
}

func run() int {
	// IMPORTANT: Defer order matters! Defers execute in LIFO order.
	// RecoverAndPanic must be deferred FIRST so it executes LAST,
	// allowing cleanup() to flush events before the re-panic.
	defer sentry.RecoverAndPanic()
	cleanup := sentry.Init(cmd.Version)
	defer cleanup()

	if err := cmd.Execute(); err != nil {
		sentry.CaptureError(err)
		// Print error since commands use SilenceErrors
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	return 0
}
