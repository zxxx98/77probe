# Probe Phase 2 Historical Trends Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist minute-level metric aggregates, retain exactly 30 days, and expose real-time/1-day/7-day/30-day charts with visible data gaps.

**Architecture:** The live ingestion path forwards accepted reports to an in-process minute aggregator without blocking the response. Gauge/rate fields track count, sum, and max; cumulative counters retain the latest sample. Completed UTC minutes are written transactionally to SQLite, queried through bounded history endpoints, and rendered with ECharts.

**Tech Stack:** Go 1.24.x, SQLite WAL, React 19, TypeScript, Apache ECharts 6, Vitest, Testing Library.

## Global Constraints

- Phase 1 must be complete and passing before this plan begins.
- Do not persist raw 5-second reports.
- Aggregate on UTC minute boundaries.
- Store average and maximum for CPU, load, memory usage, Swap usage, per-mount disk usage, aggregate disk I/O rates, and network rates.
- Store the last value for cumulative upload/download counters and capacity totals.
- Preserve missing periods as gaps; never forward-fill chart data.
- Retain exactly the latest 30 days through an internal scheduled deletion job.
- Do not add backup, export, manual cleanup, or retention-setting UI.
- Keep all history queries scoped to one authenticated administrator and one server.

## Test Fixture Contracts

```go
func migratedDatabase(t *testing.T) *sql.DB
func newHistoryStore(t *testing.T) *history.Store
func insertMinute(t *testing.T, store *history.Store, serverID, minuteUnix int64)
func reportWith(cpuUsage float64, uploadBPS, totalUpload uint64) protocol.AgentReport
func newHistoryHandler(t *testing.T) (http.Handler, *recordingHistoryStore)
func authenticatedRequest(method, path string) *http.Request
var fixedNow time.Time
```

Frontend history tests define `point(minuteUnix: number, cpuAverage: number): MinuteRecord` using the exact `MinuteRecord` API type. Every database helper uses a fresh temporary SQLite file and applies all migrations.

---

### Task 1: Add the minute-history schema

**Files:**
- Create: `migrations/002_metric_minutes.sql`
- Create: `internal/history/model.go`
- Test: `internal/history/schema_test.go`

**Interfaces:**
- Produces table: `metric_minutes`
- Produces type: `history.MinuteRecord`

- [ ] **Step 1: Write the migration schema test**

```go
func TestMetricMinuteUniqueness(t *testing.T) {
    conn := migratedDatabase(t)
    _, err := conn.Exec(`INSERT INTO metric_minutes(server_id, minute_unix, payload_json) VALUES (1, 100, '{}')`)
    if err != nil { t.Fatal(err) }
    _, err = conn.Exec(`INSERT INTO metric_minutes(server_id, minute_unix, payload_json) VALUES (1, 100, '{}')`)
    if err == nil { t.Fatal("expected unique server/minute constraint") }
}
```

- [ ] **Step 2: Run the schema test and observe failure**

Run: `go test ./internal/history -run TestMetricMinuteUniqueness -v`

Expected: FAIL because the table and package do not exist.

- [ ] **Step 3: Add the schema and record contract**

```sql
-- migrations/002_metric_minutes.sql
CREATE TABLE metric_minutes (
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    minute_unix INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (server_id, minute_unix)
);

CREATE INDEX metric_minutes_minute_idx ON metric_minutes(minute_unix);
```

```go
package history

type Pair struct { Average float64 `json:"average"`; Maximum float64 `json:"maximum"` }

type DiskMinute struct {
    Mountpoint string `json:"mountpoint"`
    Usage      Pair   `json:"usage"`
    TotalBytes uint64 `json:"totalBytes"`
    UsedBytes  uint64 `json:"usedBytes"`
}

type MinutePayload struct {
    CPUUsage       Pair         `json:"cpuUsage"`
    Load1          Pair         `json:"load1"`
    Load5          Pair         `json:"load5"`
    Load15         Pair         `json:"load15"`
    MemoryUsage    Pair         `json:"memoryUsage"`
    SwapUsage      Pair         `json:"swapUsage"`
    Disks          []DiskMinute `json:"disks"`
    DiskReadBPS    Pair         `json:"diskReadBps"`
    DiskWriteBPS   Pair         `json:"diskWriteBps"`
    UploadBPS      Pair         `json:"uploadBps"`
    DownloadBPS    Pair         `json:"downloadBps"`
    TotalUpload    uint64       `json:"totalUpload"`
    TotalDownload  uint64       `json:"totalDownload"`
}

type MinuteRecord struct {
    ServerID  int64         `json:"serverId"`
    MinuteUnix int64        `json:"minuteUnix"`
    Payload   MinutePayload `json:"payload"`
}
```

- [ ] **Step 4: Verify the migration**

Run: `go test ./internal/history -run TestMetricMinuteUniqueness -v`

Expected: PASS.

- [ ] **Step 5: Commit the history schema**

```bash
git add migrations/002_metric_minutes.sql internal/history
git commit -m "feat: add minute history schema"
```

### Task 2: Implement deterministic minute aggregation

**Files:**
- Create: `internal/history/accumulator.go`
- Create: `internal/history/aggregator.go`
- Test: `internal/history/accumulator_test.go`
- Test: `internal/history/aggregator_test.go`

**Interfaces:**
- Produces: `history.Accumulator.Add(report protocol.AgentReport)`
- Produces: `history.Accumulator.Finish(serverID, minuteUnix int64) MinuteRecord`
- Produces: `history.Aggregator.Accept(serverID int64, report protocol.AgentReport, receivedAt time.Time)`
- Produces: `history.Aggregator.FlushBefore(ctx context.Context, minute time.Time) error`

- [ ] **Step 1: Write aggregation semantics tests**

```go
func TestAccumulatorUsesAverageMaxAndLastCounter(t *testing.T) {
    var acc history.Accumulator
    acc.Add(reportWith(10, 100, 1000))
    acc.Add(reportWith(30, 300, 1500))

    record := acc.Finish(4, 600)
    if record.Payload.CPUUsage.Average != 20 || record.Payload.CPUUsage.Maximum != 30 {
        t.Fatalf("cpu = %+v", record.Payload.CPUUsage)
    }
    if record.Payload.UploadBPS.Average != 200 || record.Payload.UploadBPS.Maximum != 300 {
        t.Fatalf("upload = %+v", record.Payload.UploadBPS)
    }
    if record.Payload.TotalUpload != 1500 {
        t.Fatalf("total upload = %d", record.Payload.TotalUpload)
    }
}
```

- [ ] **Step 2: Run the accumulator test and observe failure**

Run: `go test ./internal/history -run TestAccumulatorUsesAverageMaxAndLastCounter -v`

Expected: FAIL because aggregation is absent.

- [ ] **Step 3: Implement numeric accumulators and per-mount aggregation**

```go
type numeric struct { Count uint64; Sum float64; Max float64 }

func (n *numeric) add(value float64) {
    n.Count++
    n.Sum += value
    if n.Count == 1 || value > n.Max { n.Max = value }
}

func (n numeric) pair() Pair {
    if n.Count == 0 { return Pair{} }
    return Pair{Average: n.Sum / float64(n.Count), Maximum: n.Max}
}
```

`Accumulator` contains one `numeric` per gauge/rate, a map keyed by disk mountpoint, and last cumulative/capacity values. Calculate memory, Swap, and disk usage percentages as `used / total * 100`, with zero when total is zero. Sort disk output by mountpoint for deterministic JSON and tests.

- [ ] **Step 4: Implement the server/minute aggregator**

Use a mutex-protected map keyed by `{ServerID, MinuteUnix}`. `Accept` calculates `receivedAt.UTC().Truncate(time.Minute).Unix()`. `FlushBefore` extracts all buckets with minute less than the supplied boundary, writes them through a `Writer` interface, and removes a bucket only after a successful write.

```go
type Writer interface {
    UpsertMinute(context.Context, MinuteRecord) error
}
```

Do not start a goroutine inside `NewAggregator`; application lifecycle owns the ticker.

- [ ] **Step 5: Verify aggregation behavior**

Run: `go test ./internal/history -run 'TestAccumulator|TestAggregator' -v`

Expected: PASS for average, maximum, last counter, disk sorting, UTC boundary, failed-write retention, and successful removal.

- [ ] **Step 6: Commit minute aggregation**

```bash
git add internal/history
git commit -m "feat: aggregate live metrics by minute"
```

### Task 3: Persist aggregates and delete data older than 30 days

**Files:**
- Create: `internal/history/store.go`
- Create: `internal/history/jobs.go`
- Test: `internal/history/store_test.go`
- Test: `internal/history/jobs_test.go`
- Modify: `internal/live/ingest.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces: `history.Store.UpsertMinute(ctx, record) error`
- Produces: `history.Store.Query(ctx, serverID, fromUnix, toUnix int64) ([]MinuteRecord, error)`
- Produces: `history.Store.DeleteBefore(ctx, cutoffUnix int64) (int64, error)`
- Ingestion dependency: `interface { Accept(serverID int64, report protocol.AgentReport, receivedAt time.Time) }`

- [ ] **Step 1: Write persistence and retention tests**

```go
func TestDeleteBeforeKeepsCutoffMinute(t *testing.T) {
    store := newHistoryStore(t)
    insertMinute(t, store, 1, 99)
    insertMinute(t, store, 1, 100)

    deleted, err := store.DeleteBefore(context.Background(), 100)
    if err != nil { t.Fatal(err) }
    if deleted != 1 { t.Fatalf("deleted=%d", deleted) }
    records, err := store.Query(context.Background(), 1, 0, 200)
    if err != nil || len(records) != 1 || records[0].MinuteUnix != 100 {
        t.Fatalf("records=%+v err=%v", records, err)
    }
}
```

- [ ] **Step 2: Run store tests and observe failure**

Run: `go test ./internal/history -run 'TestDeleteBefore|TestUpsertMinute' -v`

Expected: FAIL because the store is absent.

- [ ] **Step 3: Implement JSON persistence with bounded queries**

Marshal `MinutePayload` to `payload_json`. Use:

```sql
INSERT INTO metric_minutes(server_id, minute_unix, payload_json, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(server_id, minute_unix) DO UPDATE SET payload_json=excluded.payload_json
```

Query with `WHERE server_id=? AND minute_unix>=? AND minute_unix<=? ORDER BY minute_unix ASC`. Reject ranges longer than 30 days and return at most 43,201 minute records.

- [ ] **Step 4: Wire background flush and retention jobs**

- Every 5 seconds call `Aggregator.FlushBefore(now.UTC().Truncate(time.Minute))`.
- Once at startup and then every 6 hours call `DeleteBefore(now.Add(-30*24*time.Hour).UTC().Truncate(time.Minute).Unix())`.
- On application shutdown, flush every completed minute but keep the current partial minute in memory; losing a partial minute during restart is acceptable.
- After a report passes ingestion validation and updates the live store, call `Aggregator.Accept` without waiting for SQLite.

- [ ] **Step 5: Verify jobs and ingestion isolation**

Run: `go test ./internal/history ./internal/live ./internal/app -v`

Expected: PASS, including a test proving an aggregator error does not change the ingestion `204` response.

- [ ] **Step 6: Commit history persistence**

```bash
git add internal/history internal/live internal/app
git commit -m "feat: persist and retain metric history"
```

### Task 4: Expose bounded history APIs

**Files:**
- Create: `internal/history/handler.go`
- Test: `internal/history/handler_test.go`
- Modify: `internal/httpapi/router.go`

**Interfaces:**
- HTTP: `GET /api/servers/{id}/history?range=1d|7d|30d`
- Produces: `{ "fromUnix": number, "toUnix": number, "points": MinuteRecord[] }`

- [ ] **Step 1: Write handler tests for valid ranges and ownership checks**

```go
func TestHistoryRangeSevenDays(t *testing.T) {
    handler, store := newHistoryHandler(t)
    req := authenticatedRequest(http.MethodGet, "/api/servers/7/history?range=7d")
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()) }
    if store.LastFrom != fixedNow.Add(-7*24*time.Hour).UTC().Truncate(time.Minute).Unix() {
        t.Fatalf("from=%d", store.LastFrom)
    }
}
```

- [ ] **Step 2: Run handler tests and observe failure**

Run: `go test ./internal/history -run TestHistoryRangeSevenDays -v`

Expected: FAIL because the handler is absent.

- [ ] **Step 3: Implement strict range parsing and response encoding**

Accept exactly `1d`, `7d`, and `30d`; return `400` for any other value. Confirm the server exists through `servers.Service.Get` before querying. Use the server clock, UTC minute boundaries, and authenticated administrator middleware.

Return an empty `points` array rather than `null` when no data exists.

- [ ] **Step 4: Verify history endpoints**

Run: `go test ./internal/history ./internal/httpapi -v`

Expected: PASS for 1d/7d/30d, invalid range, missing server, unauthenticated request, empty points, and ordered points.

- [ ] **Step 5: Commit history API**

```bash
git add internal/history internal/httpapi
git commit -m "feat: expose server history api"
```

### Task 5: Render history charts and explicit data gaps

**Files:**
- Modify: `web/package.json`
- Create: `web/src/history/types.ts`
- Create: `web/src/history/useHistory.ts`
- Create: `web/src/components/MetricChart.tsx`
- Create: `web/src/components/RangeTabs.tsx`
- Test: `web/src/history/useHistory.test.tsx`
- Test: `web/src/components/MetricChart.test.tsx`
- Modify: `web/src/pages/ServerDetailPage.tsx`
- Modify: `web/src/styles/dashboard.css`

**Interfaces:**
- Produces: `useHistory(serverID, range)`
- Produces: `MetricChart` receiving timestamped average/max series with `null` gaps
- Activates detail ranges: `实时`, `1天`, `7天`, `30天`

- [ ] **Step 1: Write gap-building and range-selection tests**

```ts
it("inserts null for a missing minute instead of forward filling", () => {
  const series = buildMinuteSeries([
    point(600, 10),
    point(720, 30)
  ], 600, 720, value => value.payload.cpuUsage.average);
  expect(series).toEqual([[600000, 10], [660000, null], [720000, 30]]);
});
```

- [ ] **Step 2: Add ECharts and observe failing tests**

Run: `pnpm --dir web add echarts@^6 && pnpm --dir web test -- --run useHistory.test.tsx MetricChart.test.tsx`

Expected: FAIL because hooks and chart components do not exist.

- [ ] **Step 3: Implement history fetching and stale-response protection**

`useHistory` aborts the previous request when server or range changes, returns `loading/error/data`, and never mixes points from two ranges. Cache successful results in memory by `serverID:range` for 60 seconds; no persistence is required.

- [ ] **Step 4: Implement accessible charts**

Use one ECharts instance per `MetricChart`. Draw average as the primary solid line and maximum as a lighter line. Set `connectNulls: false`. Provide an adjacent text summary containing current, average, and maximum values so the chart is not the only information source. Dispose the chart on unmount and resize it with `ResizeObserver`.

Charts required:

- CPU and load
- Memory and Swap
- One usage chart per persistent disk
- One aggregate disk I/O chart for read/write rates
- Network upload/download rates and cumulative totals

- [ ] **Step 5: Integrate live and historical ranges**

`实时` keeps the Phase 1 current-value view. Historical ranges fetch minute points and show skeleton blocks while loading, a useful empty state when no points exist, an inline retry on errors, and exact gaps for missing minutes.

- [ ] **Step 6: Verify history UI**

Run: `pnpm --dir web test -- --run && pnpm --dir web build`

Expected: PASS. Verify chart contrast, keyboard-operable range tabs, `prefers-reduced-motion`, and no horizontal overflow at 390px.

- [ ] **Step 7: Commit chart UI**

```bash
git add web/package.json pnpm-lock.yaml web/src
git commit -m "feat: add historical metric charts"
```

### Task 6: Verify 10-Agent database growth and Phase 2 acceptance

**Files:**
- Modify: `cmd/loadgen/main.go`
- Create: `internal/history/load_test.go`
- Modify: `README.md`

**Interfaces:**
- Produces: accelerated history mode for local verification only
- Documents: fixed 30-day retention and absence of maintenance UI

- [ ] **Step 1: Write a bounded-growth integration test**

Create 10 servers, feed one valid report per server for 120 simulated minutes through `Aggregator.Accept`, flush each minute, and assert exactly 1,200 rows. Advance the clock beyond 30 days, run retention, and assert only rows at or after the cutoff remain.

- [ ] **Step 2: Run the integration test and observe any gap**

Run: `go test ./internal/history -run TestTenAgentsMinuteRowGrowth -v`

Expected: PASS after Tasks 1–5; if it fails, fix aggregation or retention before proceeding.

- [ ] **Step 3: Add accelerated load generation**

Add `-interval` to `cmd/loadgen` with a minimum accepted value of `100ms` only when `-allow-fast=true` is also supplied. Production Agent cadence remains hard-coded at 5 seconds.

- [ ] **Step 4: Run the Phase 2 verification suite**

Run:

```bash
go test ./...
pnpm --dir web test -- --run
pnpm --dir web build
docker compose up -d --build
```

Manually confirm 1-day, 7-day, and 30-day charts; stop the Agent for at least two minutes and confirm the chart shows a gap after it resumes.

- [ ] **Step 5: Commit Phase 2 delivery**

```bash
git add cmd/loadgen internal/history README.md
git commit -m "test: verify bounded history retention"
```
