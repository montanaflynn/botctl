package db

import (
	"os"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("MM_HOME", tmp)
	d, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpenCreatesDatabase(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("MM_HOME", tmp)
	d, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	dbPath := tmp + "/data/botctl.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("expected database file at %s", dbPath)
	}
}

func TestBeginAndEndRun(t *testing.T) {
	d := openTestDB(t)

	runID, runNum, err := d.BeginRun("test-bot", "run1.log")
	if err != nil {
		t.Fatalf("BeginRun: %v", err)
	}
	if runID < 1 {
		t.Fatalf("expected positive runID, got %d", runID)
	}
	if runNum != 1 {
		t.Fatalf("expected runNumber=1, got %d", runNum)
	}

	err = d.EndRun(runID, "sess-abc", 5000, 0.05, 3)
	if err != nil {
		t.Fatalf("EndRun: %v", err)
	}

	stats := d.GetBotStats("test-bot")
	if stats.Runs != 1 {
		t.Fatalf("expected 1 run, got %d", stats.Runs)
	}
	if stats.TotalCost != 0.05 {
		t.Fatalf("expected cost 0.05, got %f", stats.TotalCost)
	}
	if stats.TotalTurns != 3 {
		t.Fatalf("expected 3 turns, got %d", stats.TotalTurns)
	}
}

func TestRunNumberIncrementsAcrossRuns(t *testing.T) {
	d := openTestDB(t)

	_, num1, _ := d.BeginRun("bot-a", "r1.log")
	_, num2, _ := d.BeginRun("bot-a", "r2.log")
	_, num3, _ := d.BeginRun("bot-a", "r3.log")

	if num1 != 1 || num2 != 2 || num3 != 3 {
		t.Fatalf("expected run numbers 1,2,3 got %d,%d,%d", num1, num2, num3)
	}

	_, numOther, _ := d.BeginRun("bot-b", "r1.log")
	if numOther != 1 {
		t.Fatalf("expected run number 1 for new bot, got %d", numOther)
	}
}

func TestPIDOperations(t *testing.T) {
	d := openTestDB(t)

	_, ok := d.GetPID("bot-x")
	if ok {
		t.Fatal("expected no PID for unknown bot")
	}

	if err := d.SetPID("bot-x", 12345); err != nil {
		t.Fatalf("SetPID: %v", err)
	}

	pid, ok := d.GetPID("bot-x")
	if !ok || pid != 12345 {
		t.Fatalf("expected pid 12345, got %d (ok=%v)", pid, ok)
	}

	if err := d.SetPID("bot-x", 99999); err != nil {
		t.Fatalf("SetPID replace: %v", err)
	}
	pid, ok = d.GetPID("bot-x")
	if !ok || pid != 99999 {
		t.Fatalf("expected pid 99999 after replace, got %d", pid)
	}

	if err := d.RemovePID("bot-x"); err != nil {
		t.Fatalf("RemovePID: %v", err)
	}
	_, ok = d.GetPID("bot-x")
	if ok {
		t.Fatal("expected no PID after removal")
	}
}

func TestPendingMessages(t *testing.T) {
	d := openTestDB(t)

	if d.HasPendingMessages("bot-m") {
		t.Fatal("expected no pending messages initially")
	}

	msg := d.DequeueMessage("bot-m")
	if msg != "" {
		t.Fatalf("expected empty dequeue, got %q", msg)
	}

	d.EnqueueMessage("bot-m", "hello")
	d.EnqueueMessage("bot-m", "world")

	if !d.HasPendingMessages("bot-m") {
		t.Fatal("expected pending messages after enqueue")
	}

	msg = d.DequeueMessage("bot-m")
	if msg != "hello" {
		t.Fatalf("expected 'hello', got %q", msg)
	}

	msg = d.DequeueMessage("bot-m")
	if msg != "world" {
		t.Fatalf("expected 'world', got %q", msg)
	}

	msg = d.DequeueMessage("bot-m")
	if msg != "" {
		t.Fatalf("expected empty after all dequeued, got %q", msg)
	}
}

func TestDequeueAllMessages(t *testing.T) {
	d := openTestDB(t)

	d.EnqueueMessage("bot-all", "one")
	d.EnqueueMessage("bot-all", "two")
	d.EnqueueMessage("bot-all", "three")

	result := d.DequeueAllMessages("bot-all")
	if result != "one\ntwo\nthree" {
		t.Fatalf("expected joined messages, got %q", result)
	}

	if d.HasPendingMessages("bot-all") {
		t.Fatal("expected no pending messages after DequeueAll")
	}
}

func TestDequeueAllMessagesEmpty(t *testing.T) {
	d := openTestDB(t)
	result := d.DequeueAllMessages("no-bot")
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestLogEntries(t *testing.T) {
	d := openTestDB(t)

	runID, _, _ := d.BeginRun("log-bot", "test.log")

	id1, err := d.InsertLogEntry("log-bot", runID, "info", "Starting", "body1")
	if err != nil {
		t.Fatalf("InsertLogEntry: %v", err)
	}
	if id1 < 1 {
		t.Fatalf("expected positive log entry id, got %d", id1)
	}

	id2, _ := d.InsertLogEntry("log-bot", runID, "tool", "Bash", "body2")
	id3, _ := d.InsertLogEntry("log-bot", runID, "result", "Done", "body3")

	entries := d.RecentLogEntries("log-bot", 10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].ID != id1 || entries[1].ID != id2 || entries[2].ID != id3 {
		t.Fatal("entries not in oldest-first order")
	}
	if entries[0].Kind != "info" || entries[0].Heading != "Starting" {
		t.Fatalf("unexpected entry content: %+v", entries[0])
	}
	if entries[0].RunID != runID {
		t.Fatalf("expected RunID=%d, got %d", runID, entries[0].RunID)
	}
}

func TestLogEntriesOrphan(t *testing.T) {
	d := openTestDB(t)

	id, err := d.InsertLogEntry("orphan-bot", 0, "event", "TUI action", "")
	if err != nil {
		t.Fatalf("InsertLogEntry orphan: %v", err)
	}
	if id < 1 {
		t.Fatal("expected positive id for orphan entry")
	}

	entries := d.RecentLogEntries("orphan-bot", 10)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].RunID != 0 {
		t.Fatalf("expected RunID=0 for orphan, got %d", entries[0].RunID)
	}
}

func TestRecentLogEntriesLimit(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 5; i++ {
		d.InsertLogEntry("limit-bot", 0, "info", "entry", "")
	}

	entries := d.RecentLogEntries("limit-bot", 3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries with limit, got %d", len(entries))
	}
	if entries[0].ID >= entries[1].ID || entries[1].ID >= entries[2].ID {
		t.Fatal("entries should be in ascending ID order")
	}
}

func TestLogEntriesAfter(t *testing.T) {
	d := openTestDB(t)

	id1, _ := d.InsertLogEntry("after-bot", 0, "a", "", "")
	id2, _ := d.InsertLogEntry("after-bot", 0, "b", "", "")
	d.InsertLogEntry("after-bot", 0, "c", "", "")

	entries := d.LogEntriesAfter("after-bot", id1, 10)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after id %d, got %d", id1, len(entries))
	}
	if entries[0].ID != id2 {
		t.Fatalf("expected first entry id=%d, got %d", id2, entries[0].ID)
	}
}

func TestRunLogEntries(t *testing.T) {
	d := openTestDB(t)

	runID, _, _ := d.BeginRun("runlog-bot", "test.log")
	d.InsertLogEntry("runlog-bot", runID, "a", "h1", "b1")
	d.InsertLogEntry("runlog-bot", runID, "b", "h2", "b2")
	d.InsertLogEntry("runlog-bot", 0, "orphan", "x", "y")

	entries := d.RunLogEntries(runID)
	if len(entries) != 2 {
		t.Fatalf("expected 2 run log entries, got %d", len(entries))
	}
}

func TestPruneLogEntries(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 10; i++ {
		d.InsertLogEntry("prune-bot", 0, "info", "entry", "")
	}

	d.PruneLogEntries("prune-bot", 3)

	entries := d.RecentLogEntries("prune-bot", 100)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after prune, got %d", len(entries))
	}
}

func TestInsertAndRunMessages(t *testing.T) {
	d := openTestDB(t)

	runID, _, _ := d.BeginRun("msg-bot", "msg.log")

	err := d.InsertMessage(runID, 1, "msg-1", "claude-3", 100, 200, 50, 30, `{"role":"assistant"}`)
	if err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	d.InsertMessage(runID, 2, "msg-2", "claude-3", 110, 210, 55, 35, `{"role":"assistant","turn":2}`)

	msgs := d.RunMessages(runID)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0] != `{"role":"assistant"}` {
		t.Fatalf("unexpected first message: %s", msgs[0])
	}
}

func TestRunMessagesEmpty(t *testing.T) {
	d := openTestDB(t)
	msgs := d.RunMessages(999)
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages for nonexistent run, got %d", len(msgs))
	}
}

func TestGetBotStatsEmpty(t *testing.T) {
	d := openTestDB(t)
	stats := d.GetBotStats("nonexistent")
	if stats.Runs != 0 || stats.TotalCost != 0 || stats.TotalTurns != 0 || stats.LastRun != "" {
		t.Fatalf("expected zero stats, got %+v", stats)
	}
}

func TestGetBotStatsMultipleRuns(t *testing.T) {
	d := openTestDB(t)

	id1, _, _ := d.BeginRun("stats-bot", "r1.log")
	d.EndRun(id1, "s1", 1000, 0.10, 5)

	id2, _, _ := d.BeginRun("stats-bot", "r2.log")
	d.EndRun(id2, "s2", 2000, 0.20, 8)

	stats := d.GetBotStats("stats-bot")
	if stats.Runs != 2 {
		t.Fatalf("expected 2 runs, got %d", stats.Runs)
	}
	if stats.TotalCost < 0.29 || stats.TotalCost > 0.31 {
		t.Fatalf("expected total cost ~0.30, got %f", stats.TotalCost)
	}
	if stats.TotalTurns != 13 {
		t.Fatalf("expected 13 total turns, got %d", stats.TotalTurns)
	}
}

func TestLatestRunTurns(t *testing.T) {
	d := openTestDB(t)

	turns := d.LatestRunTurns("no-bot")
	if turns != 0 {
		t.Fatalf("expected 0 turns for nonexistent bot, got %d", turns)
	}

	id, _, _ := d.BeginRun("turn-bot", "t.log")
	d.EndRun(id, "s1", 1000, 0.01, 7)

	turns = d.LatestRunTurns("turn-bot")
	if turns != 7 {
		t.Fatalf("expected 7 turns, got %d", turns)
	}
}

func TestLatestRunTurnsLiveEstimate(t *testing.T) {
	d := openTestDB(t)

	runID, _, _ := d.BeginRun("live-bot", "t.log")

	d.InsertMessage(runID, 1, "m1", "claude", 0, 0, 0, 0, `{}`)
	d.InsertMessage(runID, 2, "m2", "claude", 0, 0, 0, 0, `{}`)

	turns := d.LatestRunTurns("live-bot")
	if turns != 2 {
		t.Fatalf("expected 2 live turns, got %d", turns)
	}
}

func TestRecentRunLogs(t *testing.T) {
	d := openTestDB(t)

	d.BeginRun("logs-bot", "r1.log")
	d.BeginRun("logs-bot", "r2.log")
	d.BeginRun("logs-bot", "r3.log")

	files := d.RecentRunLogs("logs-bot", 2)
	if len(files) != 2 {
		t.Fatalf("expected 2 log files, got %d", len(files))
	}
	if files[0] != "r3.log" || files[1] != "r2.log" {
		t.Fatalf("expected newest first: [r3.log, r2.log], got %v", files)
	}
}

func TestLatestRunLog(t *testing.T) {
	d := openTestDB(t)

	if log := d.LatestRunLog("no-bot"); log != "" {
		t.Fatalf("expected empty, got %q", log)
	}

	d.BeginRun("ll-bot", "first.log")
	d.BeginRun("ll-bot", "second.log")

	if log := d.LatestRunLog("ll-bot"); log != "second.log" {
		t.Fatalf("expected 'second.log', got %q", log)
	}
}

func TestPruneRuns(t *testing.T) {
	d := openTestDB(t)

	d.BeginRun("prune-bot", "r1.log")
	d.BeginRun("prune-bot", "r2.log")
	d.BeginRun("prune-bot", "r3.log")
	d.BeginRun("prune-bot", "r4.log")
	d.BeginRun("prune-bot", "r5.log")

	deleted := d.PruneRuns("prune-bot", 2)
	if len(deleted) != 3 {
		t.Fatalf("expected 3 deleted log files, got %d", len(deleted))
	}

	stats := d.GetBotStats("prune-bot")
	if stats.Runs != 2 {
		t.Fatalf("expected 2 runs after prune, got %d", stats.Runs)
	}

	remaining := d.RecentRunLogs("prune-bot", 10)
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining log files, got %d", len(remaining))
	}
}

func TestBotState(t *testing.T) {
	d := openTestDB(t)

	state, sessID, pause := d.GetBotState("new-bot")
	if state != "" || sessID != "" || pause {
		t.Fatalf("expected empty state for unknown bot, got %q %q %v", state, sessID, pause)
	}

	d.SetBotState("new-bot", "running")
	state, _, _ = d.GetBotState("new-bot")
	if state != "running" {
		t.Fatalf("expected 'running', got %q", state)
	}

	d.SetBotState("new-bot", "paused")
	state, _, _ = d.GetBotState("new-bot")
	if state != "paused" {
		t.Fatalf("expected 'paused', got %q", state)
	}
}

func TestBotSessionID(t *testing.T) {
	d := openTestDB(t)

	d.SetBotSessionID("sess-bot", "session-123")
	_, sessID, _ := d.GetBotState("sess-bot")
	if sessID != "session-123" {
		t.Fatalf("expected session-123, got %q", sessID)
	}
}

func TestPauseRequested(t *testing.T) {
	d := openTestDB(t)

	d.SetPauseRequested("pause-bot", true)
	_, _, pause := d.GetBotState("pause-bot")
	if !pause {
		t.Fatal("expected pause=true")
	}

	d.SetPauseRequested("pause-bot", false)
	_, _, pause = d.GetBotState("pause-bot")
	if pause {
		t.Fatal("expected pause=false after clearing")
	}
}

func TestClearBotState(t *testing.T) {
	d := openTestDB(t)

	d.SetBotState("clear-bot", "running")
	d.SetBotSessionID("clear-bot", "sess-x")
	d.SetPauseRequested("clear-bot", true)

	d.ClearBotState("clear-bot")
	state, sessID, pause := d.GetBotState("clear-bot")
	if state != "stopped" {
		t.Fatalf("expected 'stopped', got %q", state)
	}
	if sessID != "" {
		t.Fatalf("expected empty session, got %q", sessID)
	}
	if pause {
		t.Fatal("expected pause=false after clear")
	}
}

func TestDeleteBotData(t *testing.T) {
	d := openTestDB(t)

	runID, _, _ := d.BeginRun("del-bot", "d.log")
	d.EndRun(runID, "s1", 1000, 0.01, 1)
	d.InsertMessage(runID, 1, "m1", "claude", 0, 0, 0, 0, `{}`)
	d.InsertLogEntry("del-bot", runID, "info", "hi", "")
	d.SetPID("del-bot", 111)
	d.SetBotState("del-bot", "running")
	d.EnqueueMessage("del-bot", "msg")

	err := d.DeleteBotData("del-bot")
	if err != nil {
		t.Fatalf("DeleteBotData: %v", err)
	}

	stats := d.GetBotStats("del-bot")
	if stats.Runs != 0 {
		t.Fatalf("expected 0 runs after delete, got %d", stats.Runs)
	}

	_, ok := d.GetPID("del-bot")
	if ok {
		t.Fatal("expected no PID after delete")
	}

	entries := d.RecentLogEntries("del-bot", 10)
	if len(entries) != 0 {
		t.Fatalf("expected 0 log entries after delete, got %d", len(entries))
	}

	msgs := d.RunMessages(runID)
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after delete, got %d", len(msgs))
	}

	state, _, _ := d.GetBotState("del-bot")
	if state != "" {
		t.Fatalf("expected empty state after delete, got %q", state)
	}
}

func TestCascadeDeleteRunMessages(t *testing.T) {
	d := openTestDB(t)

	runID, _, _ := d.BeginRun("cascade-bot", "c.log")
	d.InsertMessage(runID, 1, "m1", "claude", 0, 0, 0, 0, `{"test":true}`)
	d.InsertLogEntry("cascade-bot", runID, "info", "test", "")

	pruned := d.PruneRuns("cascade-bot", 0)
	if len(pruned) != 1 {
		t.Fatalf("expected 1 pruned file, got %d", len(pruned))
	}

	msgs := d.RunMessages(runID)
	if len(msgs) != 0 {
		t.Fatalf("expected messages cascaded deleted, got %d", len(msgs))
	}

	runEntries := d.RunLogEntries(runID)
	if len(runEntries) != 0 {
		t.Fatalf("expected log entries cascaded deleted, got %d", len(runEntries))
	}
}

func TestMultipleBotsIsolation(t *testing.T) {
	d := openTestDB(t)

	d.BeginRun("bot-1", "b1.log")
	d.BeginRun("bot-1", "b1-2.log")
	d.BeginRun("bot-2", "b2.log")

	s1 := d.GetBotStats("bot-1")
	s2 := d.GetBotStats("bot-2")
	if s1.Runs != 2 {
		t.Fatalf("bot-1 expected 2 runs, got %d", s1.Runs)
	}
	if s2.Runs != 1 {
		t.Fatalf("bot-2 expected 1 run, got %d", s2.Runs)
	}

	d.EnqueueMessage("bot-1", "for-1")
	d.EnqueueMessage("bot-2", "for-2")
	msg := d.DequeueMessage("bot-1")
	if msg != "for-1" {
		t.Fatalf("expected 'for-1', got %q", msg)
	}
	msg = d.DequeueMessage("bot-2")
	if msg != "for-2" {
		t.Fatalf("expected 'for-2', got %q", msg)
	}
}
