//go:build windows

package sdk

import (
	"fmt"
	"os/exec"
	"syscall"
)

// processSysProcAttr returns the SysProcAttr for starting in a new process group.
func processSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killProcessGroup terminates the process tree rooted at the given PID.
func killProcessGroup(pid int) {
	_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()
}
