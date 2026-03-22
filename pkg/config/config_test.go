package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempBotMD(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "BOT.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFromMD(t *testing.T) {
	tests := []struct {
		name    string
		content string
		check   func(t *testing.T, cfg *BotConfig)
		wantErr bool
	}{
		{
			name: "full config",
			content: `---
id: test-bot
name: Test Bot
interval_seconds: 120
max_turns: 5
workspace: shared
skills_dir: ./skills
log_dir: ./logs
log_retention: 10
env:
  API_KEY: secret123
---
You are a helpful bot.`,
			check: func(t *testing.T, cfg *BotConfig) {
				if cfg.ID != "test-bot" {
					t.Errorf("ID = %q, want %q", cfg.ID, "test-bot")
				}
				if cfg.Name != "Test Bot" {
					t.Errorf("Name = %q, want %q", cfg.Name, "Test Bot")
				}
				if cfg.IntervalSeconds != 120 {
					t.Errorf("IntervalSeconds = %d, want 120", cfg.IntervalSeconds)
				}
				if cfg.MaxTurns != 5 {
					t.Errorf("MaxTurns = %d, want 5", cfg.MaxTurns)
				}
				if cfg.Workspace != "shared" {
					t.Errorf("Workspace = %q, want %q", cfg.Workspace, "shared")
				}
				if cfg.SkillsDir != "./skills" {
					t.Errorf("SkillsDir = %q, want %q", cfg.SkillsDir, "./skills")
				}
				if cfg.LogDir != "./logs" {
					t.Errorf("LogDir = %q, want %q", cfg.LogDir, "./logs")
				}
				if cfg.LogRetention != 10 {
					t.Errorf("LogRetention = %d, want 10", cfg.LogRetention)
				}
				if cfg.Body != "You are a helpful bot." {
					t.Errorf("Body = %q, want %q", cfg.Body, "You are a helpful bot.")
				}
				if cfg.Env["API_KEY"] != "secret123" {
					t.Errorf("Env[API_KEY] = %q, want %q", cfg.Env["API_KEY"], "secret123")
				}
			},
		},
		{
			name: "defaults applied",
			content: `---
name: Minimal Bot
---
Do things.`,
			check: func(t *testing.T, cfg *BotConfig) {
				if cfg.IntervalSeconds != 60 {
					t.Errorf("IntervalSeconds = %d, want default 60", cfg.IntervalSeconds)
				}
				if cfg.LogRetention != 30 {
					t.Errorf("LogRetention = %d, want default 30", cfg.LogRetention)
				}
			},
		},
		{
			name: "multiline body",
			content: `---
name: Multi
---
Line one.

Line two.

Line three.`,
			check: func(t *testing.T, cfg *BotConfig) {
				want := "Line one.\n\nLine two.\n\nLine three."
				if cfg.Body != want {
					t.Errorf("Body = %q, want %q", cfg.Body, want)
				}
			},
		},
		{
			name: "body is trimmed",
			content: `---
name: Trim
---

  leading and trailing whitespace
`,
			check: func(t *testing.T, cfg *BotConfig) {
				want := "leading and trailing whitespace"
				if cfg.Body != want {
					t.Errorf("Body = %q, want %q", cfg.Body, want)
				}
			},
		},
		{
			name: "raw map captures extra fields",
			content: `---
name: Raw
custom_field: hello
---
Body.`,
			check: func(t *testing.T, cfg *BotConfig) {
				if cfg.Raw == nil {
					t.Fatal("Raw is nil")
				}
				if v, ok := cfg.Raw["custom_field"].(string); !ok || v != "hello" {
					t.Errorf("Raw[custom_field] = %v, want %q", cfg.Raw["custom_field"], "hello")
				}
			},
		},
		{
			name: "no frontmatter",
			content: `Just some markdown without frontmatter.`,
			wantErr: true,
		},
		{
			name: "invalid yaml",
			content: `---
name: [invalid yaml
  bad: indentation
---
Body.`,
			wantErr: true,
		},
		{
			name: "empty body",
			content: `---
name: Empty
---
`,
			check: func(t *testing.T, cfg *BotConfig) {
				if cfg.Body != "" {
					t.Errorf("Body = %q, want empty", cfg.Body)
				}
			},
		},
		{
			name: "empty env map",
			content: `---
name: NoEnv
---
Hello.`,
			check: func(t *testing.T, cfg *BotConfig) {
				if cfg.Env != nil && len(cfg.Env) != 0 {
					t.Errorf("Env = %v, want nil or empty", cfg.Env)
				}
			},
		},
		{
			name: "zero max_turns stays zero",
			content: `---
name: ZeroTurns
max_turns: 0
---
Body.`,
			check: func(t *testing.T, cfg *BotConfig) {
				if cfg.MaxTurns != 0 {
					t.Errorf("MaxTurns = %d, want 0", cfg.MaxTurns)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempBotMD(t, tt.content)
			cfg, err := FromMD(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tt.check(t, cfg)
		})
	}
}

func TestFromMD_FileNotFound(t *testing.T) {
	_, err := FromMD("/nonexistent/path/BOT.md")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestResolveEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		osEnv   map[string]string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "literal values pass through",
			env:   map[string]string{"KEY": "value", "OTHER": "123"},
			want:  map[string]string{"KEY": "value", "OTHER": "123"},
		},
		{
			name:  "env var reference resolved",
			env:   map[string]string{"TOKEN": "${TEST_BOTCTL_TOKEN}"},
			osEnv: map[string]string{"TEST_BOTCTL_TOKEN": "secret"},
			want:  map[string]string{"TOKEN": "secret"},
		},
		{
			name:  "mixed literal and reference",
			env:   map[string]string{"LITERAL": "hello", "REF": "${TEST_BOTCTL_VAR}"},
			osEnv: map[string]string{"TEST_BOTCTL_VAR": "world"},
			want:  map[string]string{"LITERAL": "hello", "REF": "world"},
		},
		{
			name:    "unset env var returns error",
			env:     map[string]string{"MISSING": "${TEST_BOTCTL_UNSET_12345}"},
			wantErr: true,
		},
		{
			name: "empty map",
			env:  map[string]string{},
			want: map[string]string{},
		},
		{
			name: "nil map",
			env:  nil,
			want: map[string]string{},
		},
		{
			name:  "partial syntax not treated as reference",
			env:   map[string]string{"A": "${incomplete", "B": "no-braces"},
			want:  map[string]string{"A": "${incomplete", "B": "no-braces"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.osEnv {
				t.Setenv(k, v)
			}

			cfg := &BotConfig{Env: tt.env}
			got, err := cfg.ResolveEnv()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d", len(got), len(tt.want))
			}
			for k, wantV := range tt.want {
				if got[k] != wantV {
					t.Errorf("got[%q] = %q, want %q", k, got[k], wantV)
				}
			}
		})
	}
}

func TestFrontmatterRegex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		matches bool
	}{
		{
			name:    "standard frontmatter",
			input:   "---\nname: test\n---\nbody",
			matches: true,
		},
		{
			name:    "frontmatter with trailing spaces on delimiters",
			input:   "---  \nname: test\n---  \nbody",
			matches: true,
		},
		{
			name:    "no closing delimiter",
			input:   "---\nname: test\nbody",
			matches: false,
		},
		{
			name:    "empty input",
			input:   "",
			matches: false,
		},
		{
			name:    "no opening delimiter",
			input:   "name: test\n---\nbody",
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := frontmatterRe.Match([]byte(tt.input))
			if got != tt.matches {
				t.Errorf("Match(%q) = %v, want %v", tt.input, got, tt.matches)
			}
		})
	}
}
