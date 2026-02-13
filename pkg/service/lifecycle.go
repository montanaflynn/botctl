package service

import (
	"fmt"
	"os"

	"github.com/montanaflynn/botctl/pkg/process"
)

// StartBot finds a bot by name and starts it. Returns the PID.
func (s *Service) StartBot(name string) (int, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return 0, err
	}
	id := botID(bot)
	if running, pid := process.IsRunning(id, s.db); running {
		return 0, fmt.Errorf("%s is already active (pid %d)", name, pid)
	}
	pid, err := process.StartBot(name, bot.Path, bot.Config, false, s.db)
	if err != nil {
		return 0, err
	}
	s.db.SetBotState(id, "running")
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
	s.db.SetBotState(id, "running")
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
		s.db.ClearBotState(id)
	}
	pid, err := process.StartBotOnce(name, bot.Path, bot.Config, s.db, message)
	if err != nil {
		return 0, err
	}
	s.db.SetBotState(id, "running")
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
	s.db.ClearBotState(id)
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

// ClearState resets the bot state to stopped (used after TUI async kill).
func (s *Service) ClearState(id string) {
	s.db.ClearBotState(id)
}

// PauseBot pauses a running or sleeping bot.
func (s *Service) PauseBot(name string) error {
	bot, err := s.findBot(name)
	if err != nil {
		return err
	}
	id := botID(bot)
	status := s.resolveStatus(id)

	switch status {
	case "running":
		// Set pause_requested flag, send SIGUSR1 to interrupt the Claude API call
		s.db.SetPauseRequested(id, true)
		if _, pid := process.IsRunning(id, s.db); pid > 0 {
			process.WakeProcess(pid)
		}
		s.db.InsertLogEntry(id, 0, "event", "pause requested", "")
		return nil
	case "sleeping":
		// Transition directly to paused and wake the harness so it sees the state change
		s.db.SetBotState(id, "paused")
		if _, pid := process.IsRunning(id, s.db); pid > 0 {
			process.WakeProcess(pid)
		}
		s.db.InsertLogEntry(id, 0, "event", "paused", "")
		return nil
	case "paused":
		return nil // already paused
	default:
		return ErrNotActive
	}
}

// PlayBot resumes a paused bot or starts a fresh run from stopped.
// Returns the PID.
func (s *Service) PlayBot(name string, turns int) (int, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return 0, err
	}
	id := botID(bot)
	status := s.resolveStatus(id)

	switch status {
	case "paused":
		// Enqueue resume message and wake the harness
		msg := fmt.Sprintf("resume:%d", turns)
		if err := s.db.EnqueueMessage(id, msg); err != nil {
			return 0, fmt.Errorf("enqueue resume: %w", err)
		}
		s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("play for %d turns", turns), "")
		if _, pid := process.IsRunning(id, s.db); pid > 0 {
			process.WakeProcess(pid)
			return pid, nil
		}
		// Process died while paused — start fresh
		pid, err := process.StartBot(name, bot.Path, bot.Config, false, s.db)
		if err != nil {
			return 0, err
		}
		s.db.SetBotState(id, "running")
		return pid, nil
	case "stopped":
		return 0, ErrNotPaused
	default:
		return 0, ErrAlreadyActive
	}
}

// RestartBot stops and then starts a bot.
func (s *Service) RestartBot(name string) (int, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return 0, err
	}
	id := botID(bot)
	process.StopBot(id, s.db)
	s.db.ClearBotState(id)
	pid, err := process.StartBot(name, bot.Path, bot.Config, false, s.db)
	if err != nil {
		return 0, err
	}
	s.db.SetBotState(id, "running")
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

	// Stop if active
	process.StopBot(id, s.db)

	// Delete DB data (includes bot_state cleanup)
	if err := s.db.DeleteBotData(id); err != nil {
		return fmt.Errorf("delete db data: %w", err)
	}

	// Remove bot directory
	return os.RemoveAll(bot.Path)
}

// DeleteBotData deletes DB data for a bot identified by its DB key
// (used by TUI after async stop).
func (s *Service) DeleteBotData(id string) error {
	return s.db.DeleteBotData(id)
}
