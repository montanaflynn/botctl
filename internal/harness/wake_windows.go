//go:build windows

package harness

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/montanaflynn/botctl-go/internal/db"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procCreateEventW     = kernel32.NewProc("CreateEventW")
	procWaitForSingleObj = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle      = kernel32.NewProc("CloseHandle")
)

const infinite = 0xFFFFFFFF

// newWakeChannel creates a named Windows event and returns a channel that
// receives a value when the event is signaled (by WakeProcess from the process package).
func newWakeChannel(_ string, pid int) (chan struct{}, func()) {
	wakeCh := make(chan struct{}, 1)

	eventName := fmt.Sprintf("botctl-wake-%d", pid)
	namePtr, err := syscall.UTF16PtrFromString(eventName)
	if err != nil {
		// Fallback: return channel that never fires
		return wakeCh, func() {}
	}

	h, _, _ := procCreateEventW.Call(0, 0, 0, uintptr(unsafe.Pointer(namePtr)))
	if h == 0 {
		return wakeCh, func() {}
	}
	handle := syscall.Handle(h)

	stopCh := make(chan struct{})
	go func() {
		for {
			done := make(chan bool, 1)
			go func() {
				r, _, _ := procWaitForSingleObj.Call(uintptr(handle), infinite)
				done <- (r == 0) // WAIT_OBJECT_0
			}()
			select {
			case signaled := <-done:
				if signaled {
					select {
					case wakeCh <- struct{}{}:
					default:
					}
				}
			case <-stopCh:
				procCloseHandle.Call(uintptr(handle))
				return
			}
		}
	}()

	cleanup := func() {
		close(stopCh)
	}
	return wakeCh, cleanup
}

// sleepUntilWake sleeps for the given duration but can be woken early
// by a signal on the provided channel.
func sleepUntilWake(seconds int, wakeCh <-chan struct{}) {
	select {
	case <-wakeCh:
	case <-time.After(time.Duration(seconds) * time.Second):
	}
}

// startInterruptForwarder starts a goroutine that listens for wake signals
// and forwards them to the interrupt channel if there are pending messages.
// Returns a stop channel that should be closed when the run completes.
func startInterruptForwarder(wakeCh <-chan struct{}, interruptCh chan<- struct{}, database *db.DB, botID string) chan struct{} {
	stopForward := make(chan struct{})
	go func() {
		for {
			select {
			case <-wakeCh:
				if database.HasPendingMessages(botID) {
					select {
					case interruptCh <- struct{}{}:
					default:
					}
					return
				}
			case <-stopForward:
				return
			}
		}
	}()
	return stopForward
}
