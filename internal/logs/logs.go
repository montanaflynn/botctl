package logs

import (
	"github.com/montanaflynn/botctl-go/internal/db"
)

// RecentLines returns rendered lines from a bot's recent log entries.
func RecentLines(dbKey string, lines int, database *db.DB) []string {
	entries := database.RecentLogEntries(dbKey, lines)
	if len(entries) == 0 {
		return nil
	}
	var result []string
	for _, e := range entries {
		result = append(result, RenderEntry(e)...)
	}
	// Trim to requested line count
	if len(result) > lines {
		result = result[len(result)-lines:]
	}
	return result
}
