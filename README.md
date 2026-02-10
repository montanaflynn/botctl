# `botctl` - control autonomous agent bots

Process manager for autonomous bots. Run persistent agents from a single CLI (`botctl`) with a terminal dashboard and declaritive bot configuration protocol.

## Install

```
go install github.com/montanaflynn/botctl/v1
```

## Glossary

| Term | Definition |
|------|-----------|
| **Bot** | An ai-powered agent. Lives at `~/.botctl/bots/{name}/` with a `BOT.md` config file. |
| **Harness** | The background process that runs and interacts with a bots loop, start|stop|interrupt|resume|restart logic. |
| **Run** | One iteration of the harness loop. A run starts when the harness wakes up and ends when it goes back to sleep. Each run produces a log file. |
| **Turn** | A single assistant response from Claude within a run. A run with `max_turns: 10` allows Claude to respond 10 times (each response may include tool calls). |
| **Session** | Claude's conversation context. Includes all messages exchanged so far. Identified by a session ID. |
| **Resume** | Continuing a session after it hit `max_turns`. The harness picks up where Claude left off using the saved session ID. |
| **Skill** | A markdown file in the bot's `skills_dir` that gets injected as instructions into Claude's system prompt. |
| **Workspace** | The directory where a bot reads and writes files. Can be `local` (per-bot) or `shared` (common across bots). |

## Quick start

```bash
# Create a bot (interactive)
botctl create my-bot

# Start the TUI dashboard
botctl

# Or start a specific bot in the background
botctl start my-bot --detach

# Send a message to a running bot
botctl start my-bot --message "check the error logs"

# Run once and exit
botctl start my-bot --once

# View logs
botctl logs my-bot -f

# Stop
botctl stop my-bot
```

## CLI commands

| Command | Description |
|---------|-------------|
| `botctl` | Open the TUI dashboard |
| `botctl create [name]` | Create a new bot via Claude (`-d` description, `-i` interval, `-m` max turns) |
| `botctl start [name]` | Start a bot (`-d` detach, `-m` message, `--once` single run) |
| `botctl stop [name]` | Stop a bot (no args = stop all) |
| `botctl list` | List bots with status |
| `botctl status` | Detailed status of all bots |
| `botctl logs [name]` | View logs (`-n` lines, `-f` follow) |
| `botctl delete <name>` | Delete a bot and its data (`-y` skip confirmation) |

## TUI keybindings

| Key | Action |
|-----|--------|
| `s` | Start/stop selected bot |
| `r` | Restart selected bot |
| `u` | Resume (editable turn count) |
| `m` | Send message to bot |
| `c` | Create new bot |
| `o` | Open bot directory |
| `f` | Filter bots by name |
| `up`/`down` or `j`/`k` | Navigate bot list |
| `tab` | Switch focus between table and logs |
| `enter` | Sort column (when on header) |
| `q` | Quit |

## BOT.md format

Each bot is defined by a `BOT.md` file with YAML frontmatter and a markdown body:

```markdown
---
name: my-bot
id: my-bot-001
interval_seconds: 300
max_turns: 20
workspace: local
skills_dir: ./skills
log_retention: 30
---

# My Bot

You are an autonomous agent that...

## Steps

- Reads tasks.md
- Does the tasks, marking them as done and leaving summary under it. Use subagents if you want, just make sure each one only changes it's tasks and does git worktree so it's not interferring. If you're marking a task as done you also merge to main and delete the worktree after your task is done.

## Finishing tasks

- Always plan how to accomplish a task effeciently. Use your skills effeciently, prefer cli tools.
- If applicable create tests, documentation, and other supporting new files or update existing files.
```

### Frontmatter fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Display name |
| `id` | string | folder name | Stable database key (survives folder renames) |
| `interval_seconds` | int | 60 | Seconds between runs |
| `max_turns` | int | 0 (unlimited) | Turn limit per run. When hit, the session is saved and can be resumed with `u`. |
| `workspace` | string | `local` | `local` = per-bot directory, `shared` = `~/.botctl/workspace/` |
| `skills_dir` | string | — | Relative path to skill markdown files |
| `log_dir` | string | `logs/` | Custom log directory |
| `log_retention` | int | 30 | Number of run logs to keep before pruning |
| `env` | map | — | Environment variables (supports `${VAR}` references) |

### Markdown body

The body becomes the bot's prompt. Claude sees it every run along with the system prompt (workspace path, skills, turn limit).

## How it works

```
┌─────────────────────────────────────────┐
│              Harness Loop               │
│                                         │
│  1. Reload BOT.md config                │
│  2. Check message queue                 │
│  3. Run Claude task (up to max_turns)   │
│  4. Record stats (cost, turns, session) │
│  5. Write log file                      │
│  6. Sleep for interval_seconds          │
│     (woken early by messages/resume)    │
│  7. Repeat                              │
│                                         │
└─────────────────────────────────────────┘
```

The harness reloads `BOT.md` every iteration, so config changes (including `max_turns`) take effect on the next run without restarting.

### Resume flow

When a run hits `max_turns`, the session ID is saved. Pressing `u` in the TUI opens an input pre-filled with the current `max_turns` from BOT.md. Edit the number and press enter to resume the session with that turn limit.

### Skills

Skills are markdown files in the bot's `skills_dir`. The harness tells Claude to read every file in the directory. Each skill is an instruction set the bot follows.

```
my-bot/
  BOT.md
  skills/
    api-access.md
    safety-rules.md
  workspace/
    state.json
```

### Workspaces

- **Local** (`workspace: local`): `~/.botctl/bots/{name}/workspace/` — private to this bot
- **Shared** (`workspace: shared`): `~/.botctl/workspace/` — shared across all bots that opt in

Claude's working directory is set to the workspace. All file operations happen there by default.

## File layout

```
~/.botctl/
  botctl.db                          # SQLite database (runs, logs, PIDs, messages)
  workspace/                     # Shared workspace
  bots/
    my-bot/
      BOT.md                     # Bot config + prompt
      skills/                    # Skill files (optional)
      workspace/                 # Local workspace
      logs/
        20260208-142305.log      # Per-run log files
        boot.log                 # Harness startup log
```

## Database

SQLite at `~/.botctl/botctl.db` with WAL mode. Stores:

- **runs** — execution history (duration, cost, turns, session ID)
- **pids** — active process tracking
- **messages** — raw Claude API responses
- **pending_messages** — message queue for operator-to-bot communication
- **log_entries** — structured log records

Set `MM_HOME` to override the default `~/.botctl` directory.
