package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) submitTell() (tea.Model, tea.Cmd) {
	message := strings.TrimSpace(m.tellInput.Value())
	m.tellInput.SetValue("")
	m.tellInput.Blur()

	if message == "" {
		return m, nil
	}

	name := m.botNames[m.cursor]

	// Enqueue message + wake/start via service
	result, err := m.svc.SendMessage(name, message)
	if err != nil {
		m.setNotify(fmt.Sprintf("message failed: %v", err), true)
		return m, nil
	}

	m.appendLogEvent(name, fmt.Sprintf("message: %s", message))

	if strings.HasPrefix(result, "started") {
		m.appendLogEvent(name, result)
		m.refreshBots()
	}
	m.setNotify(fmt.Sprintf("message %s for %s", result, name), false)

	// Keep cursor on the messaged bot (table may have re-sorted)
	m.selectBotByName(name)
	return m, nil
}
