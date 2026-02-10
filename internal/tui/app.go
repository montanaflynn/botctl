package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/montanaflynn/botctl-go/internal/config"
	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/discovery"
	"github.com/montanaflynn/botctl-go/internal/process"
)

// Version is set by the CLI package before launching.
var Version = "dev"

type sortOrder int

const (
	sortNone sortOrder = iota
	sortAsc
	sortDesc
)

const maxVisibleRows = 5

type model struct {
	table         table.Model
	logView       viewport.Model
	filter        textinput.Model
	filterFocused bool
	width         int
	height        int
	ready         bool
	cursor        int                  // arrow navigation position
	activeLogs    []string             // currently viewed bot (0 or 1)
	logPanels     map[string]*logPanel // log state per bot
	botCache      map[string]*discovery.Bot
	botNames      []string // ordered bot names from last refresh
	notify        string
	notifyErr     bool
	notifyTime    time.Time
	tableTop      int
	logTop        int
	autoShown     bool
	newLogLines   int // unread log lines when scrolled up
	database      *db.DB
	allRows       []table.Row // all rows (pre-pagination)
	tableOffset   int         // first visible row index for pagination
	sortCol       int         // which table column (0-5) is sorted
	sortOrder     sortOrder   // current sort direction
	headerSlot    int         // position in header bar: 0=NAME, 1=filter, 2+=other columns

	creating        bool              // true when create screen is open
	createPending   bool              // true while waiting for Claude to generate
	createName      string            // name of bot being created
	createInputs    []textinput.Model // form fields: name, desc, interval, max_turns
	createStep      int               // which field is focused
	createErr       string            // inline validation error
	createLines     []string          // progress lines from Claude
	createProgressC chan string        // channel for create progress messages

	tellInput    textinput.Model // always-visible message input
	resuming     bool            // true when resume turn-count input is open
	resumeInput  textinput.Model // editable turn count
	pending      *pendingAction  // in-flight kill operation
	confirmDelete bool           // true when waiting for y/n to confirm delete
	deleteTarget  string         // bot name pending deletion
}

// slotToCol maps a header slot to a table column index. Returns -1 for the filter slot.
func slotToCol(slot int) int {
	if slot == 0 {
		return 0
	}
	if slot == 1 {
		return -1 // filter, not a column
	}
	return slot - 1
}

// colToSlot maps a table column index to its header slot.
func colToSlot(col int) int {
	if col == 0 {
		return 0
	}
	return col + 1
}

func newModel(database *db.DB) model {
	cols := []table.Column{
		{Title: "NAME", Width: 20},
		{Title: "STATUS", Width: 10},
		{Title: "PID", Width: 8},
		{Title: "RUNS", Width: 6},
		{Title: "COST", Width: 10},
		{Title: "LAST RUN", Width: 20},
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(false),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		Bold(true).
		Foreground(lipgloss.Color("255")).
		BorderBottom(false).
		PaddingLeft(0)
	s.Cell = s.Cell.PaddingLeft(0)
	s.Selected = lipgloss.NewStyle()
	t.SetStyles(s)

	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.Width = 15
	ti.Prompt = ""
	ti.PlaceholderStyle = helpStyle
	ti.TextStyle = helpStyle
	ti.Cursor.Style = helpStyle
	lv := viewport.New(80, 10)
	lv.MouseWheelEnabled = true

	tell := textinput.New()
	tell.Placeholder = "message for the bot..."
	tell.CharLimit = 1024
	tell.Width = 60

	return model{
		table:         t,
		logView:       lv,
		filter:        ti,
		tellInput:     tell,
		filterFocused: false,
		logPanels:     make(map[string]*logPanel),
		botCache:      make(map[string]*discovery.Bot),
		database:      database,
		sortCol:       5, // LAST RUN column
		sortOrder:     sortDesc,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.WindowSize(),
		tickCmd(),
		logTickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func logTickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return logTickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Handle process stopped before anything else
	if msg, ok := msg.(processStoppedMsg); ok {
		return m.handleProcessStopped(msg)
	}

	// Route to create form when active
	if m.creating {
		return m.createUpdate(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.refreshBots()
		m.recalcLayout()
		m.initLogView()
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		// Global quit
		if !m.filterFocused {
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			}
		} else if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// Inline message input
		if m.tellInput.Focused() {
			switch msg.String() {
			case "esc":
				m.tellInput.Blur()
				return m, nil
			case "enter":
				return m.submitTell()
			default:
				m.tellInput, cmd = m.tellInput.Update(msg)
				return m, cmd
			}
		}

		// Delete confirmation
		if m.confirmDelete {
			switch msg.String() {
			case "y":
				return m.executeDelete()
			case "n", "esc":
				m.confirmDelete = false
				m.deleteTarget = ""
				return m, nil
			default:
				return m, nil
			}
		}

		// Inline resume turn-count input
		if m.resuming {
			switch msg.String() {
			case "esc":
				m.resuming = false
				return m, nil
			case "enter":
				return m.submitResume()
			default:
				m.resumeInput, cmd = m.resumeInput.Update(msg)
				return m, cmd
			}
		}

		// Filter / column header key handling
		if m.filterFocused {
			onFilter := m.headerSlot == 1

			if onFilter {
				// --- Filter input row ---
				switch msg.String() {
				case "enter":
					if m.filter.Value() != "" && len(m.botNames) > 0 {
						m.cursor = 0
						m.selectCurrentBot()
					}
					return m, nil
				case "left":
					if m.filter.Value() != "" && m.filter.Position() > 0 {
						m.filter, cmd = m.filter.Update(msg)
						return m, cmd
					}
					return m, nil
				case "right":
					if m.filter.Value() != "" && m.filter.Position() < len(m.filter.Value()) {
						m.filter, cmd = m.filter.Update(msg)
						return m, cmd
					}
					return m, nil
				case "down":
					// Move to column header row, start at NAME
					m.headerSlot = 0
					m.filter.Blur()
					return m, nil
				case "esc":
					m.filterFocused = false
					m.filter.Blur()
					m.syncTableCursor()
					return m, nil
				case "tab":
					m.filterFocused = false
					m.filter.Blur()
					m.syncTableCursor()
					return m, nil
				default:
					prevValue := m.filter.Value()
					m.filter, cmd = m.filter.Update(msg)
					if m.filter.Value() != prevValue {
						m.cursor = 0
						m.refreshBots()
					}
					return m, cmd
				}
			} else {
				// --- Column header row ---
				switch msg.String() {
				case "enter":
					col := slotToCol(m.headerSlot)
					if col == m.sortCol {
						// Toggle between asc and desc
						if m.sortOrder == sortAsc {
							m.sortOrder = sortDesc
						} else {
							m.sortOrder = sortAsc
						}
					} else {
						m.sortCol = col
						m.sortOrder = sortAsc
					}
					m.refreshBots()
					m.recalcLayout()
					return m, nil
				case "left":
					// Navigate columns, skip filter slot (1)
					if m.headerSlot == 2 {
						m.headerSlot = 0
					} else if m.headerSlot > 2 {
						m.headerSlot--
					}
					return m, nil
				case "right":
					maxSlot := len(m.table.Columns()) // 6 columns → max slot 6
					if m.headerSlot == 0 {
						m.headerSlot = 2
					} else if m.headerSlot < maxSlot {
						m.headerSlot++
					}
					return m, nil
				case "up":
					// Back to filter
					m.headerSlot = 1
					m.filter.Focus()
					return m, textinput.Blink
				case "down":
					// Down to table
					m.filterFocused = false
					m.filter.Blur()
					m.selectCurrentBot()
					return m, nil
				case "esc":
					m.filterFocused = false
					m.filter.Blur()
					m.syncTableCursor()
					return m, nil
				case "tab":
					m.filterFocused = false
					m.filter.Blur()
					m.syncTableCursor()
					return m, nil
				default:
					// Fall through to normal key handling (q quits, s starts, etc.)
					m.filterFocused = false
					m.filter.Blur()
				}
			}
		}

		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.selectCurrentBot()
			} else {
				// At top of list — focus column headers
				m.filterFocused = true
				m.headerSlot = 0
				m.filter.Blur()
				m.syncTableCursor()
				return m, nil
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.botNames)-1 {
				m.cursor++
				m.selectCurrentBot()
			}
			return m, nil
		case "f":
			m.filterFocused = true
			m.headerSlot = 1
			m.filter.Focus()
			m.syncTableCursor()
			return m, textinput.Blink
		case "s":
			if m.pending != nil {
				return m, nil
			}
			return m.toggleBot()
		case "r":
			if m.pending != nil {
				return m, nil
			}
			return m.resumeBot()
		case "o":
			m.openBotDir()
			return m, nil
		case "c":
			if m.pending != nil {
				return m, nil
			}
			return m.clearBot()
		case "n":
			m.initCreateForm()
			return m, textinput.Blink
		case "d":
			if m.pending != nil {
				return m, nil
			}
			if len(m.botNames) > 0 {
				name := m.cursorBotName()
				if name != "" {
					m.confirmDelete = true
					m.deleteTarget = name
				}
			}
			return m, nil
		case "m":
			if m.pending != nil {
				return m, nil
			}
			if len(m.botNames) > 0 {
				m.tellInput.Focus()
				return m, textinput.Blink
			}
		case "enter":
			if len(m.botNames) > 0 && m.pending == nil {
				m.tellInput.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case "tab":
			m.filterFocused = true
			m.headerSlot = 1
			m.filter.Focus()
			m.syncTableCursor()
			return m, textinput.Blink
		}
		return m, nil

	case tickMsg:
		m.refreshBots()
		return m, tickCmd()

	case logTickMsg:
		m.refreshLogs()
		return m, logTickCmd()

	}

	// Forward blink messages to textinput
	if m.filterFocused {
		m.filter, cmd = m.filter.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	if m.tellInput.Focused() {
		m.tellInput, cmd = m.tellInput.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	y := msg.Y

	isClick := msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft

	// Check if click is on the message prompt row (second to last line)
	if isClick && m.cursorBotName() != "" && y == m.height-2 {
		m.filterFocused = false
		m.filter.Blur()
		m.tellInput.Focus()
		return m, textinput.Blink
	}

	// Check if click is on the <bots> label line (filter area)
	botsLabelY := headerLines(m.width) + 1 // +1 for blank line after header
	if isClick && y == botsLabelY {
		m.filterFocused = true
		m.headerSlot = 1
		m.filter.Focus()
		m.syncTableCursor()
		return m, textinput.Blink
	}

	switch {
	case y >= m.tableTop && (m.logTop == 0 || y < m.logTop):
		// In table area
		if isClick && y == m.tableTop {
			// Click on table header row — determine which column and sort
			col := m.colFromX(msg.X)
			if col >= 0 {
				m.filterFocused = true
				m.filter.Blur()
				m.headerSlot = colToSlot(col)
				if col == m.sortCol {
					if m.sortOrder == sortAsc {
						m.sortOrder = sortDesc
					} else {
						m.sortOrder = sortAsc
					}
				} else {
					m.sortCol = col
					m.sortOrder = sortAsc
				}
				m.refreshBots()
				m.recalcLayout()
			}
			return m, nil
		}

		if m.filterFocused {
			m.filterFocused = false
			m.filter.Blur()
		}

		if isClick {
			rowY := y - m.tableTop - 1
			absIdx := m.tableOffset + rowY
			if rowY >= 0 && absIdx < len(m.botNames) {
				m.cursor = absIdx
				m.selectCurrentBot()
			}
		}
		m.syncTableCursor()
		return m, nil

	case y >= m.logTop:
		// In log area
		if m.filterFocused {
			m.filterFocused = false
			m.filter.Blur()
		}
		m.logView, cmd = m.logView.Update(msg)
		if m.logView.AtBottom() && m.newLogLines > 0 {
			m.syncLogContent()
		}
		return m, cmd
	}

	// Mouse wheel anywhere scrolls logs if active
	if len(m.activeLogs) > 0 && msg.Action == tea.MouseActionPress &&
		(msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *model) recalcLayout() {
	rowCount := len(m.table.Rows())
	tableHeight := rowCount + 1 // +1 for header (no border)
	if tableHeight < 2 {
		tableHeight = 2
	}
	m.table.SetHeight(tableHeight)
	m.table.SetWidth(m.width)

	// Distribute column widths to fill terminal width
	// Fixed columns: Status(10) + PID(8) + Runs(6) + Cost(10) + gaps(~6 for 6 cols)
	fixed := 10 + 8 + 6 + 10 + 6
	remaining := m.width - fixed
	lastRunW := 20
	nameW := remaining - lastRunW
	if nameW < 10 {
		nameW = 10
	}
	if lastRunW < 10 {
		lastRunW = 10
	}
	m.table.SetColumns([]table.Column{
		{Title: "NAME", Width: nameW},
		{Title: "STATUS", Width: 10},
		{Title: "PID", Width: 8},
		{Title: "RUNS", Width: 6},
		{Title: "COST", Width: 10},
		{Title: "LAST RUN", Width: lastRunW},
	})

	tableRendered := strings.Count(m.table.View(), "\n") + 1
	headerHeight := headerLines(m.width)

	m.tableTop = headerHeight + 2                   // header + blank + <bots>
	m.logTop = headerHeight + 2 + tableRendered + 2 // +\n\n+<logs>

	used := headerHeight + 1 + 1 + tableRendered + 1 + 1 + 1 + 1 + 1 // header+blank+<bots> +table +blank+<logs> +footer(help+message) +gap

	logHeight := m.height - used
	if logHeight < 3 {
		logHeight = 3
	}

	// Resize viewport without recreating it (preserves scroll position)
	m.logView.Width = m.width
	m.logView.Height = logHeight
	m.logView.YPosition = m.logTop
}

// initLogView sets up the log viewport with initial content.
func (m *model) initLogView() {
	if len(m.activeLogs) > 0 {
		if p, ok := m.logPanels[m.activeLogs[0]]; ok {
			m.logView.SetContent(styleLogs(m.wrapLog(p.content()), m.width))
			m.logView.GotoBottom()
			m.newLogLines = 0
		}
	} else {
		m.logView.SetContent(helpStyle.Render("  select a bot to view logs"))
	}
}

func (m *model) refreshBots() {
	bots, _ := discovery.DiscoverBots()
	m.botCache = make(map[string]*discovery.Bot, len(bots))
	m.botNames = make([]string, 0, len(bots))

	filterText := strings.ToLower(m.filter.Value())

	var rows []table.Row
	lastRunRaw := make(map[string]string) // botName → raw ISO timestamp for sorting
	for _, bot := range bots {
		if filterText != "" && !strings.Contains(strings.ToLower(bot.Name), filterText) {
			continue
		}

		b := bot
		m.botCache[bot.Name] = &b
		m.botNames = append(m.botNames, bot.Name)

		running, pid := process.IsRunning(bot.ID, m.database)
		status := "stopped"
		pidStr := "-"
		if running {
			status = "running"
			pidStr = fmt.Sprintf("%d", pid)
		}

		// Optimistic UI: override status for in-flight actions
		if m.pending != nil && m.pending.botName == bot.Name {
			switch m.pending.kind {
			case "stop":
				status = "stopping"
				pidStr = "-"
			case "restart":
				status = "restarting"
				pidStr = "-"
			}
		}

		stats := m.database.GetBotStats(bot.ID)
		runs := fmt.Sprintf("%d", stats.Runs)
		cost := fmt.Sprintf("$%.2f", stats.TotalCost)
		lastRun := "-"
		if stats.LastRun != "" {
			lastRun = timeAgo(stats.LastRun)
			lastRunRaw[bot.Name] = stats.LastRun
		}

		rows = append(rows, table.Row{bot.Name, status, pidStr, runs, cost, lastRun})
	}

	// On initial load, switch to status sort if any bot is running
	if !m.autoShown && len(rows) > 0 {
		for _, row := range rows {
			if row[1] == "running" {
				m.sortCol = 1
				m.sortOrder = sortAsc
				break
			}
		}
	}

	// Sort rows if a sort column is active
	if m.sortOrder != sortNone && len(rows) > 0 {
		col := m.sortCol
		asc := m.sortOrder == sortAsc
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i][col], rows[j][col]
			// LAST RUN column: sort by raw timestamp, not display string
			if col == 5 {
				a, b = lastRunRaw[rows[i][0]], lastRunRaw[rows[j][0]]
			}
			// Try numeric comparison for numeric-looking columns
			if na, err := parseNumeric(a); err == nil {
				if nb, err := parseNumeric(b); err == nil {
					if asc {
						return na < nb
					}
					return na > nb
				}
			}
			if asc {
				return strings.ToLower(a) < strings.ToLower(b)
			}
			return strings.ToLower(a) > strings.ToLower(b)
		})
		// Rebuild botNames to match sorted order
		m.botNames = m.botNames[:0]
		for _, row := range rows {
			m.botNames = append(m.botNames, row[0])
		}
	}

	// Clamp cursor
	if m.cursor >= len(rows) && len(rows) > 0 {
		m.cursor = len(rows) - 1
	}
	if len(rows) == 0 {
		m.cursor = 0
	}

	// Clamp tableOffset and set only visible rows
	m.allRows = rows
	m.ensureCursorVisible()

	// On initial load, select the first bot
	if !m.autoShown && len(rows) > 0 {
		m.autoShown = true
		m.cursor = 0
		m.ensureCursorVisible()
		m.selectCurrentBot()
	}

	if m.ready {
		m.recalcLayout()
	}
}

// syncTableCursor updates the table's focus state and cursor position.
func (m *model) syncTableCursor() {
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true).Foreground(lipgloss.Color("255")).BorderBottom(false).PaddingLeft(0)
	s.Cell = s.Cell.PaddingLeft(0)

	if m.filterFocused || len(m.botNames) == 0 {
		m.table.Blur()
		s.Selected = lipgloss.NewStyle()
		m.table.SetStyles(s)
		return
	}

	m.table.Focus()
	m.table.SetCursor(m.cursor - m.tableOffset)
	s.Selected = lipgloss.NewStyle().
		Background(lipgloss.Color("238"))
	m.table.SetStyles(s)
}

// ensureCursorVisible adjusts tableOffset so the cursor is within the visible window,
// then sets the visible row slice on the table widget.
func (m *model) ensureCursorVisible() {
	if m.cursor < m.tableOffset {
		m.tableOffset = m.cursor
	}
	if m.cursor >= m.tableOffset+maxVisibleRows {
		m.tableOffset = m.cursor - maxVisibleRows + 1
	}
	// Clamp offset
	if m.tableOffset < 0 {
		m.tableOffset = 0
	}
	if max := len(m.allRows) - maxVisibleRows; m.tableOffset > max && max > 0 {
		m.tableOffset = max
	}

	end := m.tableOffset + maxVisibleRows
	if end > len(m.allRows) {
		end = len(m.allRows)
	}
	m.table.SetRows(m.allRows[m.tableOffset:end])
	m.syncTableCursor()
}

func (m *model) refreshLogs() {
	if len(m.activeLogs) == 0 {
		return
	}
	name := m.activeLogs[0]
	if p, ok := m.logPanels[name]; ok {
		atBottom := m.logView.AtBottom()
		prevLen := len(p.lines)
		if p.refresh() {
			newCount := len(p.lines) - prevLen
			if atBottom {
				m.logView.SetContent(styleLogs(m.wrapLog(p.content()), m.width))
				m.logView.GotoBottom()
				m.newLogLines = 0
			} else {
				// Don't touch the viewport — just track unread count
				m.newLogLines += newCount
			}
		}
	}
}

// syncLogContent updates the viewport with the latest log content.
// Called when the user scrolls back to bottom.
func (m *model) syncLogContent() {
	if len(m.activeLogs) == 0 {
		return
	}
	if p, ok := m.logPanels[m.activeLogs[0]]; ok {
		m.logView.SetContent(styleLogs(m.wrapLog(p.content()), m.width))
		m.logView.GotoBottom()
		m.newLogLines = 0
	}
}

// styledTableView renders the table and applies purple background to the active-log bot's row.
func (m model) styledTableView() string {
	raw := m.table.View()
	lines := strings.Split(raw, "\n")

	// Replace header line with custom render that includes the filter and sort arrows
	if len(lines) > 0 {
		cols := m.table.Columns()
		var header strings.Builder
		for i, col := range cols {
			var cell string

			title := col.Title

			// Sort arrow to right-align in the column
			arrow := ""
			if m.sortOrder != sortNone && m.sortCol == i {
				if m.sortOrder == sortAsc {
					arrow = "▲"
				} else {
					arrow = "▼"
				}
			}

			// Green when the header slot arrow cursor is on this column
			colStyle := headerStyle
			if m.filterFocused && m.headerSlot != 1 && colToSlot(i) == m.headerSlot {
				colStyle = headerSelectedStyle
			}

			if arrow != "" {
				cell = colStyle.Render(title + " " + arrow)
			} else {
				cell = colStyle.Render(title)
			}

			visualW := lipgloss.Width(cell)
			pad := col.Width - visualW
			if pad > 0 {
				cell += strings.Repeat(" ", pad)
			}
			cell += " " // matches Header style PaddingRight(1)
			header.WriteString(cell)
		}
		lines[0] = header.String()
	}

	return strings.Join(lines, "\n")
}

// styleLogs applies per-line coloring to log content.
// Tag lines are rendered as bordered boxes for visual separation.
func styleLogs(content string, width int) string {
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		// XML-style tags — render as bordered box
		case strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, ">"):
			label := tagToLabel(trimmed)
			boxWidth := width - 2 // account for border chars
			if boxWidth < 10 {
				boxWidth = 10
			}
			box := logTagStyle.Width(boxWidth).Render(label)
			result = append(result, box)

		// Legacy markdown formats (old logs)
		case strings.HasPrefix(trimmed, "## "):
			result = append(result, logHeadingStyle.Render(line))
		case strings.HasPrefix(trimmed, "### "):
			result = append(result, logHeadingStyle.Render(line))
		case strings.HasPrefix(trimmed, "#### "):
			result = append(result, logSubheadingStyle.Render(line))
		case trimmed == "---":
			result = append(result, logDimStyle.Render(line))
		case strings.HasPrefix(trimmed, "$") && strings.Contains(trimmed, " | ") && strings.Contains(trimmed, "turns"):
			result = append(result, logDimStyle.Render(line))
		case strings.HasPrefix(trimmed, "sleeping "):
			result = append(result, logDimStyle.Render(line))
		case strings.HasPrefix(trimmed, "warning:"):
			result = append(result, logDimStyle.Render(line))
		case strings.HasPrefix(trimmed, "─── "):
			result = append(result, logHeadingStyle.Render(line))
		case strings.HasPrefix(trimmed, "── "):
			result = append(result, logHeadingStyle.Render(line))
		case strings.HasPrefix(trimmed, "cost:"):
			result = append(result, logDimStyle.Render(line))
		case strings.HasPrefix(trimmed, "▶ "), strings.HasPrefix(trimmed, "◀ "), strings.HasPrefix(trimmed, "✗ "):
			result = append(result, logDimStyle.Render(line))
		case strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "] running task"):
			result = append(result, logDimStyle.Render(line))
		default:
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// tagToLabel converts a raw tag like "<cost usd=$0.3 turns=3>" into "Cost usd=$0.3 turns=3".
func tagToLabel(tag string) string {
	// Strip < and >
	tag = strings.TrimPrefix(tag, "<")
	tag = strings.TrimSuffix(tag, ">")
	// Capitalize first letter
	if len(tag) > 0 {
		tag = strings.ToUpper(tag[:1]) + tag[1:]
	}
	return tag
}

// wrapLog wraps long lines to fit the terminal width using rune-aware cutting.
func (m model) wrapLog(content string) string {
	if m.width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	var wrapped []string
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) <= m.width {
			wrapped = append(wrapped, line)
			continue
		}
		for len(runes) > 0 {
			cut := m.width
			if cut > len(runes) {
				cut = len(runes)
			}
			wrapped = append(wrapped, string(runes[:cut]))
			runes = runes[cut:]
		}
	}
	return strings.Join(wrapped, "\n")
}

// colFromX returns which table column index an X coordinate falls in, or -1.
func (m model) colFromX(x int) int {
	cols := m.table.Columns()
	offset := 0
	for i, col := range cols {
		end := offset + col.Width + 1 // +1 for padding
		if x >= offset && x < end {
			return i
		}
		offset = end
	}
	return -1
}

// selectBotByName moves the cursor to the named bot and selects it.
func (m *model) selectBotByName(name string) {
	for i, n := range m.botNames {
		if n == name {
			m.cursor = i
			m.selectCurrentBot()
			return
		}
	}
}

// selectCurrentBot opens logs for the bot at the current cursor position.
func (m *model) selectCurrentBot() {
	m.ensureCursorVisible()
	name := m.cursorBotName()
	if name == "" {
		return
	}
	// Switch to this bot's log (don't toggle off)
	if len(m.activeLogs) == 1 && m.activeLogs[0] == name {
		return
	}
	for _, old := range m.activeLogs {
		delete(m.logPanels, old)
	}
	m.activeLogs = []string{name}
	m.newLogLines = 0
	m.logPanels[name] = newLogPanel(name, m.botID(name), m.width, m.database)
	m.recalcLayout()
	m.initLogView()
}

func (m *model) cursorBotName() string {
	if m.cursor < 0 || m.cursor >= len(m.botNames) {
		return ""
	}
	return m.botNames[m.cursor]
}

// botID returns the stable DB key for a bot (from config id field, or folder name).
func (m *model) botID(name string) string {
	if b, ok := m.botCache[name]; ok && b.ID != "" {
		return b.ID
	}
	return name
}

func (m *model) openBotDir() {
	name := m.cursorBotName()
	if name == "" {
		return
	}
	bot := m.botCache[name]
	if bot == nil {
		return
	}
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	default:
		cmd = "xdg-open"
	}
	exec.Command(cmd, bot.Path).Start()
}

func (m *model) toggleBot() (tea.Model, tea.Cmd) {
	name := m.cursorBotName()
	if name == "" {
		return m, nil
	}
	bot := m.botCache[name]
	if bot == nil {
		return m, nil
	}

	id := m.botID(name)
	running, pid := process.IsRunning(id, m.database)
	if running {
		// Stop: send kill in goroutine, no DB access
		m.pending = &pendingAction{kind: "stop", botName: name, botID: id, bot: bot}
		m.setNotify(fmt.Sprintf("stopping %s...", name), false)
		m.appendLogEvent(name, "stopping...")
		m.refreshBots()
		return m, killProcessCmd(name, pid)
	}
	// Start: synchronous
	newPid, err := process.StartBot(name, bot.Path, bot.Config, false, m.database)
	if err != nil {
		m.setNotify(fmt.Sprintf("failed to start %s: %v", name, err), true)
		m.appendLogEvent(name, fmt.Sprintf("failed to start: %v", err))
		return m, nil
	}
	m.setNotify(fmt.Sprintf("%s started (pid %d)", name, newPid), false)
	m.appendLogEvent(name, fmt.Sprintf("started (pid %d)", newPid))
	m.refreshBots()

	return m, nil
}

// clearBot restarts a running bot to clear its session context. If stopped, just starts it.
func (m *model) clearBot() (tea.Model, tea.Cmd) {
	name := m.cursorBotName()
	if name == "" {
		return m, nil
	}
	bot := m.botCache[name]
	if bot == nil {
		return m, nil
	}

	id := m.botID(name)
	running, pid := process.IsRunning(id, m.database)
	if running {
		m.pending = &pendingAction{kind: "clear", botName: name, botID: id, bot: bot}
		m.setNotify(fmt.Sprintf("clearing %s...", name), false)
		m.appendLogEvent(name, "clearing context...")
		m.refreshBots()
		return m, killProcessCmd(name, pid)
	}
	// Not running — just start fresh
	newPid, err := process.StartBot(name, bot.Path, bot.Config, false, m.database)
	if err != nil {
		m.setNotify(fmt.Sprintf("failed to start %s: %v", name, err), true)
		m.appendLogEvent(name, fmt.Sprintf("failed to start: %v", err))
		return m, nil
	}
	m.setNotify(fmt.Sprintf("%s started (pid %d)", name, newPid), false)
	m.appendLogEvent(name, fmt.Sprintf("started (pid %d)", newPid))
	m.refreshBots()
	return m, nil
}

// executeDelete performs the bot deletion after y/n confirmation.
func (m *model) executeDelete() (tea.Model, tea.Cmd) {
	name := m.deleteTarget
	m.confirmDelete = false
	m.deleteTarget = ""

	bot := m.botCache[name]
	if bot == nil {
		return m, nil
	}
	id := m.botID(name)

	running, pid := process.IsRunning(id, m.database)
	if running {
		// Stop first, then delete in handleProcessStopped
		m.pending = &pendingAction{kind: "delete", botName: name, botID: id, bot: bot}
		m.setNotify(fmt.Sprintf("stopping %s...", name), false)
		return m, killProcessCmd(name, pid)
	}

	// Not running — delete immediately
	m.finishDelete(name, id, bot)
	return m, nil
}

// finishDelete removes DB data and the bot directory.
func (m *model) finishDelete(name, id string, bot *discovery.Bot) {
	m.database.DeleteBotData(id)
	os.RemoveAll(bot.Path)

	// Clean up log panel if viewing this bot
	if len(m.activeLogs) > 0 && m.activeLogs[0] == name {
		delete(m.logPanels, name)
		m.activeLogs = nil
		m.logView.SetContent(helpStyle.Render("  select a bot to view logs"))
	}

	m.setNotify(fmt.Sprintf("%s deleted", name), false)
	m.refreshBots()
}

// resumeBot opens the inline resume input pre-filled with the current max_turns from BOT.md.
func (m *model) resumeBot() (tea.Model, tea.Cmd) {
	name := m.cursorBotName()
	if name == "" {
		return m, nil
	}
	bot := m.botCache[name]
	if bot == nil {
		return m, nil
	}

	// Re-read BOT.md to get the current max_turns value
	maxTurns := 0
	if cfg, err := config.FromMD(bot.Path + "/BOT.md"); err == nil {
		maxTurns = cfg.MaxTurns
	}
	if maxTurns <= 0 {
		maxTurns = 50 // sensible default
	}

	m.resuming = true
	m.resumeInput = textinput.New()
	m.resumeInput.CharLimit = 6
	m.resumeInput.Width = 6
	m.resumeInput.SetValue(strconv.Itoa(maxTurns))
	m.resumeInput.Focus()
	return m, textinput.Blink
}

// submitResume parses the turn count and sends a resume:N command.
func (m model) submitResume() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.resumeInput.Value())
	m.resuming = false
	if val == "" {
		return m, nil
	}
	turns, err := strconv.Atoi(val)
	if err != nil || turns <= 0 {
		m.setNotify("invalid turn count", true)
		return m, nil
	}

	name := m.cursorBotName()
	if name == "" {
		return m, nil
	}
	id := m.botID(name)
	bot := m.botCache[name]
	if bot == nil {
		return m, nil
	}

	msg := fmt.Sprintf("resume:%d", turns)
	if err := m.database.EnqueueMessage(id, msg); err != nil {
		m.setNotify(fmt.Sprintf("resume failed: %v", err), true)
		return m, nil
	}

	m.appendLogEvent(name, fmt.Sprintf("resume for %d turns", turns))

	running, pid := process.IsRunning(id, m.database)
	if running {
		syscall.Kill(pid, syscall.SIGUSR1)
		m.setNotify(fmt.Sprintf("resume sent to %s (%d turns)", name, turns), false)
	} else {
		newPid, err := process.StartBot(name, bot.Path, bot.Config, false, m.database)
		if err != nil {
			m.setNotify(fmt.Sprintf("failed to start %s: %v", name, err), true)
			m.appendLogEvent(name, fmt.Sprintf("failed to start: %v", err))
		} else {
			m.setNotify(fmt.Sprintf("%s started with resume (pid %d)", name, newPid), false)
			m.appendLogEvent(name, fmt.Sprintf("started (pid %d)", newPid))
		}
		m.refreshBots()
	}

	m.selectBotByName(name)
	return m, nil
}

// killProcessCmd returns a tea.Cmd that kills a process group with zero DB access.
// Sends SIGTERM, polls for up to 3s, then SIGKILL if still alive.
func killProcessCmd(name string, pid int) tea.Cmd {
	return func() tea.Msg {
		_ = syscall.Kill(-pid, syscall.SIGTERM)

		for i := 0; i < 30; i++ {
			time.Sleep(100 * time.Millisecond)
			proc, err := os.FindProcess(pid)
			if err != nil {
				return processStoppedMsg{botName: name}
			}
			if err := proc.Signal(syscall.Signal(0)); err != nil {
				return processStoppedMsg{botName: name}
			}
		}

		// Force kill if still alive
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		return processStoppedMsg{botName: name}
	}
}

// handleProcessStopped dispatches on the pending action kind after the kill goroutine completes.
func (m model) handleProcessStopped(msg processStoppedMsg) (tea.Model, tea.Cmd) {
	if m.pending == nil {
		return m, nil
	}
	p := m.pending
	m.pending = nil

	// Clean up PID from DB (synchronous, fast)
	m.database.RemovePID(p.botID)

	switch p.kind {
	case "stop":
		m.setNotify(fmt.Sprintf("%s stopped", p.botName), false)
		m.appendLogEvent(p.botName, "stopped")
		m.refreshBots()

	case "clear":
		newPid, err := process.StartBot(p.botName, p.bot.Path, p.bot.Config, false, m.database)
		if err != nil {
			m.setNotify(fmt.Sprintf("failed to start %s: %v", p.botName, err), true)
			m.appendLogEvent(p.botName, fmt.Sprintf("failed to start: %v", err))
		} else {
			m.setNotify(fmt.Sprintf("%s cleared (pid %d)", p.botName, newPid), false)
			m.appendLogEvent(p.botName, fmt.Sprintf("context cleared (pid %d)", newPid))
		}
		m.refreshBots()

	case "delete":
		m.finishDelete(p.botName, p.botID, p.bot)
	}
	return m, nil
}

// appendLogEvent adds an event line to a bot's log panel and refreshes the viewport.
func (m *model) appendLogEvent(name, event string) {
	if p, ok := m.logPanels[name]; ok {
		p.appendEvent(event)
		m.logView.SetContent(styleLogs(m.wrapLog(p.content()), m.width))
		m.logView.GotoBottom()
		m.newLogLines = 0
	}
}

// setNotify sets the notification message and timestamp.
func (m *model) setNotify(text string, isErr bool) {
	m.notify = text
	m.notifyErr = isErr
	m.notifyTime = time.Now()
}

func (m model) View() string {
	if !m.ready {
		return "  Initializing..."
	}

	if m.creating {
		return m.createView()
	}

	var b strings.Builder

	// Header with aggregate stats
	var totalRuns int
	var totalCost float64
	var runningCount int
	for _, name := range m.botNames {
		stats := m.database.GetBotStats(m.botID(name))
		totalRuns += stats.Runs
		totalCost += stats.TotalCost
		running, _ := process.IsRunning(m.botID(name), m.database)
		if running {
			runningCount++
		}
	}
	b.WriteString(renderHeader(m.width, runningCount, len(m.botNames), totalRuns, totalCost))
	b.WriteString("\n\n")

	// Bots header with label and filter
	botsLabel := panelLabelStyle
	b.WriteString(botsLabel.Render("<bots>"))
	b.WriteString(" ")
	m.filter.Width = m.width/3 - 8
	if m.filter.Width < 8 {
		m.filter.Width = 8
	}
	if m.filterFocused && m.headerSlot == 1 {
		b.WriteString(m.filter.View())
	} else if m.filter.Value() != "" {
		b.WriteString(filterStyle.Render(m.filter.Value()))
	} else {
		b.WriteString(helpStyle.Render("filter..."))
	}

	// Pagination indicator when total rows exceed visible window
	total := len(m.allRows)
	if total > maxVisibleRows {
		first := m.tableOffset + 1
		last := m.tableOffset + maxVisibleRows
		if last > total {
			last = total
		}
		indicator := headerVersionStyle.Render(fmt.Sprintf("%d-%d of %d", first, last, total))
		labelW := lipgloss.Width(botsLabel.Render("<bots>")) + 1
		var filterW int
		if m.filterFocused && m.headerSlot == 1 {
			filterW = lipgloss.Width(m.filter.View())
		} else if m.filter.Value() != "" {
			filterW = lipgloss.Width(filterStyle.Render(m.filter.Value()))
		} else {
			filterW = lipgloss.Width(helpStyle.Render("filter..."))
		}
		pad := m.width - labelW - filterW - lipgloss.Width(indicator)
		if pad < 1 {
			pad = 1
		}
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(indicator)
	}
	b.WriteString("\n")

	// Table — post-process to highlight active-log bot row with purple
	// and inject filter input into the header NAME column
	b.WriteString(m.styledTableView())

	// Logs section
	logsLabel := panelLabelStyle
	b.WriteString("\n\n")
	b.WriteString(logsLabel.Render("<logs>"))
	if logBot := m.cursorBotName(); logBot != "" {
		bot := m.botCache[logBot]
		if bot != nil && bot.Config.MaxTurns > 0 {
			turns := m.database.LatestRunTurns(m.botID(logBot))
			turnInfo := headerVersionStyle.Render(fmt.Sprintf("turn %d/%d", turns, bot.Config.MaxTurns))
			b.WriteString(" " + turnInfo)
		}
	}
	b.WriteString("\n")
	b.WriteString(m.logView.View())

	// Footer — two rows: message input + help bar
	b.WriteString("\n\n")

	// Row 1: message input (only when a bot is selected)
	name := m.cursorBotName()
	if m.resuming {
		prompt := helpStyle.Render(fmt.Sprintf("resume %s for", name))
		b.WriteString(prompt + " " + m.resumeInput.View() + " " + helpStyle.Render("turns"))
		b.WriteString("\n")
	} else if name != "" {
		prompt := helpStyle.Render(fmt.Sprintf("message %s:", name))
		promptW := lipgloss.Width(prompt) + 1 // +1 for space
		m.tellInput.Width = m.width - promptW - 1
		if m.tellInput.Width < 10 {
			m.tellInput.Width = 10
		}
		b.WriteString(prompt + " " + m.tellInput.View())
		b.WriteString("\n")
	}

	// Row 2: help bar
	startStop := "s:start"
	if name := m.cursorBotName(); name != "" {
		if running, _ := process.IsRunning(m.botID(name), m.database); running {
			startStop = "s:stop"
		}
	}
	helpText := startStop + "  r:resume  c:clear  d:delete  n:new  o:open  f:filter  q:quit"
	if m.confirmDelete {
		helpText = fmt.Sprintf("delete %s? y:confirm  n:cancel", m.deleteTarget)
	} else if m.tellInput.Focused() {
		helpText = "enter:send  esc:cancel"
	} else if m.filterFocused {
		helpText = "type to filter  ←→:column  enter:sort  ↓/esc:back  ctrl+c:quit"
	}
	help := helpStyle.Render(helpText)

	// Right side: new log lines indicator or notification
	rightText := ""
	if m.newLogLines > 0 {
		rightText = notifyStyle.Render(fmt.Sprintf("+%d new lines", m.newLogLines))
	} else if m.notify != "" && time.Since(m.notifyTime) < 5*time.Second {
		if m.notifyErr {
			rightText = errorStyle.Render(m.notify)
		} else {
			rightText = notifyStyle.Render(m.notify)
		}
	}

	if rightText != "" {
		pad := m.width - lipgloss.Width(help) - lipgloss.Width(rightText)
		if pad < 1 {
			pad = 1
		}
		b.WriteString(help + strings.Repeat(" ", pad) + rightText)
	} else {
		b.WriteString(help)
	}

	return b.String()
}

// --- Helpers ---

func timeAgo(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.999999", iso)
		if err != nil {
			return iso
		}
	}

	delta := time.Since(t)
	secs := int(delta.Seconds())
	if secs < 0 {
		return "just now"
	}
	if secs < 60 {
		return fmt.Sprintf("%ds ago", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm ago", mins)
	}
	hours := mins / 60
	if hours < 24 {
		m := mins % 60
		if m > 0 {
			return fmt.Sprintf("%dh %dm ago", hours, m)
		}
		return fmt.Sprintf("%dh ago", hours)
	}
	days := hours / 24
	h := hours % 24
	if h > 0 {
		return fmt.Sprintf("%dd %dh ago", days, h)
	}
	return fmt.Sprintf("%dd ago", days)
}

// parseNumeric extracts a float from strings like "$1.23", "42", etc.
func parseNumeric(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimSuffix(s, "%")
	return strconv.ParseFloat(s, 64)
}

// --- Header rendering ---

const asciiLogo = ` ___  ___ _____ ___ _____ _
| _ )/ _ \_   _/ __|_   _| |
| _ \ (_) || || (__  | | | |__
|___/\___/ |_| \___| |_| |____|`

const asciiLogoHeight = 4

func headerLines(width int) int {
	if width >= 35 {
		return asciiLogoHeight
	}
	return 1
}

func renderHeader(width, runningCount, totalBots, totalRuns int, totalCost float64) string {
	statsLines := []string{
		fmt.Sprintf("bots: %d/%d", runningCount, totalBots),
		fmt.Sprintf("runs: %d", totalRuns),
		fmt.Sprintf("cost: $%.2f", totalCost),
	}

	if width >= 35 {
		lines := strings.Split(asciiLogo, "\n")
		var out strings.Builder
		for i, line := range lines {
			left := headerLabelStyle.Render(line)
			// Right-align stats on the last N lines of the logo
			statsIdx := i - (len(lines) - len(statsLines))
			if statsIdx >= 0 && statsIdx < len(statsLines) {
				right := headerVersionStyle.Render(statsLines[statsIdx])
				pad := width - lipgloss.Width(left) - lipgloss.Width(right)
				if pad < 1 {
					pad = 1
				}
				out.WriteString(left + strings.Repeat(" ", pad) + right)
			} else {
				out.WriteString(left)
			}
			if i < len(lines)-1 {
				out.WriteString("\n")
			}
		}
		return out.String()
	}

	left := headerLabelStyle.Render("BOTCTL")
	right := headerVersionStyle.Render(statsLines[0] + "  " + statsLines[1] + "  " + statsLines[2])
	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// Run launches the TUI.
func Run() error {
	database, err := db.Open()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	p := tea.NewProgram(
		newModel(database),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err = p.Run()
	return err
}
