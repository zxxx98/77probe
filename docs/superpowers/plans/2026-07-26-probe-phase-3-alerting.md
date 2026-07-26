# Probe Phase 3 Webhook Alerting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add durable alert rules and events, threshold-duration evaluation, offline and recovery alerts, and generic Webhook delivery with templates and three-attempt retry.

**Architecture:** An in-process alert evaluator consumes live snapshot changes and offline transitions. A pure state machine decides `normal`, `pending`, `firing`, and `recovered`; a repository persists state/events. A bounded Webhook dispatcher renders JSON templates, applies configured headers, retries without blocking ingestion, and records every attempt.

**Tech Stack:** Go 1.24.x, SQLite WAL, Go `text/template`, React 19, TypeScript, Vitest, Testing Library.

## Global Constraints

- Phase 1 and Phase 2 must be complete and passing before this plan begins.
- Support only offline, CPU usage, memory usage, disk usage, and disk free-space rules.
- Resource rules default to a 5-minute duration; offline state is determined by the existing 30-second live cutoff.
- Send one firing notification and one recovery notification per alert episode.
- Repeated reminders are disabled by default; an optional repeat interval may be configured.
- Notification transport is generic Webhook only.
- Webhook supports URL, custom headers, JSON body template, test send, enable/disable, and three attempts with increasing delay.
- Webhook failures must never block or fail Agent ingestion.
- Do not add email, Telegram, DingTalk, Feishu, WeCom, HMAC signing, multi-user routing, or remote remediation.

## Test Fixture Contracts

```go
func newRepository(t *testing.T) *alerting.Repository
func newEvaluator(t *testing.T) (*alerting.Evaluator, *alerting.Repository, *recordingDeliveryQueue)
func createOfflineRule(t *testing.T, repo *alerting.Repository, serverID int64) alerting.Rule
func snapshot(serverID int64, online bool) live.Snapshot
func ptr(value time.Time) *time.Time
func assertLatestEventStatus(t *testing.T, repo *alerting.Repository, status alerting.Status)
func assertDeliveryCount(t *testing.T, queue *recordingDeliveryQueue, count int)
func alwaysFailingServer(t *testing.T) *httptest.Server
func newTestDispatcher(t *testing.T, url string) (*alerting.Dispatcher, *recordingAttemptStore)
func deliveryJob() alerting.DeliveryJob
func authenticatedAlertHandler(t *testing.T) http.Handler
func performJSON(handler http.Handler, method, path, body string) *httptest.ResponseRecorder
```

Frontend tests define complete `serversFixture`, `maskedWebhookFixture`, and API spies in their test modules. Dispatcher tests inject a fake sleeper so the `5s` and `15s` delays do not slow the suite.

---

### Task 1: Add alerting schema and repository contracts

**Files:**
- Create: `migrations/003_alerting.sql`
- Create: `internal/alerting/model.go`
- Create: `internal/alerting/repository.go`
- Test: `internal/alerting/repository_test.go`

**Interfaces:**
- Produces tables: `webhook_configs`, `alert_rules`, `alert_states`, `alert_events`, `webhook_attempts`
- Produces: `alerting.Repository`

- [ ] **Step 1: Write repository round-trip tests**

```go
func TestRepositoryCreatesRuleAndEvent(t *testing.T) {
    repo := newRepository(t)
    ctx := context.Background()
    rule, err := repo.CreateRule(ctx, alerting.Rule{
        ServerID: 1, Metric: alerting.MetricCPUUsage, Operator: alerting.OperatorGreaterThan,
        Threshold: 85, DurationSeconds: 300, Enabled: true,
    })
    if err != nil { t.Fatal(err) }
    event, err := repo.CreateEvent(ctx, alerting.Event{
        RuleID: rule.ID, ServerID: 1, Status: alerting.StatusFiring,
        CurrentValue: 91, Threshold: 85, StartedAt: time.Unix(100, 0).UTC(),
    })
    if err != nil { t.Fatal(err) }
    if event.ID == 0 || event.RuleID != rule.ID { t.Fatalf("event=%+v", event) }
}
```

- [ ] **Step 2: Run the repository test and observe failure**

Run: `go test ./internal/alerting -run TestRepositoryCreatesRuleAndEvent -v`

Expected: FAIL because schema and repository do not exist.

- [ ] **Step 3: Add the alerting schema**

```sql
-- migrations/003_alerting.sql
CREATE TABLE webhook_configs (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    url TEXT NOT NULL,
    headers_json TEXT NOT NULL DEFAULT '{}',
    body_template TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    duration_seconds INTEGER NOT NULL,
    repeat_seconds INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE alert_states (
    rule_id INTEGER PRIMARY KEY REFERENCES alert_rules(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    pending_since TEXT,
    firing_since TEXT,
    last_notified_at TEXT,
    last_value REAL,
    updated_at TEXT NOT NULL
);

CREATE TABLE alert_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id INTEGER NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    current_value REAL,
    threshold REAL,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE webhook_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER REFERENCES alert_events(id) ON DELETE CASCADE,
    is_test INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL,
    response_status INTEGER,
    error_text TEXT NOT NULL DEFAULT '',
    sent_at TEXT NOT NULL
);
```

- [ ] **Step 4: Define strict enums and repository methods**

```go
type Metric string
const (
    MetricOffline Metric = "offline"
    MetricCPUUsage Metric = "cpu_usage"
    MetricMemoryUsage Metric = "memory_usage"
    MetricDiskUsage Metric = "disk_usage"
    MetricDiskFreeBytes Metric = "disk_free_bytes"
)

type Status string
const (
    StatusNormal Status = "normal"
    StatusPending Status = "pending"
    StatusFiring Status = "firing"
    StatusRecovered Status = "recovered"
)
```

Repository methods must create/update/list/delete rules, get/update state transactionally, create/list events, upsert/get Webhook config, and record/list attempts. Validate enum values before SQL and return typed `ErrNotFound` for absent rows.

- [ ] **Step 5: Verify repository behavior**

Run: `go test ./internal/alerting -run TestRepository -v`

Expected: PASS for rule CRUD, one state per rule, event ordering newest first, singleton Webhook config, and cascading delete.

- [ ] **Step 6: Commit alerting persistence**

```bash
git add migrations/003_alerting.sql internal/alerting
git commit -m "feat: add alerting persistence"
```

### Task 2: Implement the pure alert state machine

**Files:**
- Create: `internal/alerting/state_machine.go`
- Test: `internal/alerting/state_machine_test.go`

**Interfaces:**
- Produces: `alerting.Evaluate(input EvaluationInput) EvaluationResult`
- No database, HTTP, logging, or wall-clock calls inside the pure evaluator

- [ ] **Step 1: Write table-driven transition tests**

```go
func TestEvaluateTransitions(t *testing.T) {
    now := time.Unix(1000, 0).UTC()
    cases := []struct{
        name string
        state State
        breached bool
        duration time.Duration
        want Status
        notify bool
    }{
        {"normal remains normal", State{Status: StatusNormal}, false, 5*time.Minute, StatusNormal, false},
        {"normal becomes pending", State{Status: StatusNormal}, true, 5*time.Minute, StatusPending, false},
        {"pending becomes firing", State{Status: StatusPending, PendingSince: ptr(now.Add(-5*time.Minute))}, true, 5*time.Minute, StatusFiring, true},
        {"firing remains firing", State{Status: StatusFiring, FiringSince: ptr(now.Add(-10*time.Minute))}, true, 5*time.Minute, StatusFiring, false},
        {"firing recovers", State{Status: StatusFiring, FiringSince: ptr(now.Add(-10*time.Minute))}, false, 5*time.Minute, StatusRecovered, true},
        {"recovered settles normal", State{Status: StatusRecovered}, false, 5*time.Minute, StatusNormal, false},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            got := Evaluate(EvaluationInput{State: tc.state, Breached: tc.breached, Duration: tc.duration, Now: now})
            if got.State.Status != tc.want || got.Notify != tc.notify { t.Fatalf("got=%+v", got) }
        })
    }
}
```

- [ ] **Step 2: Run transition tests and observe failure**

Run: `go test ./internal/alerting -run TestEvaluateTransitions -v`

Expected: FAIL because `Evaluate` is absent.

- [ ] **Step 3: Implement deterministic transitions and reminders**

```go
type EvaluationInput struct {
    State          State
    Breached       bool
    CurrentValue   float64
    Duration       time.Duration
    RepeatInterval time.Duration
    Now            time.Time
}

type EvaluationResult struct {
    State  State
    Notify bool
}
```

Rules:

- `normal + breached` sets `pending_since=now` unless duration is zero, in which case it enters `firing` and notifies.
- `pending + breached` enters `firing` when `now-pending_since >= duration` and notifies once.
- `pending + healthy` returns to `normal` without notification.
- `firing + breached` notifies only when repeat interval is non-zero and elapsed since `last_notified_at` meets it.
- `firing + healthy` enters `recovered` and notifies once.
- `recovered + healthy` enters `normal` without notification on the next evaluation.
- `recovered + breached` starts a new `pending` episode.

- [ ] **Step 4: Verify edge cases**

Run: `go test ./internal/alerting -run 'TestEvaluateTransitions|TestEvaluateReminder|TestEvaluatePendingReset' -v`

Expected: PASS for duration zero, exact boundary, disabled reminder, enabled reminder, pending reset, and new episode after recovery.

- [ ] **Step 5: Commit the state machine**

```bash
git add internal/alerting/state_machine.go internal/alerting/state_machine_test.go
git commit -m "feat: add alert state machine"
```

### Task 3: Evaluate live and offline snapshots without blocking ingestion

**Files:**
- Create: `internal/alerting/evaluator.go`
- Create: `internal/alerting/metrics.go`
- Test: `internal/alerting/evaluator_test.go`
- Modify: `internal/live/ingest.go`
- Modify: `internal/live/sweeper.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Consumes: `live.Event`
- Produces: `alerting.Evaluator.Submit(event live.Event)`
- Produces: durable rule state and event rows
- Produces: `DeliveryJob` only when notification is required

- [ ] **Step 1: Write evaluation tests for CPU, disk free space, and offline recovery**

```go
func TestOfflineEventFiresAndRecovers(t *testing.T) {
    evaluator, repo, deliveries := newEvaluator(t)
    serverID := int64(9)
    createOfflineRule(t, repo, serverID)

    evaluator.EvaluateNow(context.Background(), live.Event{Type: "snapshot.offline", Snapshot: snapshot(serverID, false)})
    assertLatestEventStatus(t, repo, StatusFiring)
    assertDeliveryCount(t, deliveries, 1)

    evaluator.EvaluateNow(context.Background(), live.Event{Type: "snapshot.updated", Snapshot: snapshot(serverID, true)})
    assertLatestEventStatus(t, repo, StatusRecovered)
    assertDeliveryCount(t, deliveries, 2)
}
```

- [ ] **Step 2: Run evaluator tests and observe failure**

Run: `go test ./internal/alerting -run TestOfflineEventFiresAndRecovers -v`

Expected: FAIL because the evaluator is absent.

- [ ] **Step 3: Implement metric extraction and comparison**

For memory and Swap usage, compute `used/total*100`, treating total zero as zero. For `disk_usage`, evaluate every persistent disk and use the highest usage percentage. For `disk_free_bytes`, use the lowest free bytes across disks. Offline value is `1` when offline and `0` when online.

Support operators `gt` and `lt`. Enforce `lt` for disk free bytes and `gt` for percentage metrics and offline.

- [ ] **Step 4: Implement serialized asynchronous evaluation**

`Submit` writes to a bounded channel with capacity 128. One evaluator goroutine processes events in order, loads enabled rules for the server, computes breach/value, calls the pure state machine, and updates state plus event in one transaction. If the queue is full, log the drop and schedule a server-wide reevaluation from the latest snapshot; never block Agent ingestion.

Offline rules use duration zero because the live store has already applied the 30-second cutoff. Resource rules use their configured duration.

- [ ] **Step 5: Verify ingestion isolation and durable transitions**

Run: `go test ./internal/alerting ./internal/live ./internal/app -v`

Expected: PASS for CPU duration, memory zero-total, highest disk usage, lowest disk free bytes, offline firing, recovery, disabled rule, queue saturation fallback, and `204` ingestion during slow evaluation.

- [ ] **Step 6: Commit live alert evaluation**

```bash
git add internal/alerting internal/live internal/app
git commit -m "feat: evaluate live alert rules"
```

### Task 4: Render and deliver generic Webhooks with three attempts

**Files:**
- Create: `internal/alerting/template.go`
- Create: `internal/alerting/webhook.go`
- Create: `internal/alerting/dispatcher.go`
- Test: `internal/alerting/template_test.go`
- Test: `internal/alerting/webhook_test.go`
- Test: `internal/alerting/dispatcher_test.go`

**Interfaces:**
- Produces: `alerting.RenderTemplate(templateText string, data TemplateData) ([]byte, error)`
- Produces: `alerting.WebhookClient.Send(ctx, config, body) AttemptResult`
- Produces: `alerting.Dispatcher.Enqueue(DeliveryJob) error`

- [ ] **Step 1: Write template and retry tests**

```go
func TestRenderTemplateProducesValidJSON(t *testing.T) {
    body, err := RenderTemplate(`{"server":{{json .ServerName}},"status":"{{.Status}}"}`, TemplateData{ServerName: `home"lab`, Status: StatusFiring})
    if err != nil { t.Fatal(err) }
    var got map[string]string
    if err := json.Unmarshal(body, &got); err != nil { t.Fatal(err) }
    if got["server"] != `home"lab` { t.Fatalf("server=%q", got["server"]) }
}
```

```go
func TestDispatcherRetriesThreeTimes(t *testing.T) {
    endpoint := alwaysFailingServer(t)
    dispatcher, attempts := newTestDispatcher(t, endpoint.URL)
    dispatcher.DispatchNow(context.Background(), deliveryJob())
    if attempts.Count() != 3 { t.Fatalf("attempts=%d", attempts.Count()) }
}
```

- [ ] **Step 2: Run Webhook tests and observe failure**

Run: `go test ./internal/alerting -run 'TestRenderTemplate|TestDispatcherRetriesThreeTimes' -v`

Expected: FAIL because rendering and dispatch are absent.

- [ ] **Step 3: Implement safe JSON template rendering**

Use `text/template` with `missingkey=error` and a `json` helper that applies `json.Marshal` to a value and returns the encoded JSON token. After rendering, call `json.Valid`; reject invalid output before sending.

Template data fields:

```go
type TemplateData struct {
    EventID      int64
    ServerID     int64
    ServerName   string
    Metric       Metric
    Status       Status
    CurrentValue float64
    Threshold    float64
    StartedAt    time.Time
    EndedAt      *time.Time
    DetailURL    string
}
```

- [ ] **Step 4: Implement HTTP delivery and retries**

Require an absolute `http` or `https` URL. Send `POST` with `Content-Type: application/json`, configured headers, and a 10-second request timeout. Treat only status `200–299` as success. Limit response/error text stored in SQLite to 2,048 bytes.

Attempt delays are `0s`, `5s`, and `15s`. Persist each attempt before deciding whether to retry. Dispatcher uses two workers and a bounded queue of 64; enqueue failure is logged and the event remains visible as undelivered.

- [ ] **Step 5: Verify Webhook behavior**

Run: `go test ./internal/alerting -run 'TestRenderTemplate|TestWebhook|TestDispatcher' -v`

Expected: PASS for JSON escaping, missing variables, invalid JSON, custom headers, 2xx success, non-2xx failure, timeout, three attempts, test-send attempt, and queue-full behavior.

- [ ] **Step 6: Commit Webhook delivery**

```bash
git add internal/alerting
git commit -m "feat: add generic webhook delivery"
```

### Task 5: Add alert, event, and Webhook management APIs

**Files:**
- Create: `internal/alerting/handler.go`
- Test: `internal/alerting/handler_test.go`
- Modify: `internal/httpapi/router.go`

**Interfaces:**
- HTTP: `GET/POST /api/alert-rules`
- HTTP: `PATCH/DELETE /api/alert-rules/{id}`
- HTTP: `GET /api/alert-events`
- HTTP: `GET/PUT /api/webhook`
- HTTP: `POST /api/webhook/test`

- [ ] **Step 1: Write API validation tests**

```go
func TestCreateCPURuleDefaultsDurationToFiveMinutes(t *testing.T) {
    handler := authenticatedAlertHandler(t)
    rec := performJSON(handler, http.MethodPost, "/api/alert-rules", `{"serverId":1,"metric":"cpu_usage","operator":"gt","threshold":85}`)
    if rec.Code != http.StatusCreated { t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()) }
    var got alerting.Rule
    decodeJSON(t, rec.Body, &got)
    if got.DurationSeconds != 300 { t.Fatalf("duration=%d", got.DurationSeconds) }
}
```

- [ ] **Step 2: Run API tests and observe failure**

Run: `go test ./internal/alerting -run TestCreateCPURuleDefaultsDurationToFiveMinutes -v`

Expected: FAIL because handlers are absent.

- [ ] **Step 3: Implement strict rule validation**

Validation matrix:

| Metric | Operator | Threshold | Duration |
|---|---|---|---|
| `offline` | `gt` | exactly `0` | forced to `0` |
| `cpu_usage` | `gt` | `0–100` | default `300`, range `0–86400` |
| `memory_usage` | `gt` | `0–100` | default `300`, range `0–86400` |
| `disk_usage` | `gt` | `0–100` | default `300`, range `0–86400` |
| `disk_free_bytes` | `lt` | greater than `0` | default `300`, range `0–86400` |

`repeatSeconds` defaults to `0`; when non-zero it must be at least `300` and at most `604800`.

- [ ] **Step 4: Implement Webhook configuration and test send**

Default body template:

```json
{
  "server": {{json .ServerName}},
  "metric": "{{.Metric}}",
  "status": "{{.Status}}",
  "currentValue": {{.CurrentValue}},
  "threshold": {{.Threshold}},
  "startedAt": "{{.StartedAt.Format \"2006-01-02T15:04:05Z07:00\"}}",
  "detailUrl": {{json .DetailURL}}
}
```

The API stores header values but returns values masked as `••••••` for header names containing `authorization`, `token`, `secret`, or `key`. A PUT request using the masked value preserves the existing secret.

Test send uses sample server name `Webhook 测试`, status `firing`, current value `85`, threshold `80`, and records attempts with `is_test=1` and no event ID.

- [ ] **Step 5: Verify management APIs**

Run: `go test ./internal/alerting ./internal/httpapi -v`

Expected: PASS for authentication, validation matrix, default duration, event pagination, masked headers, invalid template, test success, and test failure details.

- [ ] **Step 6: Commit alerting APIs**

```bash
git add internal/alerting/handler.go internal/alerting/handler_test.go internal/httpapi
git commit -m "feat: add alert and webhook api"
```

### Task 6: Build alert management and event UI

**Files:**
- Create: `web/src/alerts/types.ts`
- Create: `web/src/alerts/api.ts`
- Create: `web/src/components/AlertRuleForm.tsx`
- Create: `web/src/components/WebhookForm.tsx`
- Create: `web/src/components/AlertEventList.tsx`
- Create: `web/src/pages/AlertsPage.tsx`
- Create: `web/src/pages/SettingsPage.tsx`
- Test: `web/src/components/AlertRuleForm.test.tsx`
- Test: `web/src/components/WebhookForm.test.tsx`
- Test: `web/src/pages/AlertsPage.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/components/AppNav.tsx`
- Modify: `web/src/styles/dashboard.css`

**Interfaces:**
- Consumes: alerting APIs from Task 5
- Produces: rule CRUD, event list, Webhook setup, test send, and failure details

- [ ] **Step 1: Write rule-form and Webhook masking tests**

```tsx
it("forces offline rules to zero duration", async () => {
  render(<AlertRuleForm servers={serversFixture} onSaved={vi.fn()} />);
  await userEvent.selectOptions(screen.getByLabelText("指标"), "offline");
  expect(screen.getByLabelText("持续时间（秒）")).toHaveValue(0);
  expect(screen.getByLabelText("持续时间（秒）")).toBeDisabled();
});
```

```tsx
it("preserves a masked authorization header", async () => {
  render(<WebhookForm initial={maskedWebhookFixture} />);
  expect(screen.getByLabelText("Authorization")).toHaveValue("••••••");
  await userEvent.click(screen.getByRole("button", { name: "保存 Webhook" }));
  expect(updateWebhook).toHaveBeenCalledWith(expect.objectContaining({ headers: { Authorization: "••••••" } }));
});
```

- [ ] **Step 2: Run UI tests and observe failure**

Run: `pnpm --dir web test -- --run AlertRuleForm.test.tsx WebhookForm.test.tsx AlertsPage.test.tsx`

Expected: FAIL because alert UI is absent.

- [ ] **Step 3: Implement the alert rule experience**

Use an inline form rather than a modal. Fields: server, metric, operator displayed as fixed language, threshold with metric-specific unit, duration, optional repeat interval, enabled state. Show validation beside the field and keep submitted values after an API error.

Rule list displays server name, human-readable condition, current state, enabled switch, edit, and delete. Require an inline delete confirmation row; do not use a browser confirm dialog.

- [ ] **Step 4: Implement Webhook settings and test feedback**

Fields: URL, enabled state, editable header rows, JSON template textarea. Provide a variable reference panel. `发送测试` shows sending, success status code, or the final failure summary and attempt count. Secret-looking headers remain masked until explicitly replaced.

- [ ] **Step 5: Implement event history**

Show newest events first with semantic status, server, metric, current value, threshold, start/end times, and Webhook delivery result. Expand a row inline to show three attempt records; do not use a modal.

- [ ] **Step 6: Verify alert UI and responsive behavior**

Run: `pnpm --dir web test -- --run && pnpm --dir web build`

Expected: PASS. At 390px, form labels remain visible, tables become stacked event rows, and no secret header value is exposed after reload.

- [ ] **Step 7: Commit alerting UI**

```bash
git add web/src
git commit -m "feat: add alert and webhook management ui"
```

### Task 7: Run end-to-end alert acceptance and fault isolation

**Files:**
- Create: `internal/alerting/integration_test.go`
- Modify: `cmd/loadgen/main.go`
- Modify: `README.md`

**Interfaces:**
- Produces: scripted high-CPU, low-disk, offline, recovery, and failing-Webhook scenarios
- Documents: Webhook variables, retry behavior, and no-email boundary

- [ ] **Step 1: Write the end-to-end alert integration test**

The test must:

1. Create one server, CPU rule with 2-second duration, and test Webhook server.
2. Submit a healthy report.
3. Submit breached reports across the duration boundary.
4. Assert one firing event and one successful Webhook attempt.
5. Submit another breached report and assert no duplicate notification.
6. Submit a healthy report and assert one recovered event and recovery Webhook.

- [ ] **Step 2: Run the integration test**

Run: `go test ./internal/alerting -run TestAlertEpisodeEndToEnd -v`

Expected: PASS. Fix any cross-package mismatch before continuing.

- [ ] **Step 3: Add load-generator alert scenarios**

Add flags:

```text
-cpu-percent=95
-disk-used-percent=92
-stop-after=2m
```

Values must remain bounded and deterministic. `-stop-after` stops reporting but leaves the process alive long enough to observe an offline alert, then resumes when `-resume-after` is supplied.

- [ ] **Step 4: Verify failing Webhook isolation**

Configure a Webhook endpoint returning `500`, run 10 simulated Agents, and confirm:

- Agent report responses remain `204`.
- Live dashboard continues updating.
- Each alert records exactly three attempts.
- The failure appears in event history.
- Recovery produces a separate three-attempt sequence.

- [ ] **Step 5: Run the full product verification suite**

Run:

```bash
go test ./...
pnpm --dir web test -- --run
pnpm --dir web build
docker compose up -d --build
curl --fail http://localhost:8080/api/health
```

Expected: all commands PASS and the container is healthy.

- [ ] **Step 6: Commit Phase 3 delivery**

```bash
git add internal/alerting cmd/loadgen README.md
git commit -m "test: verify webhook alert delivery"
```
