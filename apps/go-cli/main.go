package main

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/detent/go-cli/cmd"
	"github.com/detent/go-cli/internal/actbin"
	"github.com/detent/go-cli/internal/sentry"
	"github.com/detent/go-cli/internal/tui"
	"github.com/detentsh/core/act"
	"github.com/detentsh/core/extract"
	"github.com/detentsh/core/tools"
)

func init() {
	// Inject CLI-specific act binary resolver into core
	act.DefaultActPathResolver = actbin.ActPath

	// Inject sentry reporters for unknown patterns and unsupported tools
	extract.DefaultUnknownPatternReporter = func(patterns []string) {
		sentry.CaptureMessage("Unknown error patterns: " + strings.Join(patterns, ", "))
	}
	tools.DefaultUnsupportedToolsReporter = func(toolNames []string) {
		sentry.CaptureMessage("Unsupported tools: " + strings.Join(toolNames, ", "))
	}
}

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
