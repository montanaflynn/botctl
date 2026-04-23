package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var frontmatterRe = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n(.*)`)

// BotConfig holds parsed bot configuration from a BOT.md file.
type BotConfig struct {
	ID              string            `yaml:"id"`
	Name            string            `yaml:"name"`
	IntervalSeconds int               `yaml:"interval_seconds"`
	MaxTurns        int               `yaml:"max_turns"`
	Env             map[string]string `yaml:"env"`
	SkillsDir       string            `yaml:"skills_dir"`
	Workspace       string            `yaml:"workspace"`
	LogDir          string            `yaml:"log_dir"`
	LogRetention    int               `yaml:"log_retention"`
	Backend         string            `yaml:"backend"`
	Model           string            `yaml:"model"`
	Provider        string            `yaml:"provider"`
	Body            string            `yaml:"-"`
	Raw             map[string]any    `yaml:"-"`
}

// FromMD parses a BOT.md file with YAML frontmatter and a markdown body.
func FromMD(path string) (*BotConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	matches := frontmatterRe.FindSubmatch(data)
	if matches == nil {
		return nil, fmt.Errorf("no YAML frontmatter found in %s", path)
	}

	var cfg BotConfig
	if err := yaml.Unmarshal(matches[1], &cfg); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	// Also unmarshal into raw map for extra fields
	var raw map[string]any
	_ = yaml.Unmarshal(matches[1], &raw)
	cfg.Raw = raw

	cfg.Body = strings.TrimSpace(string(matches[2]))

	if cfg.IntervalSeconds == 0 {
		cfg.IntervalSeconds = 60
	}
	if cfg.LogRetention == 0 {
		cfg.LogRetention = 30
	}

	// Backfill from raw if yaml tags didn't catch them
	if cfg.SkillsDir == "" {
		if v, ok := raw["skills_dir"].(string); ok {
			cfg.SkillsDir = v
		}
	}
	if cfg.Workspace == "" {
		if v, ok := raw["workspace"].(string); ok {
			cfg.Workspace = v
		}
	}
	if cfg.LogDir == "" {
		if v, ok := raw["log_dir"].(string); ok {
			cfg.LogDir = v
		}
	}
	if cfg.LogRetention == 30 { // default — check if raw has an override
		if v, ok := raw["log_retention"].(int); ok && v > 0 {
			cfg.LogRetention = v
		}
	}

	if cfg.Backend == "opencode" && cfg.Model == "" {
		return nil, fmt.Errorf("backend=opencode requires a `model` field in frontmatter")
	}

	return &cfg, nil
}

// ResolveEnv resolves ${VAR} references in the env map from the OS environment.
func (c *BotConfig) ResolveEnv() (map[string]string, error) {
	resolved := make(map[string]string, len(c.Env))
	for k, v := range c.Env {
		if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
			envName := v[2 : len(v)-1]
			envVal := os.Getenv(envName)
			if envVal == "" {
				return nil, fmt.Errorf("environment variable %s is not set", envName)
			}
			resolved[k] = envVal
		} else {
			resolved[k] = v
		}
	}
	return resolved, nil
}
