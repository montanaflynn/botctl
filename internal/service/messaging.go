package service

import (
	"fmt"

	"github.com/montanaflynn/botctl/internal/process"
)

// SendMessage enqueues a message for a bot and wakes it (or starts it if stopped).
// Returns the action taken: "queued" if woken, or "started (pid N)" if started.
func (s *Service) SendMessage(name, message string) (string, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return "", err
	}
	id := botID(bot)

	if err := s.db.EnqueueMessage(id, message); err != nil {
		return "", fmt.Errorf("enqueue message: %w", err)
	}

	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("message: %s", message), "")

	running, pid := process.IsRunning(id, s.db)
	if running {
		process.WakeProcess(pid)
		return "queued", nil
	}

	// Start bot to pick up message
	newPid, err := process.StartBot(name, bot.Path, bot.Config, false, s.db)
	if err != nil {
		return "", fmt.Errorf("message queued but failed to start: %w", err)
	}
	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("started (pid %d)", newPid), "")
	return fmt.Sprintf("started (pid %d)", newPid), nil
}

// Resume enqueues a "resume:N" message and wakes or starts the bot.
// Returns the action taken.
func (s *Service) Resume(name string, turns int) (string, error) {
	bot, err := s.findBot(name)
	if err != nil {
		return "", err
	}
	id := botID(bot)

	msg := fmt.Sprintf("resume:%d", turns)
	if err := s.db.EnqueueMessage(id, msg); err != nil {
		return "", fmt.Errorf("enqueue resume: %w", err)
	}

	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("resume for %d turns", turns), "")

	running, pid := process.IsRunning(id, s.db)
	if running {
		process.WakeProcess(pid)
		return fmt.Sprintf("resume sent (%d turns)", turns), nil
	}

	newPid, err := process.StartBot(name, bot.Path, bot.Config, false, s.db)
	if err != nil {
		return "", fmt.Errorf("resume queued but failed to start: %w", err)
	}
	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("started (pid %d)", newPid), "")
	return fmt.Sprintf("resume sent, bot started (pid %d)", newPid), nil
}
