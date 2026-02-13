package service

import (
	"github.com/montanaflynn/botctl/internal/discovery"
	"github.com/montanaflynn/botctl/internal/process"
)

// ListBots discovers all bots, enriches them with status and stats,
// and optionally filters by name substring. Returns bot infos and
// any discovery error messages.
func (s *Service) ListBots(filter string) ([]BotInfo, []string) {
	bots, errs := discovery.DiscoverBots()
	var result []BotInfo
	for _, bot := range bots {
		if filter != "" && !containsLower(bot.Name, filter) {
			continue
		}
		result = append(result, s.buildBotInfo(&bot))
	}
	return result, errs
}

// GetBot looks up a single bot by folder name.
func (s *Service) GetBot(name string) (BotInfo, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return BotInfo{}, err
	}
	return s.buildBotInfo(bot), nil
}

// GetStats returns aggregate stats across all discovered bots.
func (s *Service) GetStats() AggregateStats {
	bots, _ := discovery.DiscoverBots()
	var stats AggregateStats
	stats.TotalBots = len(bots)
	for _, bot := range bots {
		id := botID(&bot)
		if running, _ := process.IsRunning(id, s.db); running {
			stats.RunningBots++
		}
		bs := s.db.GetBotStats(id)
		stats.TotalRuns += bs.Runs
		stats.TotalCost += bs.TotalCost
		stats.TotalTurns += bs.TotalTurns
	}
	return stats
}

// findBot discovers all bots and returns the one matching name, or ErrBotNotFound.
func (s *Service) findBot(name string) (*discovery.Bot, error) {
	bots, _ := discovery.DiscoverBots()
	for _, b := range bots {
		if b.Name == name {
			return &b, nil
		}
	}
	return nil, ErrBotNotFound
}

// buildBotInfo enriches a discovered bot with running status and DB stats.
func (s *Service) buildBotInfo(bot *discovery.Bot) BotInfo {
	id := botID(bot)
	running, pid := process.IsRunning(id, s.db)
	stats := s.db.GetBotStats(id)

	status := "stopped"
	if running {
		status = "running"
	}

	return BotInfo{
		Name:   bot.Name,
		ID:     id,
		Path:   bot.Path,
		Status: status,
		PID:    pid,
		Config: bot.Config,
		Stats: BotStats{
			Runs:       stats.Runs,
			TotalTurns: stats.TotalTurns,
			TotalCost:  stats.TotalCost,
			LastRun:    stats.LastRun,
		},
	}
}

// botID returns the stable DB key for a bot (config.ID or folder name).
func botID(bot *discovery.Bot) string {
	if bot.ID != "" {
		return bot.ID
	}
	return bot.Name
}

// containsLower checks if s contains substr (case-insensitive).
func containsLower(s, substr string) bool {
	// Simple ASCII-safe lowercase contains
	ls := toLower(s)
	lsub := toLower(substr)
	return len(lsub) <= len(ls) && contains(ls, lsub)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
