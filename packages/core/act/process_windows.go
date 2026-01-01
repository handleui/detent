//go:build windows

package act

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup is a no-op on Windows (process groups work differently)
func setupProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't support Unix-style process groups
}

// killProcessGroup on Windows just kills the process (no process group support)
func killProcessGroup(pid int, sig syscall.Signal) error {
	// Windows doesn't support Unix-style process groups or signals
	// The Cancel function will be called but process termination
	// is handled by Go's os.Process.Kill() via WaitDelay
	return nil
}
