package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// helper to create a skill directory with a SKILL.md file
func makeSkill(t *testing.T, base, dirName, content string) {
	t.Helper()
	dir := filepath.Join(base, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validSkillMD = `---
name: test-skill
description: A test skill for unit tests
---

This is the skill body.
`

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		skill   Skill
		dirName string
		wantErr string
	}{
		{
			name:    "valid skill",
			skill:   Skill{Name: "my-skill", Description: "Does things"},
			dirName: "my-skill",
		},
		{
			name:    "missing name",
			skill:   Skill{Description: "Does things"},
			dirName: "my-skill",
			wantErr: "missing required field: name",
		},
		{
			name:    "missing description",
			skill:   Skill{Name: "my-skill"},
			dirName: "my-skill",
			wantErr: "missing required field: description",
		},
		{
			name:    "name too long",
			skill:   Skill{Name: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Description: "ok"},
			dirName: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr: "name exceeds 64 characters",
		},
		{
			name:    "invalid name - uppercase",
			skill:   Skill{Name: "MySkill", Description: "ok"},
			dirName: "MySkill",
			wantErr: "must be lowercase alphanumeric",
		},
		{
			name:    "invalid name - underscores",
			skill:   Skill{Name: "my_skill", Description: "ok"},
			dirName: "my_skill",
			wantErr: "must be lowercase alphanumeric",
		},
		{
			name:    "invalid name - double hyphen",
			skill:   Skill{Name: "my--skill", Description: "ok"},
			dirName: "my--skill",
			wantErr: "must be lowercase alphanumeric",
		},
		{
			name:    "invalid name - leading hyphen",
			skill:   Skill{Name: "-skill", Description: "ok"},
			dirName: "-skill",
			wantErr: "must be lowercase alphanumeric",
		},
		{
			name:    "name does not match directory",
			skill:   Skill{Name: "foo", Description: "ok"},
			dirName: "bar",
			wantErr: "does not match directory",
		},
		{
			name:    "single word name",
			skill:   Skill{Name: "slack", Description: "ok"},
			dirName: "slack",
		},
		{
			name:    "numeric name",
			skill:   Skill{Name: "s3", Description: "ok"},
			dirName: "s3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.skill.Validate(tt.dirName)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, got)
			}
		})
	}
}

func TestParseLoose(t *testing.T) {
	tmp := t.TempDir()

	t.Run("valid SKILL.md", func(t *testing.T) {
		path := filepath.Join(tmp, "valid-SKILL.md")
		os.WriteFile(path, []byte(validSkillMD), 0o644)

		s, err := parseLoose(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != "test-skill" {
			t.Errorf("expected name 'test-skill', got %q", s.Name)
		}
		if s.Description != "A test skill for unit tests" {
			t.Errorf("expected description 'A test skill for unit tests', got %q", s.Description)
		}
		if s.Path != path {
			t.Errorf("expected path %q, got %q", path, s.Path)
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		path := filepath.Join(tmp, "no-fm.md")
		os.WriteFile(path, []byte("just some text"), 0o644)

		_, err := parseLoose(path)
		if err == nil {
			t.Error("expected error for missing frontmatter")
		}
	})

	t.Run("missing name field", func(t *testing.T) {
		path := filepath.Join(tmp, "no-name.md")
		os.WriteFile(path, []byte("---\ndescription: something\n---\nbody"), 0o644)

		_, err := parseLoose(path)
		if err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := parseLoose(filepath.Join(tmp, "nonexistent.md"))
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := filepath.Join(tmp, "bad-yaml.md")
		os.WriteFile(path, []byte("---\n: :\n---\nbody"), 0o644)

		_, err := parseLoose(path)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})
}

func TestParse(t *testing.T) {
	tmp := t.TempDir()

	t.Run("valid with matching dir name", func(t *testing.T) {
		makeSkill(t, tmp, "test-skill", validSkillMD)
		s, err := parse(filepath.Join(tmp, "test-skill", "SKILL.md"), "test-skill")
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != "test-skill" {
			t.Errorf("expected 'test-skill', got %q", s.Name)
		}
	})

	t.Run("name does not match dir", func(t *testing.T) {
		makeSkill(t, tmp, "wrong-dir", validSkillMD)
		_, err := parse(filepath.Join(tmp, "wrong-dir", "SKILL.md"), "wrong-dir")
		if err == nil {
			t.Error("expected error when name doesn't match directory")
		}
	})
}

func TestDiscover(t *testing.T) {
	tmp := t.TempDir()
	dir1 := filepath.Join(tmp, "shared")
	dir2 := filepath.Join(tmp, "bot")

	os.MkdirAll(dir1, 0o755)
	os.MkdirAll(dir2, 0o755)

	makeSkill(t, dir1, "alpha", "---\nname: alpha\ndescription: first\n---\n")
	makeSkill(t, dir1, "beta", "---\nname: beta\ndescription: second\n---\n")
	makeSkill(t, dir2, "alpha", "---\nname: alpha\ndescription: override\n---\n")
	makeSkill(t, dir2, "gamma", "---\nname: gamma\ndescription: third\n---\n")

	t.Run("deduplicates with first-wins precedence", func(t *testing.T) {
		skills := Discover([]string{dir1, dir2})

		names := make(map[string]string)
		for _, s := range skills {
			names[s.Name] = s.Description
		}

		if len(skills) != 3 {
			t.Errorf("expected 3 unique skills, got %d", len(skills))
		}
		if names["alpha"] != "first" {
			t.Errorf("expected alpha from dir1 (first-wins), got description %q", names["alpha"])
		}
		if _, ok := names["beta"]; !ok {
			t.Error("expected beta to be present")
		}
		if _, ok := names["gamma"]; !ok {
			t.Error("expected gamma to be present")
		}
	})

	t.Run("DiscoverAll returns duplicates", func(t *testing.T) {
		all := DiscoverAll([]string{dir1, dir2})

		alphaCount := 0
		for _, s := range all {
			if s.Name == "alpha" {
				alphaCount++
			}
		}
		if alphaCount != 2 {
			t.Errorf("expected 2 alpha entries, got %d", alphaCount)
		}
	})

	t.Run("nonexistent directory is skipped", func(t *testing.T) {
		skills := Discover([]string{filepath.Join(tmp, "nope"), dir1})
		if len(skills) != 2 {
			t.Errorf("expected 2 skills from dir1, got %d", len(skills))
		}
	})

	t.Run("empty dirs returns nil", func(t *testing.T) {
		skills := Discover(nil)
		if skills != nil {
			t.Errorf("expected nil, got %v", skills)
		}
	})
}

func TestDiscoverSkipsInvalid(t *testing.T) {
	tmp := t.TempDir()

	// Valid skill
	makeSkill(t, tmp, "good", "---\nname: good\ndescription: valid\n---\n")

	// Invalid: no frontmatter
	os.MkdirAll(filepath.Join(tmp, "bad"), 0o755)
	os.WriteFile(filepath.Join(tmp, "bad", "SKILL.md"), []byte("no frontmatter"), 0o644)

	// Invalid: file instead of directory
	os.WriteFile(filepath.Join(tmp, "not-a-dir"), []byte("file"), 0o644)

	// Invalid: directory without SKILL.md
	os.MkdirAll(filepath.Join(tmp, "empty-dir"), 0o755)

	skills := Discover([]string{tmp})
	if len(skills) != 1 {
		t.Errorf("expected 1 valid skill, got %d", len(skills))
	}
	if skills[0].Name != "good" {
		t.Errorf("expected 'good', got %q", skills[0].Name)
	}
}

func TestDiscoverSetsSource(t *testing.T) {
	tmp := t.TempDir()
	makeSkill(t, tmp, "src-test", "---\nname: src-test\ndescription: ok\n---\n")

	skills := Discover([]string{tmp})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Source != tmp {
		t.Errorf("expected source %q, got %q", tmp, skills[0].Source)
	}
}

func TestFormatPrompt(t *testing.T) {
	t.Run("empty skills", func(t *testing.T) {
		result := FormatPrompt(nil)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("single skill", func(t *testing.T) {
		skills := []Skill{{Name: "slack", Description: "Send Slack messages"}}
		result := FormatPrompt(skills)

		if !contains(result, "slack: Send Slack messages") {
			t.Errorf("expected skill listing, got %q", result)
		}
		if !contains(result, "The following skills are available") {
			t.Errorf("expected header, got %q", result)
		}
	})

	t.Run("multiple skills", func(t *testing.T) {
		skills := []Skill{
			{Name: "alpha", Description: "First"},
			{Name: "beta", Description: "Second"},
		}
		result := FormatPrompt(skills)

		if !contains(result, "- alpha: First") {
			t.Errorf("missing alpha in %q", result)
		}
		if !contains(result, "- beta: Second") {
			t.Errorf("missing beta in %q", result)
		}
	})
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dest")

	// Create source structure
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("skill content"), 0o644)
	os.WriteFile(filepath.Join(src, "helper.sh"), []byte("#!/bin/sh"), 0o644)
	os.MkdirAll(filepath.Join(src, "templates"), 0o755)
	os.WriteFile(filepath.Join(src, "templates", "base.txt"), []byte("template"), 0o644)

	// Files that should be skipped
	os.WriteFile(filepath.Join(src, "README.md"), []byte("readme"), 0o644)
	os.WriteFile(filepath.Join(src, "metadata.json"), []byte("{}"), 0o644)
	os.MkdirAll(filepath.Join(src, ".git"), 0o755)
	os.WriteFile(filepath.Join(src, ".git", "config"), []byte("git"), 0o644)
	os.MkdirAll(filepath.Join(src, "_drafts"), 0o755)
	os.WriteFile(filepath.Join(src, "_drafts", "draft.md"), []byte("draft"), 0o644)

	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}

	// Verify copied files
	for _, path := range []string{
		"SKILL.md",
		"helper.sh",
		"templates/base.txt",
	} {
		if _, err := os.Stat(filepath.Join(dst, path)); err != nil {
			t.Errorf("expected %s to be copied, got %v", path, err)
		}
	}

	// Verify skipped files
	for _, path := range []string{
		"README.md",
		"metadata.json",
		".git",
		"_drafts",
	} {
		if _, err := os.Stat(filepath.Join(dst, path)); err == nil {
			t.Errorf("expected %s to be skipped, but it exists", path)
		}
	}

	// Verify content
	data, _ := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if string(data) != "skill content" {
		t.Errorf("expected 'skill content', got %q", string(data))
	}
}

func TestDiscoverInClone(t *testing.T) {
	tmp := t.TempDir()

	t.Run("finds skills in root-level directories", func(t *testing.T) {
		root := filepath.Join(tmp, "repo1")
		makeSkill(t, root, "my-tool", "---\nname: my-tool\ndescription: A tool\n---\n")

		found := discoverInClone(root)
		if len(found) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(found))
		}
		if found[0].name != "my-tool" {
			t.Errorf("expected 'my-tool', got %q", found[0].name)
		}
	})

	t.Run("finds skills in skills/ subdirectory", func(t *testing.T) {
		root := filepath.Join(tmp, "repo2")
		os.MkdirAll(root, 0o755)
		makeSkill(t, filepath.Join(root, "skills"), "checker", "---\nname: checker\ndescription: Checks things\n---\n")

		found := discoverInClone(root)
		if len(found) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(found))
		}
		if found[0].name != "checker" {
			t.Errorf("expected 'checker', got %q", found[0].name)
		}
	})

	t.Run("deduplicates across conventional paths", func(t *testing.T) {
		root := filepath.Join(tmp, "repo3")
		makeSkill(t, root, "dupe", "---\nname: dupe\ndescription: In root\n---\n")
		makeSkill(t, filepath.Join(root, "skills"), "dupe", "---\nname: dupe\ndescription: In skills/\n---\n")

		found := discoverInClone(root)
		if len(found) != 1 {
			t.Errorf("expected 1 deduplicated skill, got %d", len(found))
		}
	})

	t.Run("skips dot-prefixed and underscore-prefixed dirs", func(t *testing.T) {
		root := filepath.Join(tmp, "repo4")
		makeSkill(t, root, "visible", "---\nname: visible\ndescription: ok\n---\n")
		makeSkill(t, root, ".hidden", "---\nname: hidden\ndescription: ok\n---\n")
		makeSkill(t, root, "_private", "---\nname: private\ndescription: ok\n---\n")

		found := discoverInClone(root)
		if len(found) != 1 {
			t.Errorf("expected 1 skill, got %d", len(found))
		}
	})

	t.Run("empty repo returns nil", func(t *testing.T) {
		root := filepath.Join(tmp, "repo5")
		os.MkdirAll(root, 0o755)

		found := discoverInClone(root)
		if len(found) != 0 {
			t.Errorf("expected 0 skills, got %d", len(found))
		}
	})
}

func TestInstallSource(t *testing.T) {
	t.Run("rejects invalid source format", func(t *testing.T) {
		_, err := Install("not-a-repo", InstallOpts{DestDir: t.TempDir()})
		if err == nil {
			t.Error("expected error for invalid source")
		}
		if !contains(err.Error(), "owner/repo format") {
			t.Errorf("expected owner/repo error, got %q", err.Error())
		}
	})

	t.Run("rejects empty owner", func(t *testing.T) {
		_, err := Install("/repo", InstallOpts{DestDir: t.TempDir()})
		if err == nil {
			t.Error("expected error for empty owner")
		}
	})

	t.Run("rejects empty repo", func(t *testing.T) {
		_, err := Install("owner/", InstallOpts{DestDir: t.TempDir()})
		if err == nil {
			t.Error("expected error for empty repo")
		}
	})
}

func TestNameRegex(t *testing.T) {
	valid := []string{"a", "abc", "my-skill", "s3-backup", "a1b2c3"}
	invalid := []string{"", "-", "A", "my_skill", "my--skill", "-foo", "foo-", "FOO", "has space", "a.b"}

	for _, name := range valid {
		if !nameRe.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range invalid {
		if nameRe.MatchString(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
