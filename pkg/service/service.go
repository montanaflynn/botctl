package service

import (
	"errors"

	"github.com/montanaflynn/botctl/pkg/config"
	"github.com/montanaflynn/botctl/pkg/db"
)

// Service encapsulates all bot operations, providing a shared business logic
// layer for CLI, TUI, and Web UIs.
type Service struct {
	db *db.DB
}

// New creates a new Service backed by the given database.
func New(database *db.DB) *Service {
	return &Service{db: database}
}

// DB returns the underlying database for cases that need direct access
// (e.g. harness, migrations).
func (s *Service) DB() *db.DB {
	return s.db
}

// BotInfo holds enriched bot information suitable for display.
type BotInfo struct {
	Name   string
	ID     string
	Path   string
	Status string // "stopped" | "running" | "sleeping" | "paused"
	PID    int
	Config *config.BotConfig
	Stats  BotStats
}

// BotStats holds aggregated stats for a single bot.
type BotStats struct {
	Runs       int
	TotalTurns int
	TotalCost  float64
	LastRun    string
}

// AggregateStats holds stats across all bots.
type AggregateStats struct {
	TotalBots    int
	ActiveBots   int // running + sleeping + paused
	RunningBots  int
	SleepingBots int
	PausedBots   int
	TotalRuns    int
	TotalTurns   int
	TotalCost    float64
}

// IsActive returns true if a bot status is not stopped.
func IsActive(status string) bool {
	return status == "running" || status == "sleeping" || status == "paused"
}

// Sentinel errors.
var (
	ErrBotNotFound    = errors.New("bot not found")
	ErrAlreadyRunning = errors.New("bot is already running")
	ErrAlreadyActive  = errors.New("bot is already active")
	ErrNotRunning     = errors.New("bot is not running")
	ErrNotActive      = errors.New("bot is not active")
	ErrNotPaused      = errors.New("bot is not paused")
)
