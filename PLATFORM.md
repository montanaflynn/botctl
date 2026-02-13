# Platform

Hosted multi-tenant platform for running autonomous AI agent bots. Users create bots via `BOT.md` configs and manage them through a REST API, enabling mobile apps, web dashboards, and third-party integrations.

## Architecture

### Two-Repo Model

```
botctl/              (public, open source)
  pkg/                 shared library packages
    config/            bot configuration parsing
    create/            bot creation helpers
    db/                database layer
    discovery/         bot detection & status
    harness/           core execution loop
    logs/              log rendering & formatting
    paths/             file path resolution
    process/           process management
    service/           business logic facade
  internal/            CLI-specific (not importable)
    cli/               cobra commands
    tui/               terminal UI
    web/               local web UI
    website/           docs site builder
    update/            self-update

botctl-platform/     (private, closed source)
  imports github.com/montanaflynn/botctl/pkg/...
  cmd/botctld/         platform server binary
  internal/
    api/               REST API handlers + middleware
    auth/              authentication (API keys, JWT, OAuth)
    billing/           usage tracking, Stripe integration
    tenant/            multi-tenant isolation layer
    sandbox/           bot execution sandboxing
    queue/             job queue for bot runs
    notify/            webhooks, push notifications
    migrate/           database migrations (Postgres)
```

### Shared vs Platform-Specific

| Layer | Package | Shared (pkg/) | Platform-only |
|-------|---------|:---:|:---:|
| Config parsing | `config` | x | |
| Database ops | `db` | x | |
| Bot discovery | `discovery` | x | |
| Execution engine | `harness` | x | |
| Log formatting | `logs` | x | |
| Path resolution | `paths` | x | |
| Process mgmt | `process` | x | |
| Business logic | `service` | x | |
| Auth/identity | `auth` | | x |
| Billing/metering | `billing` | | x |
| Multi-tenancy | `tenant` | | x |
| Sandboxing | `sandbox` | | x |
| Job queue | `queue` | | x |
| Notifications | `notify` | | x |
| API layer | `api` | | x |

## API Design

### Authentication

Every request requires an API key in the `Authorization` header:

```
Authorization: Bearer btl_sk_...
```

API keys are scoped per-user with optional per-key permissions (read-only, manage, admin).

Future: OAuth2 for third-party app integrations.

### Endpoints

```
POST   /v1/bots                 Create a bot (BOT.md content in body)
GET    /v1/bots                 List user's bots
GET    /v1/bots/:id             Get bot details + status
PUT    /v1/bots/:id             Update bot config
DELETE /v1/bots/:id             Delete bot

POST   /v1/bots/:id/start      Start bot
POST   /v1/bots/:id/stop       Stop bot
POST   /v1/bots/:id/restart    Restart bot
POST   /v1/bots/:id/pause      Pause bot schedule
POST   /v1/bots/:id/play       Resume bot schedule

POST   /v1/bots/:id/messages   Send message to bot
POST   /v1/bots/:id/resume     Resume with optional turn override

GET    /v1/bots/:id/logs        Get log entries (paginated)
GET    /v1/bots/:id/logs/stream SSE stream of live logs
GET    /v1/bots/:id/runs        List runs with stats

GET    /v1/stats                Aggregate stats (total runs, cost, etc.)
GET    /v1/user                 Current user info + usage
```

### WebSocket (Future)

Real-time bidirectional channel for mobile/desktop clients:

```
ws://api.botctl.com/v1/bots/:id/ws
```

Events: log entries, status changes, run start/complete, message received.

## Database

### Migration from SQLite to Postgres

The `pkg/db` package currently uses SQLite. The platform wraps this with a Postgres-backed implementation that adds:

- `users` table (id, email, api_key_hash, plan, created_at)
- `api_keys` table (id, user_id, key_hash, name, permissions, last_used_at)
- `user_id` foreign key on all existing tables (bots, runs, log_entries, etc.)
- `usage_events` table for billing (user_id, event_type, units, timestamp)

Options:
1. **Adapter pattern** — define a `Store` interface in `pkg/db`, implement for both SQLite (local) and Postgres (platform)
2. **Postgres only** for platform — fork the db layer, keep SQLite for local CLI

Option 1 is cleaner long-term but more upfront work. Option 2 ships faster.

## Multi-Tenancy

### Isolation Model

- **Data isolation**: all queries scoped by `user_id`, enforced at the db layer
- **Execution isolation**: each bot runs in its own container/sandbox
- **Resource isolation**: per-user limits on concurrent bots, runs/day, API calls/min
- **Workspace isolation**: per-user storage volumes, no shared filesystem access

### Sandboxing

Bots execute arbitrary Claude tool calls (bash, file I/O). Sandboxing options:

1. **Container per bot** — Docker/Firecracker, strongest isolation, highest overhead
2. **Container per user** — shared container for all of a user's bots, medium isolation
3. **Namespace/cgroup** — Linux namespaces, lighter weight but Linux-only
4. **Cloud functions** — serverless execution per run (Lambda, Cloud Run)

Recommendation: Start with **container per bot run** using Firecracker or gVisor. Each run gets a fresh container with the bot's workspace mounted. Kill container on run completion or timeout.

## Monetization

### Pricing Model

Users bring their own Claude API key OR use platform-provided credits.

**BYOK (Bring Your Own Key):**
- Free tier: 3 bots, 100 runs/month
- Pro ($19/mo): 20 bots, unlimited runs, priority execution
- Team ($49/mo/seat): shared bots, team workspace, audit logs

**Platform Credits (markup on Claude API):**
- Pay-as-you-go: $X per 1K input tokens, $Y per 1K output tokens (margin on top of Anthropic pricing)
- Bundled with Pro/Team plans as monthly credit allowance

### Usage Metering

Track per-run:
- Input/output tokens (from Claude API response)
- Run duration
- Tool calls count
- Storage used

## Client Connectivity

### Mobile Apps
REST API + push notifications (APNs/FCM) for bot events. Lightweight client that shows bot status, logs, and allows messaging.

### Web Dashboard
React/Next.js app consuming the same API. Replaces the local TUI for hosted users.

### CLI Integration
`botctl` could add `botctl login` and `botctl remote` commands to manage platform bots from the terminal, using the same API.

### Webhooks
User-configured HTTP callbacks for events:
- Bot started/stopped
- Run completed (with stats)
- Bot error/crash
- Message received

## Implementation Phases

### Phase 1 — API Server
- REST API wrapping `pkg/service`
- API key auth
- Single-user (no multi-tenancy yet)
- Postgres database
- Deploy on single VPS

### Phase 2 — Multi-Tenancy
- User registration + API key management
- Per-user data isolation
- Rate limiting
- Basic web dashboard

### Phase 3 — Sandboxed Execution
- Container-based bot execution
- Workspace isolation
- Resource limits + timeouts

### Phase 4 — Billing
- Usage metering
- Stripe integration
- Plan management
- BYOK support

### Phase 5 — Clients
- Mobile app (iOS/Android)
- Webhooks
- CLI `botctl remote` commands
- WebSocket real-time streaming

## Open Questions

- **Domain**: api.botctl.com? platform.botctl.com?
- **BYOK security**: how to securely store user API keys (envelope encryption? vault?)
- **Bot marketplace**: should users be able to publish/share BOT.md templates?
- **Collaboration**: shared bots within a team, role-based access?
- **Execution region**: single region to start, multi-region later?
- **SDK**: publish client SDKs (JS, Python, Go) for the API?
