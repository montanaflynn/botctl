# botctl

Process manager for autonomous AI agent bots. Run persistent agents from a single CLI with a terminal dashboard, web UI, and declarative configuration.

**[Website](https://botctl.dev)** &middot; **[Docs](https://botctl.dev/docs)** &middot; **[Releases](https://github.com/montanaflynn/botctl/releases)**

## Install

```bash
curl -fsSL https://botctl.dev/install.sh | sh
```

Or with Go:

```bash
go install github.com/montanaflynn/botctl@latest
```

Pre-built binaries for macOS, Linux, and Windows are on the [releases](https://github.com/montanaflynn/botctl/releases) page.

## Quick Start

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

## CLI Commands

| Command | Description |
|---------|-------------|
| `botctl` | Open the TUI dashboard (`--web-ui` for web dashboard, `--port` to set port) |
| `botctl create [name]` | Create a new bot via Claude (`-d` description, `-i` interval, `-m` max turns) |
| `botctl start [name]` | Start a bot (`-d` detach, `-m` message, `--once` single run) |
| `botctl stop [name]` | Stop a bot (no args = stop all) |
| `botctl list` | List bots with status |
| `botctl status` | Detailed status of all bots |
| `botctl logs [name]` | View logs (`-n` lines, `-f` follow) |
| `botctl delete <name>` | Delete a bot and its data (`-y` skip confirmation) |
| `botctl update` | Self-update to the latest release |

## TUI Keybindings

| Key | Action |
|-----|--------|
| `s` | Start/stop selected bot |
| `r` | Resume (editable turn count) |
| `m` / `enter` | Send message to bot |
| `n` | Create new bot |
| `c` | Clear bot logs and runs |
| `d` | Delete bot (with confirmation) |
| `o` | Open bot directory |
| `f` / `tab` | Focus filter bar |
| `j` / `k` | Navigate bot list |
| `q` | Quit |

## BOT.md Configuration

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

You are an autonomous agent that...
```

### Frontmatter Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | required | Display name |
| `id` | string | folder name | Stable database key (survives folder renames) |
| `interval_seconds` | int | 60 | Seconds between runs |
| `max_turns` | int | 0 (unlimited) | Turn limit per run |
| `workspace` | string | `local` | `local` (per-bot) or `shared` (`~/.botctl/workspace/`) |
| `skills_dir` | string | — | Relative path to skill markdown files |
| `log_retention` | int | 30 | Number of run logs to keep |
| `env` | map | — | Environment variables (supports `${VAR}` references) |

The markdown body becomes the bot's system prompt. Claude sees it every run.

## How It Works

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

Config changes (including `max_turns`) take effect on the next run without restarting.

### Resume

When a run hits `max_turns`, the session ID is saved. Press `r` in the TUI to resume with an editable turn limit.

### Skills

Skills are markdown files in `skills_dir` that get injected into the bot's system prompt:

```
my-bot/
  BOT.md
  skills/
    api-access.md
    safety-rules.md
  workspace/
```

## File Layout

```
~/.botctl/
  botctl.db              # SQLite database
  workspace/             # Shared workspace
  bots/
    my-bot/
      BOT.md             # Bot config + prompt
      skills/            # Skill files (optional)
      workspace/         # Local workspace
      logs/              # Per-run log files
```

## Web Dashboard

```bash
botctl --web-ui              # default port 4444
botctl --web-ui --port 8080  # custom port
```

## Updates

```bash
botctl update
```

Checks for new releases and replaces the binary in-place.

## License

MIT
