# botctl

A process manager for autonomous AI agent bots. Each bot is defined by a `BOT.md` file with a YAML frontmatter config and a markdown system prompt. Bots run as background processes on a schedule, powered by Claude.

## Install

```sh
curl -fsSL https://botctl.dev/install.sh | sh
```

Supports macOS and Linux (amd64/arm64). Installs to `/usr/local/bin/botctl`.

## Quick Start

```sh
# Create a bot interactively (generates BOT.md via Claude)
botctl create my-bot

# Or create with flags
botctl create my-bot -d "Monitor disk usage and alert if above 90%" -i 300 -m 20

# Start a bot (opens TUI dashboard)
botctl start my-bot

# Start detached (background only)
botctl start my-bot -d

# Start with an initial message
botctl start my-bot -d -m "check the staging server"

# Run a single task and exit
botctl start my-bot --once -m "generate the weekly report"
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `botctl` | Open the TUI dashboard (`--web-ui` for browser, `--port N` to set port) |
| `botctl create [name]` | Create a new bot (`-d` description, `-i` interval, `-m` max turns) |
| `botctl start [name]` | Start a bot (`-d` detach, `-m` message, `--once` single run) |
| `botctl stop [name]` | Stop a bot (no args = stop all) |
| `botctl pause <name>` | Pause a running or sleeping bot |
| `botctl play <name>` | Resume a paused bot (`-t` turns, default 50) |
| `botctl list` | List all bots with status |
| `botctl status` | Detailed status of all bots |
| `botctl logs [name]` | View logs (`-n` lines, `-f` follow) |
| `botctl delete <name>` | Delete a bot and all its data (`-y` skip confirmation) |
| `botctl skills list` | List discovered skills (`--bot` to filter by bot) |
| `botctl skills search <query>` | Search skills.sh for community skills (`-n` limit) |
| `botctl skills add <owner/repo>` | Install skills from a GitHub repo (`--skill`, `--bot`, `--global`) |
| `botctl skills view <name>` | View a skill's SKILL.md and list its files |
| `botctl skills remove <name>` | Remove an installed skill |
| `botctl update` | Self-update to the latest release |

## BOT.md Format

Each bot lives in `~/.botctl/bots/<name>/` and is configured by a `BOT.md` file. YAML frontmatter sets config, the markdown body is the bot's system prompt.

```markdown
---
name: my-bot
interval_seconds: 300
max_turns: 20
workspace: local
---

# My Bot

You are a bot that does a specific task. Each run you should:

1. Check the current state
2. Take action if needed
3. Report what you did
```

### Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | folder name | Display name |
| `id` | string | folder name | Stable DB key (survives renames) |
| `interval_seconds` | int | 60 | Seconds between runs |
| `max_turns` | int | - | Max Claude API turns per run |
| `workspace` | string | - | `local` (per-bot) or `shared` (global) |
| `skills_dir` | string | - | Relative path to skill directories |
| `env` | map | - | Environment variables (`${VAR}` resolved from OS) |
| `log_retention` | int | 30 | Number of log files to keep |

### System Prompt

The markdown body after the frontmatter is the bot's system prompt. Claude sees it every run along with the workspace path, skill index, and turn limit. Edit the file and changes take effect on the next run without restarting.

## Workspaces

- `workspace: local` — per-bot directory at `~/.botctl/bots/<name>/workspace/`
- `workspace: shared` — global directory at `~/.botctl/workspace/`

The bot's Claude process runs with its working directory set to the resolved workspace.

## Skills

Skills are directories containing a `SKILL.md` file with YAML frontmatter (`name` and `description`) and a markdown body. The harness parses frontmatter from all discovered skills and lists them by name and description in the system prompt. Claude loads full skill content on-demand via the Skill tool.

Skills are discovered from three locations (first occurrence of a name wins):

1. `~/.agents/skills/` — cross-agent shared skills
2. `~/.botctl/skills/` — botctl-wide shared skills
3. Bot's `skills_dir` — per-bot skills

```
~/.botctl/bots/my-bot/
  BOT.md
  skills/
    research/
      SKILL.md
    formatting/
      SKILL.md
  workspace/
```

Skills are re-discovered every run — add, edit, or remove them without restarting the bot.

### Managing Skills

```sh
# Search the skills.sh registry for community skills
botctl skills search slack

# Install a skill from GitHub (default: ~/.botctl/skills/)
botctl skills add owner/repo --skill slack-notify

# Install to a specific bot's skills_dir
botctl skills add owner/repo --skill slack-notify --bot my-bot

# Install to ~/.agents/skills/ (shared across all agent tools)
botctl skills add owner/repo --skill slack-notify --global

# List all discovered skills
botctl skills list
botctl skills list --bot my-bot

# View a skill's SKILL.md content
botctl skills view slack-notify

# Remove a skill
botctl skills remove slack-notify
```

## Messaging

Send a message to any bot. If the bot is running or sleeping, it wakes immediately. If stopped, it auto-starts.

```sh
# Via CLI
botctl start my-bot -m "check the staging server now"

# One-shot task (runs once, streams logs, then exits)
botctl start my-bot --once -m "generate the weekly report"
```

In the TUI dashboard, press `m` to message the selected bot.

## Lifecycle

Bots cycle through these states:

- **running** — actively executing a Claude session
- **sleeping** — waiting for the next scheduled run
- **paused** — suspended, won't run until resumed
- **stopped** — not running, must be explicitly started

```sh
botctl start my-bot -d    # start in background
botctl pause my-bot       # pause (keeps process, stops scheduling)
botctl play my-bot        # resume from pause
botctl stop my-bot        # stop completely
```

## File Layout

```
~/.botctl/
  data/
    botctl.db                  # SQLite database (WAL mode)
  workspace/                   # Shared workspace
  bots/
    <name>/
      BOT.md                   # Bot config + system prompt
      skills/                  # Skill directories (if skills_dir set)
        my-skill/
          SKILL.md
      workspace/               # Local workspace (if workspace: local)
      logs/                    # Timestamped run logs
```

## Environment Variables

- `MM_HOME` — override the default `~/.botctl` data directory

Bot configs support an `env` map with `${VAR}` references resolved from the OS environment:

```yaml
env:
  API_KEY: ${MY_API_KEY}
  SLACK_WEBHOOK: ${SLACK_URL}
```
