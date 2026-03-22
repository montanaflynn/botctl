package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	frontmatterRe = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n`)
	nameRe        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

const (
	maxNameLen        = 64
	maxDescriptionLen = 1024
)

// Skill holds the parsed metadata from a SKILL.md frontmatter.
type Skill struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Path        string `yaml:"-"` // absolute path to the SKILL.md file
	Source      string `yaml:"-"` // parent directory where the skill was found
}

// Validate checks that the skill metadata meets the spec.
func (s *Skill) Validate(dirName string) error {
	if s.Name == "" {
		return fmt.Errorf("missing required field: name")
	}
	if len(s.Name) > maxNameLen {
		return fmt.Errorf("name exceeds %d characters", maxNameLen)
	}
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("name %q must be lowercase alphanumeric with single hyphen separators", s.Name)
	}
	if s.Name != dirName {
		return fmt.Errorf("name %q does not match directory %q", s.Name, dirName)
	}
	if s.Description == "" {
		return fmt.Errorf("missing required field: description")
	}
	if len(s.Description) > maxDescriptionLen {
		return fmt.Errorf("description exceeds %d characters", maxDescriptionLen)
	}
	return nil
}

// Discover scans directories for valid SKILL.md files and returns unique skills.
// Invalid skills are silently skipped. Directories are scanned in order, so earlier
// entries take precedence if duplicate names appear.
func Discover(dirs []string) []Skill {
	seen := make(map[string]bool)
	var result []Skill
	for _, s := range DiscoverAll(dirs) {
		if seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		result = append(result, s)
	}
	return result
}

// DiscoverAll scans directories for valid SKILL.md files and returns all valid skills
// including duplicates across directories.
func DiscoverAll(dirs []string) []Skill {
	var result []Skill

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dirName := entry.Name()
			skillPath := filepath.Join(dir, dirName, "SKILL.md")
			s, err := parse(skillPath, dirName)
			if err != nil {
				continue
			}
			s.Source = dir
			result = append(result, *s)
		}
	}
	return result
}

// parse reads and validates a single SKILL.md file.
func parse(path, dirName string) (*Skill, error) {
	s, err := parseLoose(path)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(dirName); err != nil {
		return nil, fmt.Errorf("skill %s: %w", path, err)
	}
	return s, nil
}

// parseLoose reads a SKILL.md file without enforcing directory name matching.
// Used for external/cloned repos where the name may not match the directory.
func parseLoose(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	matches := frontmatterRe.FindSubmatch(data)
	if matches == nil {
		return nil, fmt.Errorf("no YAML frontmatter in %s", path)
	}

	var s Skill
	if err := yaml.Unmarshal(matches[1], &s); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	s.Path = path

	if s.Name == "" {
		return nil, fmt.Errorf("missing required field: name")
	}

	return &s, nil
}

// FormatPrompt builds the system prompt snippet listing available skills.
// Returns an empty string if no skills are found.
func FormatPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following skills are available:\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return b.String()
}
