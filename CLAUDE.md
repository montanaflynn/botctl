# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build .              # Build the binary
go test ./...           # Run all tests
go vet ./...            # Static analysis
go test ./pkg/config/   # Run tests for a single package
```

Version is injected at build time: `go build -ldflags "-X main.version=1.0.0" .`

Releases use goreleaser (`.goreleaser.yml`), triggered by git tags matching `v*`.

## Architecture

**botctl** is a process manager for autonomous AI agent bots. It runs persistent agents from a CLI with a TUI dashboard, web UI, and declarative `BOT.md` configuration.

### Two-Repo Model

This repo (`botctl`) is the open-source CLI. A private repo (`botctl-platform`) imports `pkg/` to build a hosted multi-tenant platform. This means:

- **`pkg/`** is the public API surface — importable by `botctl-platform`. Changes here affect both projects.
- **`internal/`** is CLI-specific and not importable outside this module.

### Package Roles

**`pkg/` (shared library):**
- `config` — Parses `BOT.md` files (YAML frontmatter + markdown body). Handles `${VAR}` env expansion and defaults.
- `backend` — Pluggable agent runtime abstraction. `backend.Backend` interface with `Query()`, provider-neutral `MessageEvent`/`ContentBlock`/`Options`/`Result` types. Subpackages `claude/` (wraps the inlined `claude/sdk/` wrapper around the Claude CLI) and `opencode/` (spawns `opencode run --format json`) self-register via `init()`. Selected per-bot via `backend:` in BOT.md frontmatter (default `claude`).
- `harness` — Core execution loop. Reloads config each iteration, runs the configured backend's `Query()`, records stats, handles pause/resume/interrupt, sleeps for interval.
- `db` — SQLite layer (WAL mode). Tables: runs, messages, log_entries, bot_state. Tracks run lifecycle, bot state + session, structured logs, cost/turn stats.
- `service` — Business logic facade over db + process. Start/stop/pause/play/message bots, query stats and logs.
- `process` — Spawns harness as a subprocess, manages PID lifecycle, SIGTERM→SIGKILL with timeout. Platform-specific files for Unix vs Windows.
- `discovery` — Scans `~/.botctl/bots/` for valid BOT.md files.
- `skills` — Discovers skills from 3 locations (first match wins): `~/.agents/skills/`, `~/.botctl/skills/`, per-bot `skills_dir/`.
- `paths` — Resolves all filesystem paths. `BOTCTL_HOME` env var overrides the default `~/.botctl/` root (used in tests).
- `logs` — Log rendering and formatting.
- `create` — Bot creation helpers.

**`internal/` (CLI-only):**
- `cli` — Cobra command handlers. Each command uses `withService()` to open DB and create Service.
- `tui` — Bubble Tea terminal UI with table, live log viewer, modals.
- `web` — HTTP dashboard on port 4444 with embedded static files and SSE log streaming.
- `website` — Documentation site builder.
- `update` — Self-update mechanism.

**`pkg/backend/claude/sdk/`:**
Inlined Go wrapper around the Claude CLI (stream-json over stdin/stdout). Used by `pkg/backend/claude` (the backend adapter) and `pkg/create` (interactive BOT.md generation). See `pkg/backend/claude/sdk/CLAUDE.md` for details.

### Key Flows

**Bot execution:** `cli.start` → `process.StartBot()` spawns `botctl harness` subprocess → `harness.Run()` loops: reload config → check message queue → `backend.Get(cfg.Backend).Query()` → record stats → sleep for interval.

**State transitions:** stopped → running → sleeping ↔ paused. Session IDs are persisted for resume after max_turns.

**Operator messages:** Queued in DB via `service.SendMessage()`, picked up by harness on next iteration, formatted as prominent headings in the Claude prompt.

## Conventions

- Commit messages use conventional commits (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`).
- Tests use `t.TempDir()` and set `BOTCTL_HOME` env var to isolate filesystem paths.
- Platform-specific code uses build-tagged files: `platform_unix.go`, `platform_windows.go`.
- CI runs on ubuntu, macos, and windows — use `filepath.Join` for path construction, not hardcoded slashes.
