# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This `sdk` package is a Go wrapper around the Claude CLI (`claude`) for programmatic use. It spawns the CLI as a subprocess, communicates via stream-json over stdin/stdout, and provides Go types for all message types in the protocol. Used by `pkg/backend/claude` (the claude backend adapter) and `pkg/create` (interactive BOT.md generation). Originally a separate repo (`claude-agent-sdk-go`) inlined into botctl.

## Build and Test

```bash
go build ./...
go test ./...
go vet ./...
```

No external dependencies — the module has zero `require` directives.

## Architecture

The entire SDK is three files in a single `claude` package:

- **`client.go`** — `Query()` is the sole entry point. It spawns `claude` with `--output-format stream-json --input-format stream-json --verbose`, sends an initialize request + user message via stdin, then reads stdout in a scan loop dispatching on message `type` (`assistant`, `result`, `control_request`, `user`). Handles max-turns enforcement, session resume (skips replayed messages when counting turns), process group cleanup via SIGTERM, and context cancellation.

- **`messages.go`** — All protocol types: `ContentBlock` (text/tool_use/tool_result), `AssistantMessage`, `AssistantEnvelope`, `ResultMessage`, `MessageEnvelope`, control request/response types, and the `MessageHandler`/`EnvelopeHandler` callback types.

- **`options.go`** — `Options` struct mapping to CLI flags (system prompt, allowed tools, permission mode, model, max turns, budget, session resume, continue). The `args()` method builds the CLI argument slice.

## Key Design Details

- The CLI is invoked as a child process with `Setpgid: true` so the entire process group can be killed on cancellation or max-turns.
- `control_request` messages with `subtype: "can_use_tool"` are auto-allowed (responds with `behavior: "allow"`).
- When resuming a session (`SessionID` set), the CLI replays prior messages. Turn counting only starts after the echoed `user` message to avoid counting replayed assistant turns toward `MaxTurns`.
- The scanner uses a 10MB max line buffer to handle large tool outputs.
- `EnvelopeHandler` takes priority over `MessageHandler` when both are provided — `EnvelopeHandler` gives access to `RawJSON`, `SessionID`, and `ParentToolUseID`.

## Platform Note

Uses `syscall.SysProcAttr{Setpgid: true}` and `syscall.Kill` with negative PID — this is Unix-only (macOS/Linux).
