package tui

import (
	"fmt"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/montanaflynn/botctl-go/internal/process"
)

func (m model) submitTell() (tea.Model, tea.Cmd) {
	message := strings.TrimSpace(m.tellInput.Value())
	m.tellInput.SetValue("")
	m.tellInput.Blur()

	if message == "" {
		return m, nil
	}

	name := m.botNames[m.cursor]
	bot := m.botCache[name]
	id := m.botID(name)

	// Enqueue message in DB
	if err := m.database.EnqueueMessage(id, message); err != nil {
		m.setNotify(fmt.Sprintf("message failed: %v", err), true)
		return m, nil
	}

	m.appendLogEvent(name, fmt.Sprintf("message: %s", message))

	running, pid := process.IsRunning(id, m.database)
	if running {
		// Wake the harness from sleep immediately
		syscall.Kill(pid, syscall.SIGUSR1)
		m.setNotify(fmt.Sprintf("message queued for %s", name), false)
	} else {
		// Bot is stopped — start it so it picks up the message
		newPid, err := process.StartBot(name, bot.Path, bot.Config, false, m.database)
		if err != nil {
			m.setNotify(fmt.Sprintf("failed to start %s: %v", name, err), true)
			m.appendLogEvent(name, fmt.Sprintf("failed to start: %v", err))
		} else {
			m.setNotify(fmt.Sprintf("%s started with message (pid %d)", name, newPid), false)
			m.appendLogEvent(name, fmt.Sprintf("started (pid %d)", newPid))
		}
		m.refreshBots()
	}

	// Keep cursor on the messaged bot (table may have re-sorted)
	m.selectBotByName(name)
	return m, nil
}
