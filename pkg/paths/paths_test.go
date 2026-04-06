package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeDir_Default(t *testing.T) {
	t.Setenv("BOTCTL_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".botctl")
	if got := HomeDir(); got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}

func TestHomeDir_WithBotctlHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	if got := HomeDir(); got != tmp {
		t.Errorf("HomeDir() = %q, want %q", got, tmp)
	}
}

func TestBotsDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "bots")
	if got := BotsDir(); got != want {
		t.Errorf("BotsDir() = %q, want %q", got, want)
	}
}

func TestWorkspaceDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "workspace")
	if got := WorkspaceDir(); got != want {
		t.Errorf("WorkspaceDir() = %q, want %q", got, want)
	}
}

func TestDataDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "data")
	if got := DataDir(); got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

func TestDBFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "data", "botctl.db")
	if got := DBFile(); got != want {
		t.Errorf("DBFile() = %q, want %q", got, want)
	}
}

func TestBotLogDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "bots", "mybot", "logs")
	if got := BotLogDir("mybot"); got != want {
		t.Errorf("BotLogDir(\"mybot\") = %q, want %q", got, want)
	}
}

func TestRunLogFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "bots", "mybot", "logs", "20240101-120000.log")
	if got := RunLogFile("mybot", "20240101-120000.log"); got != want {
		t.Errorf("RunLogFile() = %q, want %q", got, want)
	}
}

func TestBootLogFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "bots", "mybot", "logs", "boot.log")
	if got := BootLogFile("mybot"); got != want {
		t.Errorf("BootLogFile() = %q, want %q", got, want)
	}
}

func TestStateDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "run")
	if got := StateDir(); got != want {
		t.Errorf("StateDir() = %q, want %q", got, want)
	}
}

func TestLegacyPidFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "run", "mybot.pid")
	if got := LegacyPidFile("mybot"); got != want {
		t.Errorf("LegacyPidFile() = %q, want %q", got, want)
	}
}

func TestLegacyLogFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "run", "mybot.log")
	if got := LegacyLogFile("mybot"); got != want {
		t.Errorf("LegacyLogFile() = %q, want %q", got, want)
	}
}

func TestLegacyStatsFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "run", "mybot.stats.json")
	if got := LegacyStatsFile("mybot"); got != want {
		t.Errorf("LegacyStatsFile() = %q, want %q", got, want)
	}
}

func TestAgentsSkillsDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".agents", "skills")
	if got := AgentsSkillsDir(); got != want {
		t.Errorf("AgentsSkillsDir() = %q, want %q", got, want)
	}
}

func TestAgentsSkillsDir_IgnoresBotctlHome(t *testing.T) {
	t.Setenv("BOTCTL_HOME", filepath.Join(os.TempDir(), "custom-botctl"))
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".agents", "skills")
	if got := AgentsSkillsDir(); got != want {
		t.Errorf("AgentsSkillsDir() should not be affected by BOTCTL_HOME, got %q, want %q", got, want)
	}
}

func TestGlobalSkillsDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)
	want := filepath.Join(tmp, "skills")
	if got := GlobalSkillsDir(); got != want {
		t.Errorf("GlobalSkillsDir() = %q, want %q", got, want)
	}
}

func TestEnsureDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(tmp, "bots"),
		filepath.Join(tmp, "workspace"),
		filepath.Join(tmp, "data"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("expected directory %q to exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory", dir)
		}
	}
}

func TestEnsureDirs_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("first EnsureDirs() error: %v", err)
	}
	if err := EnsureDirs(); err != nil {
		t.Fatalf("second EnsureDirs() error: %v", err)
	}
}

func TestAllPathsRespectBotctlHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("BOTCTL_HOME", tmp)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"HomeDir", HomeDir(), tmp},
		{"BotsDir", BotsDir(), filepath.Join(tmp, "bots")},
		{"WorkspaceDir", WorkspaceDir(), filepath.Join(tmp, "workspace")},
		{"DataDir", DataDir(), filepath.Join(tmp, "data")},
		{"DBFile", DBFile(), filepath.Join(tmp, "data", "botctl.db")},
		{"StateDir", StateDir(), filepath.Join(tmp, "run")},
		{"GlobalSkillsDir", GlobalSkillsDir(), filepath.Join(tmp, "skills")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
