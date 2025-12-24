//go:build unix

package act

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures the command to run in its own process group
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends a signal to an entire process group.
// Using negative PID sends the signal to all processes in the group.
func killProcessGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}

// getProcessGroupID returns the process group ID for a process
func getProcessGroupID(pid int) (int, error) {
	return syscall.Getpgid(pid)
}

// terminateProcess attempts graceful termination then force kill
func terminateProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if pgid, err := getProcessGroupID(cmd.Process.Pid); err == nil {
		_ = killProcessGroup(pgid, syscall.SIGTERM)
	} else {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
}

// forceKillProcess forcefully kills the process and its group
func forceKillProcess(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if pgid, err := getProcessGroupID(cmd.Process.Pid); err == nil {
		_ = killProcessGroup(pgid, syscall.SIGKILL)
	}
	_ = cmd.Process.Kill()
}
