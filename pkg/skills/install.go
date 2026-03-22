package skills

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InstallOpts configures where and what to install.
type InstallOpts struct {
	SkillFilter string // install only this skill name (empty = all)
	DestDir     string // target directory for installed skills
}

// Install clones a GitHub repo, discovers skills, and copies them to destDir.
// Returns the list of installed skill names.
func Install(source string, opts InstallOpts) ([]string, error) {
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("source must be in owner/repo format, got %q", source)
	}

	repoURL := "https://github.com/" + source + ".git"

	tmpDir, err := os.MkdirTemp("", "botctl-skills-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, tmpDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone %s failed: %w", source, err)
	}

	found := discoverInClone(tmpDir)
	if len(found) == 0 {
		return nil, fmt.Errorf("no skills found in %s", source)
	}

	if opts.SkillFilter != "" {
		var filtered []cloneSkill
		for _, s := range found {
			if s.name == opts.SkillFilter {
				filtered = append(filtered, s)
				break
			}
		}
		if len(filtered) == 0 {
			var names []string
			for _, s := range found {
				names = append(names, s.name)
			}
			return nil, fmt.Errorf("skill %q not found in %s (available: %s)", opts.SkillFilter, source, strings.Join(names, ", "))
		}
		found = filtered
	}

	if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
		return nil, fmt.Errorf("create destination dir: %w", err)
	}

	var installed []string
	for _, s := range found {
		dest := filepath.Join(opts.DestDir, s.name)
		if err := copyDir(s.dir, dest); err != nil {
			return installed, fmt.Errorf("copy skill %s: %w", s.name, err)
		}
		installed = append(installed, s.name)
	}

	return installed, nil
}

// SkillPreview holds the content and file listing for a skill in a cloned repo.
type SkillPreview struct {
	Name    string   // skill name from frontmatter
	Content []byte   // raw SKILL.md content
	Files   []string // other files/dirs in the skill directory (excludes SKILL.md)
}

// Preview clones a GitHub repo and returns the preview for the named skill.
func Preview(source, skillName string) (*SkillPreview, error) {
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("source must be in owner/repo format, got %q", source)
	}

	repoURL := "https://github.com/" + source + ".git"

	tmpDir, err := os.MkdirTemp("", "botctl-skills-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, tmpDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone %s failed: %w", source, err)
	}

	found := discoverInClone(tmpDir)
	if len(found) == 0 {
		return nil, fmt.Errorf("no skills found in %s", source)
	}

	var match *cloneSkill
	for i := range found {
		if found[i].name == skillName {
			match = &found[i]
			break
		}
	}
	if match == nil {
		var names []string
		for _, s := range found {
			names = append(names, s.name)
		}
		return nil, fmt.Errorf("skill %q not found in %s (available: %s)", skillName, source, strings.Join(names, ", "))
	}

	content, err := os.ReadFile(filepath.Join(match.dir, "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}

	var files []string
	if entries, err := os.ReadDir(match.dir); err == nil {
		for _, e := range entries {
			if e.Name() == "SKILL.md" {
				continue
			}
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			files = append(files, name)
		}
	}

	return &SkillPreview{Name: match.name, Content: content, Files: files}, nil
}

type cloneSkill struct {
	name string // from SKILL.md frontmatter
	dir  string // absolute path to the skill directory in the clone
}

// discoverInClone searches a cloned repo for SKILL.md files in conventional
// locations, then falls back to a recursive walk (max 5 levels deep).
func discoverInClone(root string) []cloneSkill {
	seen := make(map[string]bool)
	var result []cloneSkill

	// Search conventional paths first.
	candidates := []string{
		root,
		filepath.Join(root, "skills"),
		filepath.Join(root, ".claude", "skills"),
		filepath.Join(root, ".agents", "skills"),
	}

	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), "_") || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			skillPath := filepath.Join(dir, e.Name(), "SKILL.md")
			s, err := parseLoose(skillPath)
			if err != nil {
				continue
			}
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			result = append(result, cloneSkill{name: s.Name, dir: filepath.Join(dir, e.Name())})
		}
	}

	if len(result) > 0 {
		return result
	}

	// Fallback: recursive walk, max 5 levels deep.
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip .git, _-prefixed, and depth check.
		name := d.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			if strings.Count(rel, string(filepath.Separator)) >= 5 {
				return filepath.SkipDir
			}
			return nil
		}
		if name != "SKILL.md" {
			return nil
		}
		dirPath := filepath.Dir(path)
		s, parseErr := parseLoose(path)
		if parseErr != nil {
			return nil
		}
		if seen[s.Name] {
			return nil
		}
		seen[s.Name] = true
		result = append(result, cloneSkill{name: s.Name, dir: dirPath})
		return nil
	})

	return result
}

// copyDir copies a directory tree, skipping .git, README.md, metadata.json,
// and _-prefixed entries.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, e := range entries {
		name := e.Name()
		if name == ".git" || name == "README.md" || name == "metadata.json" || strings.HasPrefix(name, "_") {
			continue
		}

		srcPath := filepath.Join(src, name)
		dstPath := filepath.Join(dst, name)

		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
