package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/montanaflynn/botctl/pkg/create"
)

const (
	createFieldName = iota
	createFieldDesc
	createFieldInterval
	createFieldMaxTurns
	createFieldCount
)

var createLabels = [createFieldCount]string{
	"Name",
	"Description",
	"Interval (s)",
	"Max turns",
}

func (m *model) initCreateForm() {
	m.creating = true
	m.createPending = false
	m.createName = ""
	m.createStep = 0
	m.createLines = nil
	m.createInputs = make([]textinput.Model, createFieldCount)

	for i := range m.createInputs {
		t := textinput.New()
		t.CharLimit = 256
		t.Width = 40
		switch i {
		case createFieldName:
			t.Placeholder = "my-bot"
		case createFieldDesc:
			t.Placeholder = "what this bot does"
		case createFieldInterval:
			t.Placeholder = "300"
		case createFieldMaxTurns:
			t.Placeholder = "20"
		}
		m.createInputs[i] = t
	}

	m.createInputs[0].Focus()
}

// waitCreateProgress returns a command that reads the next progress line.
func waitCreateProgress(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return createProgressMsg{line}
	}
}

func (m model) createUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case createProgressMsg:
		m.createLines = append(m.createLines, msg.line)
		return m, waitCreateProgress(m.createProgressC)

	case createDoneMsg:
		m.creating = false
		m.createPending = false
		m.createProgressC = nil
		// Clear filter so new bot is visible
		m.filter.SetValue("")
		m.filterFocused = false
		m.filter.Blur()
		m.refreshBots()
		if msg.err != nil {
			m.notify = fmt.Sprintf("create failed: %v", msg.err)
			m.notifyErr = true
		} else {
			m.notify = fmt.Sprintf("created %s", m.createName)
			m.notifyErr = false
			// Select the new bot
			for i, name := range m.botNames {
				if name == m.createName {
					m.cursor = i
					m.tableOffset = 0
					m.ensureCursorVisible()
					m.selectCurrentBot()
					break
				}
			}
		}
		m.notifyTime = time.Now()
		return m, nil

	case tea.KeyMsg:
		// While pending, only allow quit
		if m.createPending {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.creating = false
			return m, nil
		case "enter", "tab":
			if m.createStep == createFieldCount-1 {
				return m.submitCreate()
			}
			m.createInputs[m.createStep].Blur()
			m.createStep++
			m.createInputs[m.createStep].Focus()
			return m, textinput.Blink
		case "shift+tab":
			if m.createStep > 0 {
				m.createInputs[m.createStep].Blur()
				m.createStep--
				m.createInputs[m.createStep].Focus()
				return m, textinput.Blink
			}
			return m, nil
		}
	}

	if m.createPending {
		return m, nil
	}

	// Forward to active input
	var cmd tea.Cmd
	m.createInputs[m.createStep], cmd = m.createInputs[m.createStep].Update(msg)
	return m, cmd
}

func (m model) submitCreate() (tea.Model, tea.Cmd) {
	m.createErr = ""
	name := strings.TrimSpace(m.createInputs[createFieldName].Value())
	desc := strings.TrimSpace(m.createInputs[createFieldDesc].Value())

	if name == "" {
		m.createErr = "name is required"
		m.createInputs[m.createStep].Blur()
		m.createStep = createFieldName
		m.createInputs[m.createStep].Focus()
		return m, textinput.Blink
	}

	if !create.ValidName(name) {
		m.createErr = "invalid name: use alphanumeric, hyphens, underscores (no spaces)"
		m.createInputs[m.createStep].Blur()
		m.createStep = createFieldName
		m.createInputs[m.createStep].Focus()
		return m, textinput.Blink
	}

	if desc == "" {
		m.createErr = "description is required"
		m.createInputs[m.createStep].Blur()
		m.createStep = createFieldDesc
		m.createInputs[m.createStep].Focus()
		return m, textinput.Blink
	}

	interval := 300
	if v := strings.TrimSpace(m.createInputs[createFieldInterval].Value()); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			interval = n
		}
	}

	maxTurns := 20
	if v := strings.TrimSpace(m.createInputs[createFieldMaxTurns].Value()); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTurns = n
		}
	}

	m.createPending = true
	m.createName = name
	m.createLines = nil

	ch := make(chan string, 16)
	m.createProgressC = ch

	p := create.Params{
		Name:        name,
		Description: desc,
		Interval:    interval,
		MaxTurns:    maxTurns,
	}

	runCreate := func() tea.Msg {
		_, err := create.Run(p, ch)
		close(ch)
		return createDoneMsg{err}
	}

	return m, tea.Batch(runCreate, waitCreateProgress(ch))
}

func (m model) createView() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Render("Create Bot")
	b.WriteString("\n  " + title + "\n\n")

	if m.createPending {
		b.WriteString(fmt.Sprintf("  Generating BOT.md for %s...\n\n", m.createName))
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		for _, line := range m.createLines {
			b.WriteString("  " + dim.Render(line) + "\n")
		}
		if len(m.createLines) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(dim.Render("  ctrl+c:quit"))
		return b.String()
	}

	for i, label := range createLabels {
		cursor := "  "
		if i == m.createStep {
			cursor = "> "
		}
		labelStr := lipgloss.NewStyle().Width(16).Render(label + ":")
		fmt.Fprintf(&b, "  %s%s%s\n", cursor, labelStr, m.createInputs[i].View())
	}

	if m.createErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n  " + errStyle.Render(m.createErr) + "\n")
	}

	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(
		"  enter:next  shift+tab:back  esc:cancel",
	)
	b.WriteString(help)

	return b.String()
}
