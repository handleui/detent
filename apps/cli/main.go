package main

import (
	"fmt"
	"os"
	"unicode"

	"github.com/detent/cli/cmd"
	"github.com/detent/cli/internal/sentry"
	"github.com/detent/cli/internal/tui"
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
		// Print error using unified exit format
		errMsg := err.Error()
		if errMsg != "" {
			runes := []rune(errMsg)
			runes[0] = unicode.ToUpper(runes[0])
			errMsg = string(runes)
		}
		fmt.Fprintln(os.Stderr, tui.ExitError(errMsg))
		return 1
	}
	return 0
}
