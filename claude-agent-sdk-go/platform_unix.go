//go:build !windows

package claude

import "syscall"

// processSysProcAttr returns the SysProcAttr for starting in a new process group.
func processSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGTERM to the process group led by the given PID.
func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}
