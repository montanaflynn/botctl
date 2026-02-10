package tui

import (
	"strings"

	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/logs"
)

const maxLogLines = 2000

// logPanel tracks structured log entries via DB polling for one bot.
type logPanel struct {
	botName    string // folder name — used for display
	dbKey      string // stable DB key — used for queries
	database   *db.DB
	lines      []string
	lastSeenID int64 // highest log_entries.id seen so far
}

// newLogPanel creates a panel and loads initial content from the DB.
func newLogPanel(name, dbKey string, width int, database *db.DB) *logPanel {
	p := &logPanel{botName: name, dbKey: dbKey, database: database}
	p.loadInitial()
	return p
}

// loadInitial loads recent log entries from the DB.
func (p *logPanel) loadInitial() {
	entries := p.database.RecentLogEntries(p.dbKey, maxLogLines)
	if len(entries) == 0 {
		return
	}

	for _, e := range entries {
		rendered := logs.RenderEntry(e)
		p.lines = append(p.lines, rendered...)
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
	if p.database == nil {
		return false
	}

	entries := p.database.LogEntriesAfter(p.dbKey, p.lastSeenID, 500)
	if len(entries) == 0 {
		return false
	}

	for _, e := range entries {
		rendered := logs.RenderEntry(e)
		p.lines = append(p.lines, rendered...)
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
	if p.database != nil {
		id, _ := p.database.InsertLogEntry(p.dbKey, 0, "event", text, "")
		if id > p.lastSeenID {
			p.lastSeenID = id
		}
	}
	p.lines = append(p.lines, "<event type=\""+text+"\">")
	if len(p.lines) > maxLogLines {
		p.lines = p.lines[len(p.lines)-maxLogLines:]
	}
}

// content returns the log content as a single string.
func (p *logPanel) content() string {
	return strings.Join(p.lines, "\n")
}
