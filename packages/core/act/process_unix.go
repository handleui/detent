//go:build unix

package act

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures the command to run in its own process group.
// With Setpgid: true, the child's PGID equals its PID, allowing us to kill
// the entire process tree with syscall.Kill(-pid, signal).
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends a signal to an entire process group.
func killProcessGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}
