package signal

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// SetupSignalHandler creates a context that cancels on SIGINT/SIGTERM.
// This enables graceful shutdown when the user presses Ctrl+C.
func SetupSignalHandler(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	return ctx
}

// PrintCancellationMessage prints a green cancellation message to stderr.
func PrintCancellationMessage(commandName string) {
	const colorGreen = "\033[32m"
	const colorReset = "\033[0m"
	_, _ = fmt.Fprintf(os.Stderr, "\n%s%s cancelled%s\n", colorGreen, commandName, colorReset)
}
