package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/montanaflynn/botctl/internal/config"
	"github.com/montanaflynn/botctl/internal/db"
	"github.com/montanaflynn/botctl/internal/paths"
)

// IsRunning checks if a bot is alive by reading its PID from the db and checking the process.
// Returns running status and PID (0 if not running).
func IsRunning(name string, database *db.DB) (bool, int) {
	pid, ok := database.GetPID(name)
	if !ok {
		return false, 0
	}

	if !isProcessAlive(pid) {
		database.RemovePID(name)
		return false, 0
	}

	return true, pid
}

// StartBot spawns `botctl harness <bot_dir>` as a background process group leader.
func StartBot(name, botDir string, cfg *config.BotConfig, truncateLog bool, database *db.DB) (int, error) {
	return startHarness(name, botDir, cfg, database, false, "")
}

// StartBotOnce spawns a one-shot harness run (no loop).
func StartBotOnce(name, botDir string, cfg *config.BotConfig, database *db.DB, message string) (int, error) {
	return startHarness(name, botDir, cfg, database, true, message)
}

// StartBotWithMessage spawns the harness with a message appended to the first run's prompt.
func StartBotWithMessage(name, botDir string, cfg *config.BotConfig, database *db.DB, message string) (int, error) {
	return startHarness(name, botDir, cfg, database, false, message)
}

func startHarness(name, botDir string, cfg *config.BotConfig, database *db.DB, once bool, message string) (int, error) {
	id := cfg.ID
	if id == "" {
		id = name
	}

	running, pid := IsRunning(id, database)
	if running {
		return 0, fmt.Errorf("%s is already running (pid %d)", name, pid)
	}

	lf := paths.BootLogFile(name)
	if err := os.MkdirAll(filepath.Dir(lf), 0o755); err != nil {
		return 0, fmt.Errorf("create log dir: %w", err)
	}

	logHandle, err := os.OpenFile(lf, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		logHandle.Close()
		return 0, fmt.Errorf("find executable: %w", err)
	}

	args := []string{"harness"}
	if once {
		args = append(args, "--once")
	}
	if message != "" {
		args = append(args, "--message", message)
	}
	args = append(args, botDir)

	cmd := exec.Command(self, args...)
	cmd.Stdout = logHandle
	cmd.Stderr = logHandle
	cmd.SysProcAttr = sysProcAttr()

	cmd.Env = os.Environ()
	if cfg != nil && len(cfg.Env) > 0 {
		resolved, err := cfg.ResolveEnv()
		if err != nil {
			logHandle.Close()
			return 0, fmt.Errorf("resolve env: %w", err)
		}
		for k, v := range resolved {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	if err := cmd.Start(); err != nil {
		logHandle.Close()
		return 0, fmt.Errorf("start: %w", err)
	}

	if err := database.SetPID(id, cmd.Process.Pid); err != nil {
		logHandle.Close()
		return 0, fmt.Errorf("write pid: %w", err)
	}

	return cmd.Process.Pid, nil
}

// StopBot sends a termination signal to the process group, waits up to 3 seconds,
// then force-kills if necessary. Returns true if the bot was stopped.
func StopBot(name string, database *db.DB) bool {
	running, pid := IsRunning(name, database)
	if !running || pid == 0 {
		return false
	}

	KillProcessGroup(pid)

	database.RemovePID(name)
	return true
}
