package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/montanaflynn/botctl/pkg/config"
	"github.com/montanaflynn/botctl/pkg/paths"
)

// Bot represents a discovered bot with its config.
type Bot struct {
	Name   string // folder name (display + paths)
	ID     string // stable DB key (from config id or folder name)
	Path   string
	Config *config.BotConfig
}

// DiscoverBots scans the bots directory for valid BOT.md files.
// Returns discovered bots and a list of error messages for invalid ones.
func DiscoverBots() ([]Bot, []string) {
	var bots []Bot
	var errors []string

	bd := paths.BotsDir()
	entries, err := os.ReadDir(bd)
	if err != nil {
		return bots, errors
	}

	// Sort by name for consistent ordering
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		mdPath := filepath.Join(bd, entry.Name(), "BOT.md")
		if _, err := os.Stat(mdPath); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("%s: missing BOT.md", entry.Name()))
			continue
		}

		cfg, err := config.FromMD(mdPath)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: invalid BOT.md: %v", entry.Name(), err))
			continue
		}

		if cfg.Name == "" {
			cfg.Name = entry.Name()
		}

		id := cfg.ID
		if id == "" {
			id = entry.Name()
		}

		bots = append(bots, Bot{
			Name:   entry.Name(),
			ID:     id,
			Path:   filepath.Join(bd, entry.Name()),
			Config: cfg,
		})
	}

	return bots, errors
}
