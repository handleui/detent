package signal

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// SetupSignalHandler creates a context that cancels on SIGINT or SIGTERM.
// The signal handler goroutine is automatically cleaned up when the parent
// context is cancelled.
func SetupSignalHandler(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-parent.Done():
			// Parent cancelled, stop watching signals
		}
		signal.Stop(sigChan)
		close(sigChan)
	}()

	return ctx
}

// PrintCancellationMessage prints an informational cancellation message to stderr.
func PrintCancellationMessage(commandName string) {
	const colorBlue = "\033[34m"
	const colorReset = "\033[0m"
	_, _ = fmt.Fprintf(os.Stderr, "\n%si %s stopped%s\n", colorBlue, commandName, colorReset)
}
