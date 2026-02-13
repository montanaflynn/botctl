//go:build !windows

package process

import (
	"os"
	"syscall"
	"time"
)

// isProcessAlive checks if a process with the given PID exists using signal 0.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// KillProcessGroup sends SIGTERM to the process group, waits up to 3 seconds,
// then sends SIGKILL if the process is still alive.
func KillProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	for range 30 {
		time.Sleep(100 * time.Millisecond)
		if !isProcessAlive(pid) {
			return
		}
	}

	// Force kill if still alive
	if isProcessAlive(pid) {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

// IsProcessAlive checks if a process with the given PID is still running.
func IsProcessAlive(pid int) bool {
	return isProcessAlive(pid)
}

// WakeProcess sends SIGUSR1 to the given PID to wake a sleeping harness.
func WakeProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGUSR1)
}

// sysProcAttr returns the SysProcAttr for starting a detached process group.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
