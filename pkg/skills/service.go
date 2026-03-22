package skills

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/montanaflynn/botctl/pkg/config"
	"github.com/montanaflynn/botctl/pkg/paths"
)

// SkillsService provides operations for listing, searching, viewing,
// installing, and removing skills. It is safe to use from CLI, TUI,
// and web UI without duplicating business logic.
type SkillsService struct{}

// NewService returns a new SkillsService.
func NewService() *SkillsService {
	return &SkillsService{}
}

// SearchResult holds a single result from the skills.sh search API.
type SearchResult struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Installs int    `json:"installs"`
}

// ViewResult holds the content and metadata for viewing a skill.
type ViewResult struct {
	Name    string   `json:"name"`
	Content string   `json:"content"`
	Files   []string `json:"files"`
	Source  string   `json:"source"` // "local" or "owner/repo"
}

// List discovers all installed skills. If botFilter is non-empty, only
// bot-specific skills for that bot are included (in addition to shared dirs).
// If botFilter is empty, skills from all bots are included.
func (s *SkillsService) List(botFilter string) []Skill {
	dirs := s.discoverDirs(botFilter)
	return DiscoverAll(dirs)
}

// Search queries the skills.sh API for community skills matching query.
func (s *SkillsService) Search(query string, limit int) ([]SearchResult, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	u := fmt.Sprintf("https://skills.sh/api/search?q=%s&limit=%d", url.QueryEscape(query), limit)
	resp, err := client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	var result struct {
		Skills []SearchResult `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result.Skills, nil
}

// View returns the content and file listing for a skill. It checks local
// installations first, then falls back to fetching from skills.sh / GitHub.
// The repo parameter (owner/repo) can be provided to skip the API lookup.
func (s *SkillsService) View(name, repo string) (*ViewResult, error) {
	source := strings.TrimPrefix(strings.TrimPrefix(repo, "https://"), "github.com/")

	// Try local first.
	dirs := s.discoverDirs("")
	found := DiscoverAll(dirs)

	var match *Skill
	for i := range found {
		if found[i].Name == name {
			match = &found[i]
			break
		}
	}

	if match != nil {
		data, err := os.ReadFile(match.Path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", match.Path, err)
		}

		var files []string
		skillDir := filepath.Dir(match.Path)
		if entries, err := os.ReadDir(skillDir); err == nil {
			for _, e := range entries {
				if e.Name() == "SKILL.md" {
					continue
				}
				n := e.Name()
				if e.IsDir() {
					n += "/"
				}
				files = append(files, n)
			}
		}

		return &ViewResult{
			Name:    name,
			Content: string(data),
			Files:   files,
			Source:  "local",
		}, nil
	}

	// Not installed locally — resolve source via skills.sh if needed.
	if source == "" {
		results, err := s.Search(name, 5)
		if err != nil {
			return nil, fmt.Errorf("skill %q not found locally and search failed: %w", name, err)
		}
		for _, r := range results {
			if r.Name == name {
				source = r.Source
				break
			}
		}
		if source == "" {
			return nil, fmt.Errorf("skill %q not found locally or on skills.sh", name)
		}
	}

	preview, err := Preview(source, name)
	if err != nil {
		return nil, err
	}

	return &ViewResult{
		Name:    name,
		Content: string(preview.Content),
		Files:   preview.Files,
		Source:  source,
	}, nil
}

// Remove deletes all local installations of the named skill and returns
// the directories that were removed.
func (s *SkillsService) Remove(name string) ([]string, error) {
	dirs := s.discoverDirs("")
	found := DiscoverAll(dirs)

	var matches []Skill
	for _, sk := range found {
		if sk.Name == name {
			matches = append(matches, sk)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("skill %q not found", name)
	}

	var removed []string
	for _, sk := range matches {
		skillDir := filepath.Dir(sk.Path)
		if err := os.RemoveAll(skillDir); err != nil {
			return removed, fmt.Errorf("remove %s: %w", skillDir, err)
		}
		removed = append(removed, skillDir)
	}
	return removed, nil
}

// Install clones a GitHub repo and copies skills into destDir.
// It delegates to the package-level Install function.
func (s *SkillsService) Install(source string, opts InstallOpts) ([]string, error) {
	return Install(source, opts)
}

// discoverDirs builds the list of directories to scan for skills.
// If botFilter is non-empty, only that bot's skills_dir is included.
// If botFilter is empty, all bots' skills_dirs are included.
func (s *SkillsService) discoverDirs(botFilter string) []string {
	var dirs []string
	for _, candidate := range []string{paths.AgentsSkillsDir(), paths.GlobalSkillsDir()} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			dirs = append(dirs, candidate)
		}
	}

	var botNames []string
	if botFilter != "" {
		botNames = []string{botFilter}
	} else if entries, err := os.ReadDir(paths.BotsDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				botNames = append(botNames, e.Name())
			}
		}
	}

	for _, name := range botNames {
		botDir := filepath.Join(paths.BotsDir(), name)
		cfg, err := config.FromMD(filepath.Join(botDir, "BOT.md"))
		if err != nil || cfg.SkillsDir == "" {
			continue
		}
		skillsPath := filepath.Join(botDir, cfg.SkillsDir)
		if abs, err := filepath.Abs(skillsPath); err == nil {
			skillsPath = abs
		}
		if info, err := os.Stat(skillsPath); err == nil && info.IsDir() {
			dirs = append(dirs, skillsPath)
		}
	}

	return dirs
}
