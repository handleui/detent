//go:build windows

package act

import (
	"os/exec"
)

// setupProcessGroup is a no-op on Windows (process groups work differently)
func setupProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't support Unix-style process groups
}

// terminateProcess attempts to terminate the process on Windows
func terminateProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

// forceKillProcess forcefully kills the process on Windows
func forceKillProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
