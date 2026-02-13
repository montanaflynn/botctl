package service

import (
	"fmt"

	"github.com/montanaflynn/botctl/pkg/process"
)

// SendMessage enqueues a message for a bot and wakes it (or starts it if stopped).
// Works in all states: auto-starts stopped bots, auto-resumes paused bots.
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
	s.db.SetBotState(id, "running")
	s.db.InsertLogEntry(id, 0, "event", fmt.Sprintf("started (pid %d)", newPid), "")
	return fmt.Sprintf("started (pid %d)", newPid), nil
}

// Resume enqueues a "resume:N" message and wakes or starts the bot.
// Kept for backward compatibility — delegates to PlayBot.
func (s *Service) Resume(name string, turns int) (string, error) {
	pid, err := s.PlayBot(name, turns)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("play sent (pid %d, %d turns)", pid, turns), nil
}
