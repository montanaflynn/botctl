package service

import (
	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/logs"
)

// RecentLogs returns recent rendered log lines for a bot.
func (s *Service) RecentLogs(botID string, limit int) []string {
	return logs.RecentLines(botID, limit, s.db)
}

// RecentLogEntries returns the newest N structured log entries for a bot, oldest-first.
func (s *Service) RecentLogEntries(botID string, limit int) []db.LogEntry {
	return s.db.RecentLogEntries(botID, limit)
}

// LogEntriesAfter returns log entries for a bot with id > afterID, oldest-first.
func (s *Service) LogEntriesAfter(botID string, afterID int64, limit int) []db.LogEntry {
	return s.db.LogEntriesAfter(botID, afterID, limit)
}

// LogEvent inserts a UI event log entry (not associated with a run).
func (s *Service) LogEvent(botID, heading string) (int64, error) {
	return s.db.InsertLogEntry(botID, 0, "event", heading, "")
}

// RenderLogEntry delegates to logs.RenderEntry.
func (s *Service) RenderLogEntry(entry db.LogEntry) []string {
	return logs.RenderEntry(entry)
}

// LatestRunTurns returns the turn count for a bot's most recent run.
func (s *Service) LatestRunTurns(botID string) int {
	return s.db.LatestRunTurns(botID)
}

// GetBotStats returns raw stats for a bot by its DB key.
func (s *Service) GetBotStats(botID string) BotStats {
	bs := s.db.GetBotStats(botID)
	return BotStats{
		Runs:       bs.Runs,
		TotalTurns: bs.TotalTurns,
		TotalCost:  bs.TotalCost,
		LastRun:    bs.LastRun,
	}
}
