package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/montanaflynn/botctl-go/internal/config"
	"github.com/montanaflynn/botctl-go/internal/db"
	"github.com/montanaflynn/botctl-go/internal/logs"
	"github.com/montanaflynn/botctl-go/internal/paths"
	claude "github.com/montanaflynn/claude-agent-sdk-go"
)

// resolveWorkspace returns the workspace directory for a bot.
func resolveWorkspace(botDir string, cfg *config.BotConfig) string {
	ws := cfg.Workspace
	if ws == "" {
		ws = "local"
	}
	if ws == "shared" {
		return paths.WorkspaceDir()
	}
	return filepath.Join(botDir, "workspace")
}

// formatToolUse returns a clean heading representation of a tool call.
func formatToolUse(name, inputJSON string) string {
	var input map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return "### " + name + "\n" + inputJSON
	}

	switch name {
	case "Bash":
		cmd, _ := input["command"].(string)
		desc, _ := input["description"].(string)
		header := "### Bash"
		if desc != "" {
			header += " — " + desc
		}
		if cmd != "" {
			return header + "\n" + cmd
		}
		return header

	case "Read":
		fp, _ := input["file_path"].(string)
		return "### Read\n" + fp

	case "Write":
		fp, _ := input["file_path"].(string)
		return "### Write\n" + fp

	case "Edit":
		fp, _ := input["file_path"].(string)
		return "### Edit\n" + fp

	case "Glob":
		pattern, _ := input["pattern"].(string)
		return "### Glob\n" + pattern

	case "Grep":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		s := "### Grep\n" + pattern
		if path != "" {
			s += " in " + path
		}
		return s

	default:
		compact, err := json.Marshal(input)
		if err == nil {
			return "### " + name + "\n" + string(compact)
		}
		return "### " + name + "\n" + inputJSON
	}
}

// splitFormatted splits a formatToolUse string into heading and body.
func splitFormatted(s string) (heading, body string) {
	if i := strings.Index(s, "\n"); i >= 0 {
		heading = strings.TrimPrefix(s[:i], "### ")
		body = s[i+1:]
	} else {
		heading = strings.TrimPrefix(s, "### ")
	}
	return
}

// runTask executes a single Claude query for the bot.
// If feedback is non-empty, it is appended to the prompt.
// If resumeSession is non-empty, the task resumes that session instead of starting fresh.
// If interruptCh is non-nil, the query can be interrupted between turns.
func runTask(botDir string, cfg *config.BotConfig, workspace string, runID int64, database *db.DB, botID string, feedback string, resumeSession string, interruptCh <-chan struct{}) *claude.ResultMessage {
	var skillsLine string
	if cfg.SkillsDir != "" {
		skillsPath := filepath.Join(botDir, cfg.SkillsDir)
		if abs, err := filepath.Abs(skillsPath); err == nil {
			skillsPath = abs
		}
		skillsLine = fmt.Sprintf(
			"Your skills directory is %s. Read every file in it — each one is an instruction you must follow.",
			skillsPath,
		)
	}

	systemPrompt := fmt.Sprintf(
		"You are an autonomous agent managed by `botctl`.\nWorkspace directory: %s\n%s\n\nYour full instructions are in the user message below. Follow them.",
		workspace, skillsLine,
	)
	if cfg.MaxTurns > 0 {
		systemPrompt += fmt.Sprintf("\nYou have a maximum of %d turns. Plan your work to complete within this limit.", cfg.MaxTurns)
	}

	opts := claude.Options{
		SystemPrompt:   systemPrompt,
		Cwd:            workspace,
		PermissionMode: "bypassPermissions",
		MaxBufferSize:  10 * 1024 * 1024,
		InterruptCh:    interruptCh,
	}
	if cfg.MaxTurns > 0 {
		opts.MaxTurns = cfg.MaxTurns
	}

	var seq atomic.Int64

	opts.EnvelopeHandler = func(env claude.AssistantEnvelope) {
		msg := env.Message

		// Log to stdout and write structured entries to DB
		for _, block := range msg.Content {
			switch {
			case block.IsText():
				fmt.Println(block.Text)
				if database != nil && botID != "" {
					database.InsertLogEntry(botID, runID, "text", "", block.Text)
				}
			case block.IsToolUse():
				formatted := formatToolUse(block.Name, block.InputJSON())
				fmt.Println(formatted)
				if database != nil && botID != "" {
					h, b := splitFormatted(formatted)
					database.InsertLogEntry(botID, runID, "tool_use", h, b)
				}
			case block.IsToolResult():
				if block.IsError {
					content := block.ContentString()
					fmt.Printf("#### Error\n%s\n", content)
					if database != nil && botID != "" {
						database.InsertLogEntry(botID, runID, "tool_error", "", content)
					}
				} else {
					content := block.ContentString()
					if len(content) > 500 {
						content = content[:500] + fmt.Sprintf("... (%d chars)", len(content))
					}
					if content != "" {
						fmt.Printf("#### Result\n%s\n", content)
						if database != nil && botID != "" {
							database.InsertLogEntry(botID, runID, "tool_result", "", content)
						}
					}
				}
			}
		}

		// Store raw message in database
		if runID > 0 && database != nil {
			n := int(seq.Add(1))
			var inputTokens, outputTokens, cacheCreation, cacheRead int
			if msg.Usage != nil {
				inputTokens = msg.Usage.InputTokens
				outputTokens = msg.Usage.OutputTokens
				cacheCreation = msg.Usage.CacheCreationInputTokens
				cacheRead = msg.Usage.CacheReadInputTokens
			}
			if err := database.InsertMessage(runID, n, msg.ID, msg.Model, inputTokens, outputTokens, cacheCreation, cacheRead, env.RawJSON); err != nil {
				fmt.Printf("warning: failed to store message: %v\n", err)
			}
		}
	}

	var prompt string
	if resumeSession != "" {
		// Resume a previous session (e.g. after max turns)
		opts.SessionID = resumeSession
		prompt = fmt.Sprintf("## Max turns reached (%d/%d)\n## Resumed by operator", cfg.MaxTurns, cfg.MaxTurns)
		if feedback != "" {
			prompt = formatOperatorMessage(feedback)
		}
		fmt.Printf("## Resuming session\n%s\n", resumeSession)
		if database != nil && botID != "" {
			database.InsertLogEntry(botID, runID, "resume", "Resuming session", resumeSession)
		}
	} else {
		prompt = cfg.Body
		if feedback != "" {
			prompt += "\n\n" + formatOperatorMessage(feedback)
		}
	}

	result, err := claude.Query(context.Background(), prompt, opts, nil)
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		if database != nil && botID != "" {
			database.InsertLogEntry(botID, runID, "error", "", err.Error())
		}
		return nil
	}

	if result != nil {
		costLine := fmt.Sprintf("$%.4f | %d turns | %.1fs",
			result.TotalCostUSD, result.NumTurns, float64(result.DurationMS)/1000)
		fmt.Printf("\n---\n%s\n", costLine)
		if database != nil && botID != "" {
			database.InsertLogEntry(botID, runID, "cost", "", costLine)
		}
	} else {
		fmt.Println("warning: no result message received from CLI")
		if database != nil && botID != "" {
			database.InsertLogEntry(botID, runID, "warning", "", "no result message received from CLI")
		}
	}

	return result
}

// formatOperatorMessage wraps feedback in a prominent heading block.
func formatOperatorMessage(feedback string) string {
	return "## OPERATOR MESSAGE — READ AND RESPOND IMMEDIATELY\n\n" +
		"The following message is from the operator who manages you. " +
		"Prioritize this over any routine tasks:\n\n" +
		feedback
}

// Run is the main entry point for the harness — called by `botctl harness <bot_dir>`.
// If once is true, the harness runs a single task and exits.
// If message is non-empty, it is appended to the first run's prompt.
func Run(botDir string, once bool, message string) error {
	absDir, err := filepath.Abs(botDir)
	if err != nil {
		return fmt.Errorf("resolve bot dir: %w", err)
	}

	initCfg, err := config.FromMD(filepath.Join(absDir, "BOT.md"))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Folder name for display and file paths
	name := filepath.Base(absDir)

	// Stable ID for database records (survives folder renames)
	id := initCfg.ID
	if id == "" {
		id = name
	}

	workspace := resolveWorkspace(absDir, initCfg)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	database, err := db.Open()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	// Ensure log directory exists
	logDir := paths.BotLogDir(name)
	if initCfg.LogDir != "" {
		logDir = initCfg.LogDir
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	fmt.Printf("%s started at %s\n", name, time.Now().Format(time.RFC3339))
	fmt.Printf("  workspace: %s\n", workspace)

	// Persistent SIGUSR1 handler — shared across sleep and run phases
	wakeCh := make(chan os.Signal, 1)
	signal.Notify(wakeCh, syscall.SIGUSR1)
	defer signal.Stop(wakeCh)

	var lastSessionID string // set when a run hits max turns or is interrupted

	for {
		// Reload config each iteration so BOT.md edits take effect without restart
		cfg, err := config.FromMD(filepath.Join(absDir, "BOT.md"))
		if err != nil {
			fmt.Printf("warning: failed to reload config, using previous: %v\n", err)
			cfg = initCfg
		}

		// Log filename for post-run dump
		ts := time.Now().Format("20060102-150405")
		logFilename := ts + ".log"

		// Record run start in db
		runID, runNumber, err := database.BeginRun(id, logFilename)
		if err != nil {
			fmt.Printf("warning: failed to begin run: %v\n", err)
		}

		// Use CLI --message on first run, then drain DB queue
		feedback := message
		message = ""
		if feedback == "" {
			feedback = database.DequeueAllMessages(id)
		}
		if feedback != "" {
			fmt.Printf("## Feedback\n%s\n", feedback)
			database.InsertLogEntry(id, runID, "feedback", "Feedback", feedback)
		}

		// Check for resume command with optional turn count (supports "resume" and "resume:N")
		resumeSession := ""
		trimmedFeedback := strings.TrimSpace(feedback)
		if strings.HasPrefix(strings.ToLower(trimmedFeedback), "resume") {
			// Extract optional turn count override
			if parts := strings.SplitN(trimmedFeedback, ":", 2); len(parts) == 2 {
				if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && n > 0 {
					cfg.MaxTurns = n
				}
			}
			// Resume is just a message — replace with a human-readable prompt
			feedback = "Resumed by operator"
		}

		// Auto-resume: if we have a saved session and feedback, resume with it
		if lastSessionID != "" && feedback != "" {
			resumeSession = lastSessionID
		}

		runHeader := fmt.Sprintf("Run #%d", runNumber)
		fmt.Printf("\n## %s\n", runHeader)
		database.InsertLogEntry(id, runID, "run_header", runHeader, "")

		// Set up per-run interrupt channel: forwards SIGUSR1 to SDK
		interruptCh := make(chan struct{})
		stopForward := make(chan struct{})
		go func() {
			for {
				select {
				case <-wakeCh:
					// Only interrupt if there are pending messages
					if database.HasPendingMessages(id) {
						select {
						case interruptCh <- struct{}{}:
						default:
						}
						return
					}
					// Signal received but no messages — ignore (spurious wake)
				case <-stopForward:
					return
				}
			}
		}()

		result := runTask(absDir, cfg, workspace, runID, database, id, feedback, resumeSession, interruptCh)

		close(stopForward)

		var sessionID string
		var durationMS int64
		var costUSD float64
		var turns int
		wasInterrupted := result != nil && result.Interrupted
		if result != nil {
			sessionID = result.SessionID
			durationMS = int64(result.DurationMS)
			costUSD = result.TotalCostUSD
			turns = result.NumTurns
		} else if cfg.MaxTurns > 0 {
			turns = cfg.MaxTurns
		}
		if runID > 0 {
			if err := database.EndRun(runID, sessionID, durationMS, costUSD, turns); err != nil {
				fmt.Printf("warning: failed to end run: %v\n", err)
			}
		}

		// Store session ID for resume on interrupt or max turns
		if wasInterrupted {
			if sessionID != "" {
				lastSessionID = sessionID
			}
			interruptMsg := "Interrupted by operator message"
			fmt.Printf("## %s\n", interruptMsg)
			database.InsertLogEntry(id, runID, "interrupted", interruptMsg, "")
		} else if cfg.MaxTurns > 0 && (result == nil || result.NumTurns >= cfg.MaxTurns) {
			if sessionID != "" {
				lastSessionID = sessionID
			}
			maxMsg := fmt.Sprintf("Max turns reached (%d/%d)", turns, cfg.MaxTurns)
			fmt.Printf("## %s\nPress r to resume\n", maxMsg)
			database.InsertLogEntry(id, runID, "max_turns", maxMsg, "Press r to resume")
		} else {
			lastSessionID = ""
		}

		// Write log file from DB entries after run
		if runID > 0 {
			entries := database.RunLogEntries(runID)
			if len(entries) > 0 {
				content := logs.RenderEntries(entries)
				logPath := filepath.Join(logDir, logFilename)
				os.WriteFile(logPath, []byte(content), 0o644)
			}
		}

		// Prune old run logs
		deleted := database.PruneRuns(id, cfg.LogRetention)
		for _, f := range deleted {
			fp := filepath.Join(logDir, f)
			os.Remove(fp)
		}

		if once {
			break
		}

		// Skip sleep if messages are pending (e.g. after interrupt)
		if database.HasPendingMessages(id) {
			continue
		}

		sleepMsg := fmt.Sprintf("sleeping %ds...", cfg.IntervalSeconds)
		fmt.Println(sleepMsg)
		database.InsertLogEntry(id, 0, "sleep", "", sleepMsg)

		sleepUntilWake(cfg.IntervalSeconds, wakeCh)
	}

	return nil
}

// sleepUntilWake sleeps for the given duration but can be woken early
// by a signal on the provided channel (SIGUSR1 sent by TUI when a message is queued).
func sleepUntilWake(seconds int, wakeCh <-chan os.Signal) {
	select {
	case <-wakeCh:
	case <-time.After(time.Duration(seconds) * time.Second):
	}
}
