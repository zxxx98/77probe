# Personal Server Probe Implementation Roadmap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this roadmap plan-by-plan. Detailed steps live in the three linked plans.

**Goal:** Deliver a self-hosted, single-user Linux server monitor for 1–10 nodes with real-time metrics, 30-day history, and Webhook alerts.

**Architecture:** A Go monolith serves the API, embedded React application, SQLite database, SSE stream, aggregation jobs, and alert jobs. A separate Go Agent collects read-only Linux metrics and pushes reports over an authenticated HTTP(S) endpoint. Docker Compose deploys one application container and one persistent volume.

**Tech Stack:** Go 1.24.x, Chi v5, modernc SQLite, gopsutil v4, React 19, TypeScript 5.8+, Vite 7, Vitest, Testing Library, Apache ECharts 6, Docker Compose v2.

## Global Constraints

- Product is single-user and self-hosted for 1–10 Linux servers.
- Agent targets Linux `amd64` and `arm64`, stays read-only, and exposes no inbound port or command channel.
- Agent reports every 5 seconds; a server becomes offline after 30 seconds without a report.
- Browser live updates use SSE; Agent ingestion uses authenticated HTTP(S) JSON.
- SQLite runs in WAL mode; no Redis, queue, or separate worker service is allowed.
- Raw 5-second history is not persisted; minute aggregates are retained for exactly 30 days.
- Notifications use generic Webhook only; email and built-in provider adapters are excluded.
- Login rate limiting, HTTPS setup instructions, backup/export UI, remote commands, public status pages, and multi-user roles are excluded.
- UI uses the approved friendly light direction: pure-white background, restrained deep-rose brand accent, semantic green/orange/red states, 12–14px panel radius, short shadows, top navigation, and responsive server rows.
- Every implementation task follows test-driven development and ends with a focused Git commit.

---

## Plan Split

### Phase 1 — Real-time monitoring core

Plan: `docs/superpowers/plans/2026-07-26-probe-phase-1-realtime-core.md`

Produces a complete runnable product that supports first-run admin creation, login, server registration, independent Agent tokens, Linux Agent collection, authenticated 5-second ingestion, online/offline state, SSE updates, current dashboard, server details, responsive UI, and Docker Compose deployment.

Acceptance gate:

```text
Given a clean data volume
When the user creates the administrator, adds a server, and starts its Agent
Then the dashboard shows the server online with live CPU, memory, disk, and network values
And stopping the Agent marks it offline within 30–45 seconds
```

### Phase 2 — Historical trends

Plan: `docs/superpowers/plans/2026-07-26-probe-phase-2-history.md`

Adds minute aggregation, gauge average/max semantics, counter last-value semantics, 30-day rolling deletion, history API endpoints, real-time/1-day/7-day/30-day charts, data-gap handling, and database growth verification.

Acceptance gate:

```text
Given live Agent reports over more than one minute
When the user opens a server detail page
Then the selected time range shows aggregated resource history
And missing periods remain visible as gaps
And rows older than 30 days are removed automatically
```

### Phase 3 — Webhook alerting

Plan: `docs/superpowers/plans/2026-07-26-probe-phase-3-alerting.md`

Adds alert rules, `normal → pending → firing → recovered` state transitions, offline alerts, resource threshold duration, Webhook templates and headers, test sends, three-attempt retry, event history, recovery notifications, and alert management UI.

Acceptance gate:

```text
Given an enabled alert rule and Webhook
When a metric remains above its threshold for the configured duration
Then one firing event and one Webhook are created
And recovery creates one recovered event and one recovery Webhook
And a failed Webhook never blocks metric ingestion
```

## Locked File Structure

```text
cmd/
  agent/main.go                 Agent executable wiring
  server/main.go                Server executable wiring
  loadgen/main.go               Local 10-Agent verification tool
internal/
  agent/                        Linux collection, report client, retry runner
  alerting/                     Rule state machine, scheduler, Webhook sender
  app/                          Server composition and lifecycle
  auth/                         Password hashing, sessions, middleware, handlers
  db/                           SQLite open, migrations, transactions
  history/                      Minute aggregation, retention, history queries
  httpapi/                      Router, JSON helpers, health endpoints
  live/                         Latest snapshots, offline sweeper, SSE hub
  protocol/                     Agent/API JSON contracts shared by binaries
  servers/                      Server registry and Agent token lifecycle
  webui/                        Embedded frontend distribution
migrations/                     Ordered SQLite schema files
web/
  src/api/                      Typed browser API client
  src/auth/                     Setup and login flows
  src/components/               Reusable product UI components
  src/live/                     SSE subscription and snapshot state
  src/pages/                    Overview, server detail, alerts, settings
  src/styles/                   Approved tokens, base styles, responsive rules
  src/test/                     Frontend test setup and fixtures
deploy/
  Dockerfile                    Frontend, server, and Agent multi-stage build
docker-compose.yml              One app service and one named data volume
```

Files remain grouped by responsibility. Cross-package contracts must be defined in `internal/protocol` or an explicit package interface; packages must not import another package's internal implementation details.

## Execution Prerequisites

The current workspace already has Node.js `22.14.0` and pnpm `10.8.1`. Go and Docker are not currently available on `PATH`. Before Phase 1 execution, install Go `1.24.x` and Docker with Compose v2, then verify:

```bash
go version
docker version
docker compose version
node --version
pnpm --version
```

Do not install these runtimes as an unreviewed side effect of executing a code task; treat runtime installation as an explicit workstation prerequisite.

## Execution Order

1. Execute and review Phase 1 in full.
2. Run the Phase 1 acceptance gate before starting Phase 2.
3. Execute and review Phase 2 in full.
4. Run the Phase 2 acceptance gate before starting Phase 3.
5. Execute and review Phase 3 in full.
6. Run `go test ./...`, `pnpm --dir web test -- --run`, `pnpm --dir web build`, and the Docker Compose smoke test before declaring the product complete.

## Specification Coverage Matrix

| Specification requirement | Implemented by |
|---|---|
| First-run administrator, login, session security | Phase 1 Tasks 2–3 and 7 |
| Server create/rename/disable/delete and one-time Token | Phase 1 Tasks 4 and 9 |
| Linux `amd64`/`arm64` read-only Agent | Phase 1 Tasks 5 and 10 |
| CPU, load, memory, Swap, disks, disk I/O, network, uptime | Phase 1 Task 5 |
| Authenticated 5-second ingestion and source IP | Phase 1 Task 6 |
| Online/offline and 30-second cutoff | Phase 1 Task 6 |
| Friendly responsive overview and current detail page | Phase 1 Tasks 7–9 |
| Docker Compose single-container deployment | Phase 1 Task 10 |
| Minute average/max and counter-last aggregation | Phase 2 Tasks 1–3 |
| Exactly 30-day automatic retention, no maintenance UI | Phase 2 Tasks 3 and 6 |
| 1-day/7-day/30-day charts and explicit gaps | Phase 2 Tasks 4–5 |
| Offline, CPU, memory, disk usage, disk free-space rules | Phase 3 Tasks 1–3 |
| `normal/pending/firing/recovered` transitions | Phase 3 Task 2 |
| Generic Webhook, headers, JSON template, test send | Phase 3 Tasks 4–6 |
| Three attempts, recovery notifications, delivery history | Phase 3 Tasks 4–7 |
| Excluded remote control, multi-user, email, provider adapters | Enforced by all Global Constraints and absence of related routes/packages |
