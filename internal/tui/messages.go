package tui

import (
	"time"

	"github.com/montanaflynn/botctl-go/internal/service"
)

// tickMsg fires on the refresh interval.
type tickMsg time.Time

// logTickMsg fires on the log refresh interval.
type logTickMsg time.Time

// processStoppedMsg is sent when a kill goroutine finishes (process is dead).
type processStoppedMsg struct{ botName string }

// pendingAction tracks an in-flight kill operation and what to do after.
type pendingAction struct {
	kind    string // "stop", "restart", "clear", "delete"
	botName string
	botID   string
	bot     service.BotInfo
}

// createDoneMsg is sent when the botctl create subprocess finishes.
type createDoneMsg struct {
	err error
}

// createProgressMsg carries a status line from the create process.
type createProgressMsg struct {
	line string
}
