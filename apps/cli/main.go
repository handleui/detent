package main

import (
	"os"

	"github.com/detent/cli/cmd"
	"github.com/detent/cli/internal/sentry"
)

func main() {
	os.Exit(run())
}

func run() int {
	cleanup := sentry.Init(cmd.Version)
	defer cleanup()
	defer sentry.RecoverAndPanic()

	if err := cmd.Execute(); err != nil {
		sentry.CaptureError(err)
		return 1
	}
	return 0
}
