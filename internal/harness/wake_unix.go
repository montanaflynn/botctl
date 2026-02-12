//go:build !windows

package harness

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/montanaflynn/botctl-go/internal/db"
)

// newWakeChannel creates a channel that receives a value when SIGUSR1 is sent to this process.
// Returns the channel and a cleanup function.
func newWakeChannel(_ string, _ int) (chan struct{}, func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGUSR1)

	wakeCh := make(chan struct{}, 1)
	stopCh := make(chan struct{})

	go func() {
		for {
			select {
			case <-sigCh:
				select {
				case wakeCh <- struct{}{}:
				default:
				}
			case <-stopCh:
				return
			}
		}
	}()

	cleanup := func() {
		signal.Stop(sigCh)
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
