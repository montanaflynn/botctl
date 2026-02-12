package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/montanaflynn/botctl-go/internal/paths"
)

// DB wraps a SQLite database connection.
type DB struct {
	conn *sql.DB
}

// BotStats holds aggregated stats for a single bot.
type BotStats struct {
	Runs       int
	TotalCost  float64
	TotalTurns int
	LastRun    string // RFC3339
}

// LogEntry represents a single structured log entry.
type LogEntry struct {
	ID      int64
	RunID   int64
	BotID   string
	Kind    string
	Heading string
	Body    string
}

// migrateDBLocation moves DB files from ~/.botctl/botctl.db to ~/.botctl/data/botctl.db.
func migrateDBLocation() {
	home := paths.MMHome()
	oldPath := filepath.Join(home, "botctl.db")
	newPath := paths.DBFile()
	if oldPath == newPath {
		return
	}
	if _, err := os.Stat(oldPath); err != nil {
		return // old file doesn't exist, nothing to migrate
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "migrate db location: %v\n", err)
		return
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := oldPath + suffix
		dst := newPath + suffix
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				fmt.Fprintf(os.Stderr, "migrate db file %s: %v\n", src, err)
			}
		}
	}
}

// Open opens (or creates) the SQLite database and runs migrations.
func Open() (*DB, error) {
	migrateDBLocation()

	dbPath := paths.DBFile()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys for cascade deletes
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	d := &DB{conn: conn}
	if err := d.createTables(); err != nil {
		conn.Close()
		return nil, err
	}

	if err := d.migrateFromJSON(); err != nil {
		// Log but don't fail — migration is best-effort
		fmt.Fprintf(os.Stderr, "migration warning: %v\n", err)
	}

	d.migrateBotLogs()

	return d, nil
}

func (d *DB) createTables() error {
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS runs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			bot_name   TEXT NOT NULL,
			session_id TEXT,
			started_at TEXT NOT NULL,
			duration_ms INTEGER DEFAULT 0,
			cost_usd   REAL DEFAULT 0,
			turns      INTEGER DEFAULT 0,
			log_file   TEXT
		);
		CREATE TABLE IF NOT EXISTS pids (
			bot_name   TEXT PRIMARY KEY,
			pid        INTEGER NOT NULL,
			started_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS messages (
			id                    INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id                INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			seq                   INTEGER NOT NULL,
			message_id            TEXT,
			model                 TEXT,
			input_tokens          INTEGER DEFAULT 0,
			output_tokens         INTEGER DEFAULT 0,
			cache_creation_tokens INTEGER DEFAULT 0,
			cache_read_tokens     INTEGER DEFAULT 0,
			raw_json              TEXT NOT NULL,
			created_at            TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_messages_run_id ON messages(run_id);
		CREATE TABLE IF NOT EXISTS pending_messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			bot_id     TEXT NOT NULL,
			message    TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS log_entries (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			bot_id     TEXT NOT NULL,
			run_id     INTEGER REFERENCES runs(id) ON DELETE CASCADE,
			kind       TEXT NOT NULL,
			heading    TEXT NOT NULL DEFAULT '',
			body       TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_log_entries_bot ON log_entries(bot_id, id);
	`)
	if err != nil {
		return err
	}
	// Migrate existing databases: add columns if missing
	d.conn.Exec(`ALTER TABLE runs ADD COLUMN log_file TEXT`)
	d.conn.Exec(`ALTER TABLE runs ADD COLUMN run_number INTEGER DEFAULT 0`)
	return nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// BeginRun records a new run starting and returns (global row ID, per-bot run number).
func (d *DB) BeginRun(botName, logFile string) (int64, int, error) {
	// Compute per-bot run number (use max if set, otherwise count existing rows)
	var maxNum, total int
	row := d.conn.QueryRow(
		`SELECT COALESCE(MAX(run_number), 0), COUNT(*) FROM runs WHERE bot_name = ?`, botName)
	if err := row.Scan(&maxNum, &total); err != nil {
		maxNum, total = 0, 0
	}
	runNumber := maxNum
	if runNumber < total {
		runNumber = total // pre-migration rows have run_number=0
	}
	runNumber++

	res, err := d.conn.Exec(
		`INSERT INTO runs (bot_name, started_at, log_file, run_number) VALUES (?, ?, ?, ?)`,
		botName, time.Now().Format(time.RFC3339), logFile, runNumber,
	)
	if err != nil {
		return 0, 0, err
	}
	id, err := res.LastInsertId()
	return id, runNumber, err
}

// EndRun updates a run with its final stats.
func (d *DB) EndRun(runID int64, sessionID string, durationMS int64, costUSD float64, turns int) error {
	_, err := d.conn.Exec(
		`UPDATE runs SET session_id = ?, duration_ms = ?, cost_usd = ?, turns = ? WHERE id = ?`,
		sessionID, durationMS, costUSD, turns, runID,
	)
	return err
}

// InsertMessage stores a single assistant message for a run.
func (d *DB) InsertMessage(runID int64, seq int, msgID, model string, inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int, rawJSON string) error {
	_, err := d.conn.Exec(
		`INSERT INTO messages (run_id, seq, message_id, model, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens, raw_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, seq, msgID, model, inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens, rawJSON, time.Now().Format(time.RFC3339),
	)
	return err
}

// RunMessages returns the raw JSON envelopes for a run, ordered by seq.
func (d *DB) RunMessages(runID int64) []string {
	rows, err := d.conn.Query(
		`SELECT raw_json FROM messages WHERE run_id = ? ORDER BY seq`, runID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var msgs []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err == nil {
			msgs = append(msgs, raw)
		}
	}
	return msgs
}

// EnqueueMessage queues a message for a bot's harness to pick up.
func (d *DB) EnqueueMessage(botID, message string) error {
	_, err := d.conn.Exec(
		`INSERT INTO pending_messages (bot_id, message, created_at) VALUES (?, ?, ?)`,
		botID, message, time.Now().Format(time.RFC3339),
	)
	return err
}

// DequeueMessage retrieves and removes the oldest pending message for a bot.
// Returns empty string if no message is pending.
func (d *DB) DequeueMessage(botID string) string {
	var id int64
	var message string
	err := d.conn.QueryRow(
		`SELECT id, message FROM pending_messages WHERE bot_id = ? ORDER BY id LIMIT 1`,
		botID,
	).Scan(&id, &message)
	if err != nil {
		return ""
	}
	d.conn.Exec(`DELETE FROM pending_messages WHERE id = ?`, id)
	return message
}

// DequeueAllMessages retrieves and removes ALL pending messages for a bot.
// Returns them joined with newlines. Returns empty string if none pending.
func (d *DB) DequeueAllMessages(botID string) string {
	rows, err := d.conn.Query(
		`SELECT id, message FROM pending_messages WHERE bot_id = ? ORDER BY id`, botID,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()

	var ids []int64
	var msgs []string
	for rows.Next() {
		var id int64
		var msg string
		if err := rows.Scan(&id, &msg); err == nil {
			ids = append(ids, id)
			msgs = append(msgs, msg)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	for _, id := range ids {
		d.conn.Exec(`DELETE FROM pending_messages WHERE id = ?`, id)
	}
	return strings.Join(msgs, "\n")
}

// HasPendingMessages returns true if there are any pending messages for a bot.
func (d *DB) HasPendingMessages(botID string) bool {
	var count int
	err := d.conn.QueryRow(
		`SELECT COUNT(*) FROM pending_messages WHERE bot_id = ?`, botID,
	).Scan(&count)
	return err == nil && count > 0
}

// InsertLogEntry stores a single structured log entry and returns its row ID.
// Use runID=0 for entries not associated with a specific run (e.g. TUI events).
func (d *DB) InsertLogEntry(botID string, runID int64, kind, heading, body string) (int64, error) {
	var rid any
	if runID > 0 {
		rid = runID
	}
	res, err := d.conn.Exec(
		`INSERT INTO log_entries (bot_id, run_id, kind, heading, body, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		botID, rid, kind, heading, body, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecentLogEntries returns the newest N log entries for a bot, returned oldest-first.
func (d *DB) RecentLogEntries(botID string, limit int) []LogEntry {
	rows, err := d.conn.Query(
		`SELECT id, COALESCE(run_id, 0), bot_id, kind, heading, body FROM log_entries WHERE bot_id = ? ORDER BY id DESC LIMIT ?`,
		botID, limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.RunID, &e.BotID, &e.Kind, &e.Heading, &e.Body); err == nil {
			entries = append(entries, e)
		}
	}
	// Reverse to oldest-first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

// LogEntriesAfter returns log entries for a bot with id > afterID, up to limit, oldest-first.
func (d *DB) LogEntriesAfter(botID string, afterID int64, limit int) []LogEntry {
	rows, err := d.conn.Query(
		`SELECT id, COALESCE(run_id, 0), bot_id, kind, heading, body FROM log_entries WHERE bot_id = ? AND id > ? ORDER BY id ASC LIMIT ?`,
		botID, afterID, limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.RunID, &e.BotID, &e.Kind, &e.Heading, &e.Body); err == nil {
			entries = append(entries, e)
		}
	}
	return entries
}

// RunLogEntries returns all log entries for a specific run, oldest-first.
func (d *DB) RunLogEntries(runID int64) []LogEntry {
	rows, err := d.conn.Query(
		`SELECT id, COALESCE(run_id, 0), bot_id, kind, heading, body FROM log_entries WHERE run_id = ? ORDER BY id ASC`,
		runID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.RunID, &e.BotID, &e.Kind, &e.Heading, &e.Body); err == nil {
			entries = append(entries, e)
		}
	}
	return entries
}

// PruneLogEntries deletes orphaned log entries (run_id IS NULL) beyond the keep limit.
func (d *DB) PruneLogEntries(botID string, keep int) {
	d.conn.Exec(
		`DELETE FROM log_entries WHERE bot_id = ? AND run_id IS NULL AND id NOT IN (
			SELECT id FROM log_entries WHERE bot_id = ? AND run_id IS NULL ORDER BY id DESC LIMIT ?
		)`,
		botID, botID, keep,
	)
}

// RecentRunLogs returns log filenames for a bot, newest first.
func (d *DB) RecentRunLogs(botName string, limit int) []string {
	rows, err := d.conn.Query(
		`SELECT log_file FROM runs WHERE bot_name = ? AND log_file IS NOT NULL AND log_file != '' ORDER BY id DESC LIMIT ?`,
		botName, limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err == nil {
			files = append(files, f)
		}
	}
	return files
}

// LatestRunLog returns the newest run's log filename for a bot.
func (d *DB) LatestRunLog(botName string) string {
	files := d.RecentRunLogs(botName, 1)
	if len(files) == 0 {
		return ""
	}
	return files[0]
}

// PruneRuns deletes old run rows beyond the keep limit and returns the deleted log filenames.
func (d *DB) PruneRuns(botName string, keep int) []string {
	// Find rows to delete
	rows, err := d.conn.Query(
		`SELECT id, log_file FROM runs WHERE bot_name = ? ORDER BY id DESC LIMIT -1 OFFSET ?`,
		botName, keep,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []int64
	var files []string
	for rows.Next() {
		var id int64
		var f sql.NullString
		if err := rows.Scan(&id, &f); err == nil {
			ids = append(ids, id)
			if f.Valid && f.String != "" {
				files = append(files, f.String)
			}
		}
	}

	if len(ids) == 0 {
		return nil
	}

	// Delete in batches
	for _, id := range ids {
		d.conn.Exec(`DELETE FROM runs WHERE id = ?`, id)
	}

	return files
}

// GetBotStats returns aggregated stats for a bot.
func (d *DB) GetBotStats(botName string) BotStats {
	var s BotStats
	row := d.conn.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(cost_usd),0), COALESCE(SUM(turns),0), COALESCE(MAX(started_at),'') FROM runs WHERE bot_name = ?`,
		botName,
	)
	_ = row.Scan(&s.Runs, &s.TotalCost, &s.TotalTurns, &s.LastRun)
	return s
}

// LatestRunTurns returns the turn count for a bot's most recent run.
// Uses the finalized turns column when available, otherwise counts
// message envelopes as a live estimate during an active run.
func (d *DB) LatestRunTurns(botName string) int {
	var turns, runID int
	_ = d.conn.QueryRow(
		`SELECT id, COALESCE(turns, 0) FROM runs WHERE bot_name = ? ORDER BY started_at DESC LIMIT 1`,
		botName,
	).Scan(&runID, &turns)
	if turns > 0 {
		return turns
	}
	// Active run — count message envelopes as live estimate
	var count int
	_ = d.conn.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE run_id = ?`, runID,
	).Scan(&count)
	return count
}

// SetPID records a running bot's PID.
func (d *DB) SetPID(botName string, pid int) error {
	_, err := d.conn.Exec(
		`INSERT OR REPLACE INTO pids (bot_name, pid, started_at) VALUES (?, ?, ?)`,
		botName, pid, time.Now().Format(time.RFC3339),
	)
	return err
}

// GetPID returns the PID for a bot, if any.
func (d *DB) GetPID(botName string) (int, bool) {
	var pid int
	err := d.conn.QueryRow(`SELECT pid FROM pids WHERE bot_name = ?`, botName).Scan(&pid)
	if err != nil {
		return 0, false
	}
	return pid, true
}

// RemovePID removes a bot's PID record.
func (d *DB) RemovePID(botName string) error {
	_, err := d.conn.Exec(`DELETE FROM pids WHERE bot_name = ?`, botName)
	return err
}

// DeleteBotData removes all DB records for a bot (runs, messages, log_entries, pids).
func (d *DB) DeleteBotData(botName string) error {
	// Delete messages for all runs of this bot
	_, err := d.conn.Exec(
		`DELETE FROM messages WHERE run_id IN (SELECT id FROM runs WHERE bot_name = ?)`, botName)
	if err != nil {
		return err
	}
	// Delete log entries (both run-associated and orphaned events)
	if _, err := d.conn.Exec(`DELETE FROM log_entries WHERE bot_id = ?`, botName); err != nil {
		return err
	}
	if _, err := d.conn.Exec(`DELETE FROM runs WHERE bot_name = ?`, botName); err != nil {
		return err
	}
	_, err = d.conn.Exec(`DELETE FROM pids WHERE bot_name = ?`, botName)
	return err
}

// migrateFromJSON imports legacy JSON stats, PID files, and log files.
func (d *DB) migrateFromJSON() error {
	stateDir := paths.StateDir()
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		return nil // nothing to migrate
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil
	}

	migrated := false
	for _, entry := range entries {
		name := entry.Name()

		// Migrate stats JSON files
		if strings.HasSuffix(name, ".stats.json") {
			botName := strings.TrimSuffix(name, ".stats.json")
			statsPath := filepath.Join(stateDir, name)
			if err := d.migrateStatsFile(botName, statsPath); err != nil {
				fmt.Fprintf(os.Stderr, "  migrate %s stats: %v\n", botName, err)
			} else {
				os.Remove(statsPath)
				migrated = true
			}
		}

		// Migrate log files — rename to timestamped per-run log
		if strings.HasSuffix(name, ".log") {
			botName := strings.TrimSuffix(name, ".log")
			srcLog := filepath.Join(stateDir, name)
			dstDir := paths.BotLogDir(botName)
			if err := os.MkdirAll(dstDir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "  migrate %s log dir: %v\n", botName, err)
				continue
			}
			// Name based on file mtime
			logFilename := "migrated.log"
			if info, err := os.Stat(srcLog); err == nil {
				logFilename = info.ModTime().Format("20060102-150405") + ".log"
			}
			dstLog := paths.RunLogFile(botName, logFilename)
			if err := os.Rename(srcLog, dstLog); err != nil {
				fmt.Fprintf(os.Stderr, "  migrate %s log: %v\n", botName, err)
			} else {
				migrated = true
			}
		}

		// Migrate PID files
		if strings.HasSuffix(name, ".pid") {
			botName := strings.TrimSuffix(name, ".pid")
			pidPath := filepath.Join(stateDir, name)
			data, err := os.ReadFile(pidPath)
			if err == nil {
				pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
				if err == nil {
					_ = d.SetPID(botName, pid)
				}
				os.Remove(pidPath)
				migrated = true
			}
		}
	}

	// Remove run/ directory if empty
	if migrated {
		remaining, _ := os.ReadDir(stateDir)
		if len(remaining) == 0 {
			os.Remove(stateDir)
		}
	}

	return nil
}

func (d *DB) migrateStatsFile(botName, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var stats map[string]any
	if err := json.Unmarshal(data, &stats); err != nil {
		return err
	}

	totalCost, _ := stats["total_cost_usd"].(float64)
	totalTurns, _ := stats["total_turns"].(float64)
	totalDuration, _ := stats["total_duration_ms"].(float64)
	runs, _ := stats["runs"].(float64)
	lastRun, _ := stats["last_run"].(string)

	if lastRun == "" {
		lastRun = time.Now().Format(time.RFC3339)
	}

	// Insert a single summary row representing all prior runs
	if runs > 0 {
		avgDuration := int64(0)
		avgCost := float64(0)
		avgTurns := 0
		if runs > 0 {
			avgDuration = int64(totalDuration / runs)
			avgCost = totalCost / runs
			avgTurns = int(totalTurns / runs)
		}
		// Insert individual rows to preserve the run count
		for i := 0; i < int(runs); i++ {
			_, err := d.conn.Exec(
				`INSERT INTO runs (bot_name, started_at, duration_ms, cost_usd, turns) VALUES (?, ?, ?, ?, ?)`,
				botName, lastRun, avgDuration, avgCost, avgTurns,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// migrateBotLogs renames existing bot.log files to timestamped per-run names.
func (d *DB) migrateBotLogs() {
	botsDir := paths.BotsDir()
	entries, err := os.ReadDir(botsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		botName := entry.Name()
		oldLog := filepath.Join(paths.BotLogDir(botName), "bot.log")
		info, err := os.Stat(oldLog)
		if err != nil {
			continue
		}

		// Rename bot.log → {mtime}.log
		logFilename := info.ModTime().Format("20060102-150405") + ".log"
		newLog := paths.RunLogFile(botName, logFilename)
		if err := os.Rename(oldLog, newLog); err != nil {
			fmt.Fprintf(os.Stderr, "  migrate %s bot.log: %v\n", botName, err)
			continue
		}

		// Ensure at least one run row exists with this log file
		existing := d.LatestRunLog(botName)
		if existing == "" {
			d.conn.Exec(
				`INSERT INTO runs (bot_name, started_at, log_file) VALUES (?, ?, ?)`,
				botName, info.ModTime().Format(time.RFC3339), logFilename,
			)
		} else {
			// Update the latest run row to point to this file
			d.conn.Exec(
				`UPDATE runs SET log_file = ? WHERE bot_name = ? AND id = (SELECT MAX(id) FROM runs WHERE bot_name = ?)`,
				logFilename, botName, botName,
			)
		}
	}
}
