//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess = kernel32.NewProc("OpenProcess")
	procCloseHandle = kernel32.NewProc("CloseHandle")
	procOpenEventW  = kernel32.NewProc("OpenEventW")
	procSetEvent    = kernel32.NewProc("SetEvent")
)

const (
	processQueryLimitedInfo = 0x1000
	eventModifyState        = 0x0002
)

// isProcessAlive checks if a process exists by attempting to open it.
func isProcessAlive(pid int) bool {
	h, _, _ := procOpenProcess.Call(processQueryLimitedInfo, 0, uintptr(pid))
	if h == 0 {
		return false
	}
	procCloseHandle.Call(h)
	return true
}

// KillProcessGroup uses taskkill /F /T to kill the process tree.
func KillProcessGroup(pid int) {
	_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run()

	// Wait briefly for process to exit
	for range 30 {
		time.Sleep(100 * time.Millisecond)
		if !isProcessAlive(pid) {
			return
		}
	}
}

// IsProcessAlive checks if a process with the given PID is still running.
func IsProcessAlive(pid int) bool {
	return isProcessAlive(pid)
}

// WakeProcess signals a named event to wake a sleeping harness on Windows.
func WakeProcess(pid int) {
	name, err := syscall.UTF16PtrFromString(fmt.Sprintf("botctl-wake-%d", pid))
	if err != nil {
		return
	}
	h, _, _ := procOpenEventW.Call(eventModifyState, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return
	}
	procSetEvent.Call(h)
	procCloseHandle.Call(h)
}

// sysProcAttr returns the SysProcAttr for starting a detached process on Windows.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
