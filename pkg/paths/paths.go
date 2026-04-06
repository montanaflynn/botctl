package paths

import (
	"os"
	"path/filepath"
)

// HomeDir returns the root directory, respecting BOTCTL_HOME env var.
func HomeDir() string {
	if v := os.Getenv("BOTCTL_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".botctl")
}

// BotsDir returns the directory containing bot definitions.
func BotsDir() string { return filepath.Join(HomeDir(), "bots") }

// WorkspaceDir returns the shared workspace directory.
func WorkspaceDir() string { return filepath.Join(HomeDir(), "workspace") }

// DataDir returns the directory for database files.
func DataDir() string { return filepath.Join(HomeDir(), "data") }

// DBFile returns the path to the SQLite database.
func DBFile() string { return filepath.Join(DataDir(), "botctl.db") }

// BotLogDir returns the default log directory for a named bot.
func BotLogDir(name string) string { return filepath.Join(BotsDir(), name, "logs") }

// RunLogFile returns the path for a specific run log file.
func RunLogFile(name, filename string) string { return filepath.Join(BotLogDir(name), filename) }

// BootLogFile returns the boot log path used by the process spawner before the harness takes over.
func BootLogFile(name string) string { return filepath.Join(BotLogDir(name), "boot.log") }

// StateDir returns the legacy runtime state directory (for migration only).
func StateDir() string { return filepath.Join(HomeDir(), "run") }

// LegacyPidFile returns the legacy PID file path (for migration only).
func LegacyPidFile(name string) string { return filepath.Join(StateDir(), name+".pid") }

// LegacyLogFile returns the legacy log file path (for migration only).
func LegacyLogFile(name string) string { return filepath.Join(StateDir(), name+".log") }

// LegacyStatsFile returns the legacy stats JSON path (for migration only).
func LegacyStatsFile(name string) string { return filepath.Join(StateDir(), name+".stats.json") }

// AgentsSkillsDir returns the cross-agent shared skills directory.
func AgentsSkillsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agents", "skills")
}

// GlobalSkillsDir returns the botctl-wide shared skills directory.
func GlobalSkillsDir() string { return filepath.Join(HomeDir(), "skills") }

// EnsureDirs creates all required directories.
func EnsureDirs() error {
	for _, d := range []string{BotsDir(), WorkspaceDir(), DataDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
