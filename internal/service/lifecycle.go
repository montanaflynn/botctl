package service

import (
	"fmt"
	"os"

	"github.com/montanaflynn/botctl/internal/process"
)

// StartBot finds a bot by name and starts it. Returns the PID.
func (s *Service) StartBot(name string) (int, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return 0, err
	}
	id := botID(bot)
	if running, pid := process.IsRunning(id, s.db); running {
		return 0, fmt.Errorf("%s is already running (pid %d)", name, pid)
	}
	pid, err := process.StartBot(name, bot.Path, bot.Config, false, s.db)
	if err != nil {
		return 0, err
	}
	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("started (pid %d)", pid), "")
	return pid, nil
}

// StartBotWithMessage starts a bot with an initial message appended to the prompt.
func (s *Service) StartBotWithMessage(name, message string) (int, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return 0, err
	}
	pid, err := process.StartBotWithMessage(name, bot.Path, bot.Config, s.db, message)
	if err != nil {
		return 0, err
	}
	id := botID(bot)
	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("started (pid %d)", pid), "")
	return pid, nil
}

// StartBotOnce starts a bot for a single one-shot run.
func (s *Service) StartBotOnce(name, message string) (int, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return 0, err
	}
	id := botID(bot)
	// Stop if already running
	if running, _ := process.IsRunning(id, s.db); running {
		process.StopBot(id, s.db)
	}
	pid, err := process.StartBotOnce(name, bot.Path, bot.Config, s.db, message)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// StopBot finds a bot by name and stops it synchronously.
func (s *Service) StopBot(name string) error {
	bot, err := s.findBot(name)
	if err != nil {
		return err
	}
	id := botID(bot)
	if stopped := process.StopBot(id, s.db); !stopped {
		return ErrNotRunning
	}
	s.db.InsertLogEntry(id, 0, "event", "stopped", "")
	return nil
}

// StopBotByID stops a bot using its DB key directly (for TUI async flow).
func (s *Service) StopBotByID(id string) bool {
	return process.StopBot(id, s.db)
}

// IsRunning checks if a bot is running by its DB key.
func (s *Service) IsRunning(id string) (bool, int) {
	return process.IsRunning(id, s.db)
}

// RemovePID removes a stale PID record (used after TUI async kill).
func (s *Service) RemovePID(id string) {
	s.db.RemovePID(id)
}

// RestartBot stops and then starts a bot.
func (s *Service) RestartBot(name string) (int, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return 0, err
	}
	id := botID(bot)
	process.StopBot(id, s.db)
	pid, err := process.StartBot(name, bot.Path, bot.Config, false, s.db)
	if err != nil {
		return 0, err
	}
	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("started (pid %d)", pid), "")
	return pid, nil
}

// DeleteBot stops a running bot, purges its DB data, and removes its directory.
func (s *Service) DeleteBot(name string) error {
	bot, err := s.findBot(name)
	if err != nil {
		return err
	}
	id := botID(bot)

	// Stop if running
	process.StopBot(id, s.db)

	// Delete DB data
	if err := s.db.DeleteBotData(id); err != nil {
		return fmt.Errorf("delete db data: %w", err)
	}

	// Remove bot directory
	return os.RemoveAll(bot.Path)
}

// DeleteBotByID deletes DB data and directory for a bot identified by name/id/path
// (used by TUI after async stop).
func (s *Service) DeleteBotData(id string) error {
	return s.db.DeleteBotData(id)
}
