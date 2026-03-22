package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeDir_Default(t *testing.T) {
	t.Setenv("MM_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".botctl")
	if got := HomeDir(); got != want {
		t.Errorf("HomeDir() = %q, want %q", got, want)
	}
}

func TestHomeDir_WithMMHome(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/custom-botctl")
	if got := HomeDir(); got != "/tmp/custom-botctl" {
		t.Errorf("HomeDir() = %q, want %q", got, "/tmp/custom-botctl")
	}
}

func TestBotsDir(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/bots"
	if got := BotsDir(); got != want {
		t.Errorf("BotsDir() = %q, want %q", got, want)
	}
}

func TestWorkspaceDir(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/workspace"
	if got := WorkspaceDir(); got != want {
		t.Errorf("WorkspaceDir() = %q, want %q", got, want)
	}
}

func TestDataDir(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/data"
	if got := DataDir(); got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

func TestDBFile(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/data/botctl.db"
	if got := DBFile(); got != want {
		t.Errorf("DBFile() = %q, want %q", got, want)
	}
}

func TestBotLogDir(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/bots/mybot/logs"
	if got := BotLogDir("mybot"); got != want {
		t.Errorf("BotLogDir(\"mybot\") = %q, want %q", got, want)
	}
}

func TestRunLogFile(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/bots/mybot/logs/20240101-120000.log"
	if got := RunLogFile("mybot", "20240101-120000.log"); got != want {
		t.Errorf("RunLogFile() = %q, want %q", got, want)
	}
}

func TestBootLogFile(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/bots/mybot/logs/boot.log"
	if got := BootLogFile("mybot"); got != want {
		t.Errorf("BootLogFile() = %q, want %q", got, want)
	}
}

func TestStateDir(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/run"
	if got := StateDir(); got != want {
		t.Errorf("StateDir() = %q, want %q", got, want)
	}
}

func TestLegacyPidFile(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/run/mybot.pid"
	if got := LegacyPidFile("mybot"); got != want {
		t.Errorf("LegacyPidFile() = %q, want %q", got, want)
	}
}

func TestLegacyLogFile(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/run/mybot.log"
	if got := LegacyLogFile("mybot"); got != want {
		t.Errorf("LegacyLogFile() = %q, want %q", got, want)
	}
}

func TestLegacyStatsFile(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/run/mybot.stats.json"
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

func TestAgentsSkillsDir_IgnoresMMHome(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/custom-botctl")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".agents", "skills")
	if got := AgentsSkillsDir(); got != want {
		t.Errorf("AgentsSkillsDir() should not be affected by MM_HOME, got %q, want %q", got, want)
	}
}

func TestGlobalSkillsDir(t *testing.T) {
	t.Setenv("MM_HOME", "/tmp/test-botctl")
	want := "/tmp/test-botctl/skills"
	if got := GlobalSkillsDir(); got != want {
		t.Errorf("GlobalSkillsDir() = %q, want %q", got, want)
	}
}

func TestEnsureDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MM_HOME", tmp)

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
	t.Setenv("MM_HOME", tmp)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("first EnsureDirs() error: %v", err)
	}
	if err := EnsureDirs(); err != nil {
		t.Fatalf("second EnsureDirs() error: %v", err)
	}
}

func TestAllPathsRespectMMHome(t *testing.T) {
	t.Setenv("MM_HOME", "/custom/root")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"HomeDir", HomeDir(), "/custom/root"},
		{"BotsDir", BotsDir(), "/custom/root/bots"},
		{"WorkspaceDir", WorkspaceDir(), "/custom/root/workspace"},
		{"DataDir", DataDir(), "/custom/root/data"},
		{"DBFile", DBFile(), "/custom/root/data/botctl.db"},
		{"StateDir", StateDir(), "/custom/root/run"},
		{"GlobalSkillsDir", GlobalSkillsDir(), "/custom/root/skills"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
