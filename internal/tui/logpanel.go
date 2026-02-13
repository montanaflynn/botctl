package tui

import (
	"strings"

	"github.com/montanaflynn/botctl/pkg/service"
)

const maxLogLines = 2000

// logPanel tracks structured log entries via DB polling for one bot.
type logPanel struct {
	botName    string // folder name -- used for display
	dbKey      string // stable DB key -- used for queries
	svc        *service.Service
	lines      []string
	lastSeenID int64 // highest log_entries.id seen so far
}

// newLogPanel creates a panel and loads initial content from the DB.
func newLogPanel(name, dbKey string, width int, svc *service.Service) *logPanel {
	p := &logPanel{botName: name, dbKey: dbKey, svc: svc}
	p.loadInitial()
	return p
}

// loadInitial loads recent log entries from the DB.
func (p *logPanel) loadInitial() {
	entries := p.svc.RecentLogEntries(p.dbKey, maxLogLines)
	if len(entries) == 0 {
		return
	}

	for _, e := range entries {
		rendered := p.svc.RenderLogEntry(e)
		p.lines = append(p.lines, rendered...)
		p.lines = append(p.lines, "") // entry separator
		if e.ID > p.lastSeenID {
			p.lastSeenID = e.ID
		}
	}

	// Trim to max lines
	if len(p.lines) > maxLogLines {
		p.lines = p.lines[len(p.lines)-maxLogLines:]
	}
}

// refresh polls the DB for new entries since lastSeenID. Returns true if new lines were added.
func (p *logPanel) refresh() bool {
	if p.svc == nil {
		return false
	}

	entries := p.svc.LogEntriesAfter(p.dbKey, p.lastSeenID, 500)
	if len(entries) == 0 {
		return false
	}

	for _, e := range entries {
		rendered := p.svc.RenderLogEntry(e)
		p.lines = append(p.lines, rendered...)
		p.lines = append(p.lines, "") // entry separator
		if e.ID > p.lastSeenID {
			p.lastSeenID = e.ID
		}
	}

	// Keep max lines
	if len(p.lines) > maxLogLines {
		p.lines = p.lines[len(p.lines)-maxLogLines:]
	}

	return true
}

// appendEvent inserts an event into the DB and appends it locally.
func (p *logPanel) appendEvent(text string) {
	if p.svc != nil {
		id, _ := p.svc.LogEvent(p.dbKey, text)
		if id > p.lastSeenID {
			p.lastSeenID = id
		}
	}
	p.lines = append(p.lines, "<event type=\""+text+"\">", "")
	if len(p.lines) > maxLogLines {
		p.lines = p.lines[len(p.lines)-maxLogLines:]
	}
}

// content returns the log content as a single string.
func (p *logPanel) content() string {
	return strings.Join(p.lines, "\n")
}
