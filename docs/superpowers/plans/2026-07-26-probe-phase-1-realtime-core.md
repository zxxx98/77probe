# Probe Phase 1 Real-time Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a runnable single-user product that registers Linux Agents, ingests metrics every 5 seconds, shows live server status, and marks missing Agents offline.

**Architecture:** A Go server owns SQLite persistence, authentication, server registration, latest in-memory snapshots, offline detection, SSE, and embedded React assets. A Go Agent collects read-only Linux metrics through a testable source interface and posts a shared JSON contract using an independent Bearer Token.

**Tech Stack:** Go 1.24.x, Chi v5, modernc SQLite, gopsutil v4, React 19, TypeScript 5.8+, Vite 7, Vitest, Testing Library, Docker Compose v2.

## Global Constraints

- Support one administrator and 1–10 Linux servers only.
- Support Agent builds for Linux `amd64` and `arm64`.
- Keep Agent read-only, outbound-only, and free of remote command code.
- Report every 5 seconds and mark a server offline after 30 seconds without ingestion.
- Use SQLite WAL and one application process; do not add Redis, a queue, or a worker service.
- Use SSE for browser updates and authenticated JSON POST for Agent ingestion.
- Store only Agent Token SHA-256 digests and session Token SHA-256 digests.
- Do not add login rate limiting or HTTPS setup documentation.
- Use the approved friendly light UI tokens and responsive server-row layout.
- Follow TDD: failing test, observed failure, minimal implementation, passing test, focused commit.

## Test Fixture Contracts

Test snippets use focused helpers defined in the same `_test.go` file as the test that calls them:

```go
func newTestService(t *testing.T) *auth.Service
func newServerService(t *testing.T) *servers.Service
func snapshot(serverID int64, online bool) live.Snapshot
func newValidReport() protocol.AgentReport
func performJSON(handler http.Handler, method, path, body string) *httptest.ResponseRecorder
```

Each database helper opens `filepath.Join(t.TempDir(), "test.db")`, applies all migrations, and registers `t.Cleanup` to close the connection. Frontend tests define `mockStatusResponse`, `fakeEventSource`, and `serverFixture` in their own test modules; fixtures must contain the complete API shape from `web/src/api/types.ts`.

---

### Task 1: Bootstrap the Go server and health endpoint

**Files:**
- Create: `go.mod`
- Create: `cmd/server/main.go`
- Create: `internal/app/app.go`
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/health.go`
- Test: `internal/httpapi/health_test.go`

**Interfaces:**
- Produces: `httpapi.NewRouter(deps httpapi.Dependencies) http.Handler`
- Produces: `app.Run(ctx context.Context, addr string) error`

- [ ] **Step 1: Write the failing health endpoint test**

```go
package httpapi_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "probe.local/monitor/internal/httpapi"
)

func TestHealth(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
    rec := httptest.NewRecorder()

    httpapi.NewRouter(httpapi.Dependencies{}).ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
    }
    if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
        t.Fatalf("body = %q", got)
    }
}
```

- [ ] **Step 2: Run the test and observe the expected failure**

Run: `go test ./internal/httpapi -run TestHealth -v`

Expected: FAIL because `probe.local/monitor/internal/httpapi` and `NewRouter` do not exist.

- [ ] **Step 3: Add the module and minimal server implementation**

```go
// go.mod
module probe.local/monitor

go 1.24.0

require github.com/go-chi/chi/v5 v5.2.1
```

```go
// internal/httpapi/health.go
package httpapi

import (
    "encoding/json"
    "net/http"
)

func health(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

```go
// internal/httpapi/router.go
package httpapi

import (
    "net/http"

    "github.com/go-chi/chi/v5"
)

type Dependencies struct{}

func NewRouter(_ Dependencies) http.Handler {
    r := chi.NewRouter()
    r.Get("/api/health", health)
    return r
}
```

```go
// internal/app/app.go
package app

import (
    "context"
    "errors"
    "net/http"
    "time"

    "probe.local/monitor/internal/httpapi"
)

func Run(ctx context.Context, addr string) error {
    srv := &http.Server{Addr: addr, Handler: httpapi.NewRouter(httpapi.Dependencies{})}
    errCh := make(chan error, 1)
    go func() { errCh <- srv.ListenAndServe() }()

    select {
    case <-ctx.Done():
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        return srv.Shutdown(shutdownCtx)
    case err := <-errCh:
        if errors.Is(err, http.ErrServerClosed) {
            return nil
        }
        return err
    }
}
```

```go
// cmd/server/main.go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "probe.local/monitor/internal/app"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    addr := os.Getenv("TINYPROBE_ADDR")
    if addr == "" {
        addr = ":8080"
    }
    if err := app.Run(ctx, addr); err != nil {
        log.Fatal(err)
    }
}
```

- [ ] **Step 4: Format and verify the server**

Run: `go mod tidy && gofmt -w cmd internal && go test ./internal/httpapi -run TestHealth -v`

Expected: PASS for `TestHealth`.

- [ ] **Step 5: Commit the bootstrap**

```bash
git add go.mod go.sum cmd/server internal/app internal/httpapi
git commit -m "feat: bootstrap monitoring server"
```

### Task 2: Add SQLite WAL and deterministic migrations

**Files:**
- Modify: `go.mod`
- Create: `migrations/embed.go`
- Create: `migrations/001_core.sql`
- Create: `internal/db/open.go`
- Create: `internal/db/migrate.go`
- Test: `internal/db/migrate_test.go`

**Interfaces:**
- Produces: `db.Open(path string) (*sql.DB, error)`
- Produces: `db.ApplyMigrations(ctx context.Context, conn *sql.DB) error`
- Database tables: `admins`, `sessions`, `servers`, `schema_migrations`

- [ ] **Step 1: Write the migration test**

```go
package db_test

import (
    "context"
    "path/filepath"
    "testing"

    monitorDB "probe.local/monitor/internal/db"
)

func TestApplyMigrationsCreatesCoreTables(t *testing.T) {
    conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
    if err != nil { t.Fatal(err) }
    defer conn.Close()

    if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
        t.Fatal(err)
    }

    for _, table := range []string{"admins", "sessions", "servers", "schema_migrations"} {
        var name string
        err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
        if err != nil || name != table {
            t.Fatalf("table %s was not created: %v", table, err)
        }
    }
}
```

- [ ] **Step 2: Run the migration test and observe failure**

Run: `go test ./internal/db -run TestApplyMigrationsCreatesCoreTables -v`

Expected: FAIL because `db.Open` and `db.ApplyMigrations` do not exist.

- [ ] **Step 3: Add the schema and migration runner**

```sql
-- migrations/001_core.sql
CREATE TABLE admins (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE sessions (
    token_hash BLOB PRIMARY KEY,
    admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    token_hash BLOB NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1,
    agent_version TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

```go
// migrations/embed.go
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
```

```go
// internal/db/open.go
package db

import (
    "database/sql"
    "fmt"

    _ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
    conn, err := sql.Open("sqlite", path)
    if err != nil { return nil, err }
    conn.SetMaxOpenConns(1)
    if _, err = conn.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
        conn.Close()
        return nil, fmt.Errorf("configure sqlite: %w", err)
    }
    return conn, nil
}
```

```go
// internal/db/migrate.go
package db

import (
    "context"
    "database/sql"
    "fmt"
    "sort"

    "probe.local/monitor/migrations"
)

func ApplyMigrations(ctx context.Context, conn *sql.DB) error {
    if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
        return err
    }
    entries, err := migrations.Files.ReadDir(".")
    if err != nil { return err }
    names := make([]string, 0, len(entries))
    for _, entry := range entries {
        if !entry.IsDir() { names = append(names, entry.Name()) }
    }
    sort.Strings(names)
    for _, name := range names {
        var count int
        if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name=?`, name).Scan(&count); err != nil { return err }
        if count == 1 { continue }
        body, err := migrations.Files.ReadFile(name)
        if err != nil { return err }
        tx, err := conn.BeginTx(ctx, nil)
        if err != nil { return err }
        if _, err = tx.ExecContext(ctx, string(body)); err != nil { tx.Rollback(); return fmt.Errorf("apply %s: %w", name, err) }
        if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(name) VALUES (?)`, name); err != nil { tx.Rollback(); return err }
        if err = tx.Commit(); err != nil { return err }
    }
    return nil
}
```

Add the SQLite driver with `go get modernc.org/sqlite@v1.37.1`; commit the resulting `go.mod` and `go.sum`.

- [ ] **Step 4: Verify migrations and WAL**

Run: `go test ./internal/db -run TestApplyMigrationsCreatesCoreTables -v`

Expected: PASS. Add an assertion for `PRAGMA journal_mode` returning `wal` if the driver reports lowercase.

- [ ] **Step 5: Commit database bootstrap**

```bash
git add go.mod go.sum migrations internal/db
git commit -m "feat: add sqlite schema migrations"
```

### Task 3: Implement first-run administrator setup and sessions

**Files:**
- Create: `internal/auth/service.go`
- Create: `internal/auth/password.go`
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/handler.go`
- Test: `internal/auth/service_test.go`
- Test: `internal/auth/handler_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces: `auth.Service.SetupRequired(ctx) (bool, error)`
- Produces: `auth.Service.CreateAdmin(ctx, username, password string) error`
- Produces: `auth.Service.Login(ctx, username, password string) (rawToken string, expiresAt time.Time, err error)`
- Produces: `auth.Service.Authenticate(ctx, rawToken string) (Admin, error)`
- HTTP: `GET /api/setup/status`, `POST /api/setup`, `POST /api/login`, `POST /api/logout`, `GET /api/me`

- [ ] **Step 1: Write service tests for one-time setup and session authentication**

```go
func TestCreateAdminOnlyOnceAndLogin(t *testing.T) {
    svc := newTestService(t)
    ctx := context.Background()

    required, err := svc.SetupRequired(ctx)
    if err != nil || !required { t.Fatalf("required=%v err=%v", required, err) }
    if err := svc.CreateAdmin(ctx, "xiaodi", "correct horse battery staple"); err != nil { t.Fatal(err) }
    if err := svc.CreateAdmin(ctx, "second", "another password"); !errors.Is(err, auth.ErrSetupComplete) {
        t.Fatalf("second setup error = %v", err)
    }
    token, _, err := svc.Login(ctx, "xiaodi", "correct horse battery staple")
    if err != nil { t.Fatal(err) }
    admin, err := svc.Authenticate(ctx, token)
    if err != nil || admin.Username != "xiaodi" { t.Fatalf("admin=%+v err=%v", admin, err) }
}
```

- [ ] **Step 2: Run the tests and observe failure**

Run: `go test ./internal/auth -run TestCreateAdminOnlyOnceAndLogin -v`

Expected: FAIL because the authentication service is absent.

- [ ] **Step 3: Implement password hashing and hashed session tokens**

```go
// internal/auth/password.go
package auth

import "github.com/alexedwards/argon2id"

func hashPassword(password string) (string, error) {
    return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func verifyPassword(password, encoded string) (bool, error) {
    return argon2id.ComparePasswordAndHash(password, encoded)
}
```

```go
// internal/auth/service.go
package auth

type Admin struct { ID int64; Username string }

var (
    ErrSetupComplete = errors.New("administrator already exists")
    ErrInvalidLogin = errors.New("invalid username or password")
    ErrUnauthenticated = errors.New("unauthenticated")
)

type Service struct { db *sql.DB; now func() time.Time }

func NewService(conn *sql.DB) *Service { return &Service{db: conn, now: time.Now} }

func tokenPair() (string, []byte, error) {
    raw := make([]byte, 32)
    if _, err := rand.Read(raw); err != nil { return "", nil, err }
    text := base64.RawURLEncoding.EncodeToString(raw)
    digest := sha256.Sum256([]byte(text))
    return text, digest[:], nil
}
```

Implement the service methods with the following exact database behavior:

```go
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
    var count int
    if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&count); err != nil { return false, err }
    return count == 0, nil
}

func (s *Service) CreateAdmin(ctx context.Context, username, password string) error {
    if len(username) < 1 || len(username) > 64 || len(password) < 12 || len(password) > 128 { return ErrInvalidInput }
    required, err := s.SetupRequired(ctx)
    if err != nil { return err }
    if !required { return ErrSetupComplete }
    encoded, err := hashPassword(password)
    if err != nil { return err }
    _, err = s.db.ExecContext(ctx, `INSERT INTO admins(id, username, password_hash, created_at) VALUES (1, ?, ?, ?)`, username, encoded, s.now().UTC().Format(time.RFC3339Nano))
    if err != nil && strings.Contains(err.Error(), "UNIQUE") { return ErrSetupComplete }
    return err
}

func (s *Service) Login(ctx context.Context, username, password string) (string, time.Time, error) {
    var id int64
    var encoded string
    if err := s.db.QueryRowContext(ctx, `SELECT id, password_hash FROM admins WHERE username=?`, username).Scan(&id, &encoded); err != nil {
        if errors.Is(err, sql.ErrNoRows) { return "", time.Time{}, ErrInvalidLogin }
        return "", time.Time{}, err
    }
    ok, err := verifyPassword(password, encoded)
    if err != nil || !ok { return "", time.Time{}, ErrInvalidLogin }
    raw, digest, err := tokenPair()
    if err != nil { return "", time.Time{}, err }
    now := s.now().UTC()
    expires := now.Add(7 * 24 * time.Hour)
    _, err = s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash, admin_id, expires_at, created_at) VALUES (?, ?, ?, ?)`, digest, id, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
    return raw, expires, err
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (Admin, error) {
    digest := sha256.Sum256([]byte(rawToken))
    var admin Admin
    err := s.db.QueryRowContext(ctx, `SELECT a.id, a.username FROM sessions s JOIN admins a ON a.id=s.admin_id WHERE s.token_hash=? AND s.expires_at>?`, digest[:], s.now().UTC().Format(time.RFC3339Nano)).Scan(&admin.ID, &admin.Username)
    if errors.Is(err, sql.ErrNoRows) { return Admin{}, ErrUnauthenticated }
    if err != nil { return Admin{}, err }
    return admin, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
    digest := sha256.Sum256([]byte(rawToken))
    _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, digest[:])
    return err
}
```

Define `ErrInvalidInput = errors.New("invalid administrator input")`. Use a transaction or translate the unique constraint so concurrent first-run submissions still produce one administrator.

Add password hashing with `go get github.com/alexedwards/argon2id@v1.0.0`; commit the resulting module files.

- [ ] **Step 4: Add HTTP handlers and cookie middleware**

Use cookie name `tinyprobe_session`, `HttpOnly: true`, `SameSite: http.SameSiteStrictMode`, `Path: /`, `MaxAge: 604800`. Set `Secure` from `TINYPROBE_SECURE_COOKIES=true`, without adding certificate management.

Handler JSON contracts:

```json
{"setupRequired":true}
```

```json
{"username":"xiaodi","password":"correct horse battery staple"}
```

```json
{"id":1,"username":"xiaodi"}
```

Modify `httpapi.NewRouter` to accept a dependency struct:

```go
type Dependencies struct {
    Auth *auth.Service
}

func NewRouter(deps Dependencies) http.Handler
```

Protect `/api/me` and `/api/logout` with `auth.RequireSession`; leave setup status, setup creation, login, health, and future Agent ingestion public at the router level.

- [ ] **Step 5: Run service and handler tests**

Run: `go test ./internal/auth ./internal/httpapi -v`

Expected: PASS, including rejected second setup, rejected wrong password, expired session, authenticated `/api/me`, and logout deletion.

- [ ] **Step 6: Commit authentication**

```bash
git add go.mod go.sum internal/auth internal/httpapi internal/app
git commit -m "feat: add single-admin authentication"
```

### Task 4: Add server registration and independent Agent tokens

**Files:**
- Create: `internal/servers/service.go`
- Create: `internal/servers/handler.go`
- Test: `internal/servers/service_test.go`
- Test: `internal/servers/handler_test.go`
- Modify: `internal/httpapi/router.go`

**Interfaces:**
- Produces: `servers.Service.Create(ctx, name string) (Server, rawToken string, error)`
- Produces: `servers.Service.RotateToken(ctx, id int64) (rawToken string, error)`
- Produces: `servers.Service.AuthenticateToken(ctx, rawToken string) (Server, error)`
- HTTP: `GET/POST /api/servers`, `PATCH/DELETE /api/servers/{id}`, `POST /api/servers/{id}/token`

- [ ] **Step 1: Write tests for token creation, one-time return, rotation, and disablement**

```go
func TestRotateTokenInvalidatesPreviousToken(t *testing.T) {
    svc := newServerService(t)
    ctx := context.Background()
    server, oldToken, err := svc.Create(ctx, "home-lab")
    if err != nil { t.Fatal(err) }

    newToken, err := svc.RotateToken(ctx, server.ID)
    if err != nil { t.Fatal(err) }
    if _, err := svc.AuthenticateToken(ctx, oldToken); !errors.Is(err, servers.ErrInvalidToken) {
        t.Fatalf("old token error = %v", err)
    }
    got, err := svc.AuthenticateToken(ctx, newToken)
    if err != nil || got.ID != server.ID { t.Fatalf("got=%+v err=%v", got, err) }
}
```

- [ ] **Step 2: Run the server service test and observe failure**

Run: `go test ./internal/servers -run TestRotateTokenInvalidatesPreviousToken -v`

Expected: FAIL because the package is absent.

- [ ] **Step 3: Implement the registry service**

Use tokens formatted as `tp_` plus 43 base64url characters. Store `sha256.Sum256([]byte(rawToken))` in `servers.token_hash`. `Create` returns the raw token once; `List` and `Get` never return it. `Delete` removes the row. `SetEnabled(false)` makes token authentication fail with `ErrDisabled`.

```go
type Server struct {
    ID           int64     `json:"id"`
    Name         string    `json:"name"`
    Enabled      bool      `json:"enabled"`
    AgentVersion string    `json:"agentVersion"`
    CreatedAt    time.Time `json:"createdAt"`
    UpdatedAt    time.Time `json:"updatedAt"`
}
```

- [ ] **Step 4: Add authenticated registry handlers**

Return `201` with the following response only from create and rotate operations:

```json
{"server":{"id":1,"name":"home-lab","enabled":true,"agentVersion":"","createdAt":"2026-07-26T03:00:00Z","updatedAt":"2026-07-26T03:00:00Z"},"token":"tp_example-token-value"}
```

The actual test must assert the `tp_` prefix and successful authentication, not the illustrative token text. All registry routes require an administrator session.

- [ ] **Step 5: Verify registry behavior**

Run: `go test ./internal/servers ./internal/httpapi -v`

Expected: PASS for create, list without token, rename, disable, rotate, delete, unauthenticated rejection, and disabled-token rejection.

- [ ] **Step 6: Commit server registration**

```bash
git add internal/servers internal/httpapi
git commit -m "feat: add monitored server registry"
```

### Task 5: Define the Agent protocol and Linux collector

**Files:**
- Create: `internal/protocol/report.go`
- Create: `internal/agent/source.go`
- Create: `internal/agent/gopsutil_source.go`
- Create: `internal/agent/collector.go`
- Create: `internal/agent/client.go`
- Create: `internal/agent/runner.go`
- Create: `cmd/agent/main.go`
- Test: `internal/agent/collector_test.go`
- Test: `internal/agent/runner_test.go`

**Interfaces:**
- Produces: `protocol.AgentReport`
- Produces: `agent.Collector.Collect(ctx context.Context) (protocol.AgentReport, error)`
- Produces: `agent.ReportClient.Send(ctx context.Context, report protocol.AgentReport) error`
- Produces: `agent.Runner.Run(ctx context.Context) error`

- [ ] **Step 1: Define the shared report contract and collector test fixture**

```go
package protocol

type HostInfo struct {
    Hostname        string `json:"hostname"`
    OS              string `json:"os"`
    Platform        string `json:"platform"`
    PlatformVersion string `json:"platformVersion"`
    KernelVersion   string `json:"kernelVersion"`
    Architecture    string `json:"architecture"`
    CPUModel        string `json:"cpuModel"`
    CPUCores        int    `json:"cpuCores"`
    PrimaryIP       string `json:"primaryIp"`
    BootTimeUnix    int64  `json:"bootTimeUnix"`
    UptimeSeconds   uint64 `json:"uptimeSeconds"`
}

type CPUStats struct {
    UsagePercent float64 `json:"usagePercent"`
    Load1        float64 `json:"load1"`
    Load5        float64 `json:"load5"`
    Load15       float64 `json:"load15"`
}

type MemoryStats struct {
    TotalBytes     uint64 `json:"totalBytes"`
    UsedBytes      uint64 `json:"usedBytes"`
    SwapTotalBytes uint64 `json:"swapTotalBytes"`
    SwapUsedBytes  uint64 `json:"swapUsedBytes"`
}

type DiskStats struct {
    Mountpoint string `json:"mountpoint"`
    TotalBytes uint64 `json:"totalBytes"`
    UsedBytes  uint64 `json:"usedBytes"`
}

type DiskIOStats struct {
    ReadBytesPerSecond  uint64 `json:"readBytesPerSecond"`
    WriteBytesPerSecond uint64 `json:"writeBytesPerSecond"`
}

type NetworkStats struct {
    Interface              string `json:"interface"`
    UploadBytesPerSecond   uint64 `json:"uploadBytesPerSecond"`
    DownloadBytesPerSecond uint64 `json:"downloadBytesPerSecond"`
    TotalUploadBytes       uint64 `json:"totalUploadBytes"`
    TotalDownloadBytes     uint64 `json:"totalDownloadBytes"`
}

type AgentReport struct {
    CollectedAtUnix int64        `json:"collectedAtUnix"`
    AgentVersion   string       `json:"agentVersion"`
    Host           HostInfo     `json:"host"`
    CPU            CPUStats     `json:"cpu"`
    Memory         MemoryStats  `json:"memory"`
    Disks          []DiskStats  `json:"disks"`
    DiskIO         DiskIOStats  `json:"diskIo"`
    Network        NetworkStats `json:"network"`
}
```

Write `collector_test.go` against a fake `Source` that returns two persistent mounts and one temporary mount. Assert that the temporary mount is absent, the default-route interface is selected, the highest-usage mount can be derived, and all byte values remain integers.

- [ ] **Step 2: Run the collector test and observe failure**

Run: `go test ./internal/agent -run TestCollectorUsesPersistentMountsAndDefaultRoute -v`

Expected: FAIL because the collector is absent.

- [ ] **Step 3: Implement a testable metric source and collector**

```go
type Source interface {
    Host(context.Context) (protocol.HostInfo, error)
    CPU(context.Context) (protocol.CPUStats, error)
    Memory(context.Context) (protocol.MemoryStats, error)
    PersistentDisks(context.Context) ([]protocol.DiskStats, error)
    DiskIO(context.Context) (protocol.DiskIOStats, error)
    DefaultRouteNetwork(context.Context) (protocol.NetworkStats, error)
}

type Collector struct {
    Source       Source
    AgentVersion string
    Now          func() time.Time
}

func (c Collector) Collect(ctx context.Context) (protocol.AgentReport, error) {
    host, err := c.Source.Host(ctx); if err != nil { return protocol.AgentReport{}, err }
    cpu, err := c.Source.CPU(ctx); if err != nil { return protocol.AgentReport{}, err }
    memory, err := c.Source.Memory(ctx); if err != nil { return protocol.AgentReport{}, err }
    disks, err := c.Source.PersistentDisks(ctx); if err != nil { return protocol.AgentReport{}, err }
    diskIO, err := c.Source.DiskIO(ctx); if err != nil { return protocol.AgentReport{}, err }
    network, err := c.Source.DefaultRouteNetwork(ctx); if err != nil { return protocol.AgentReport{}, err }
    return protocol.AgentReport{CollectedAtUnix: c.Now().Unix(), AgentVersion: c.AgentVersion, Host: host, CPU: cpu, Memory: memory, Disks: disks, DiskIO: diskIO, Network: network}, nil
}
```

Install `github.com/shirou/gopsutil/v4@v4.25.5` and implement `GopsutilSource` with it. Read `/proc/net/route` to find the interface whose destination and mask are both zero. Exclude filesystems `tmpfs`, `devtmpfs`, `squashfs`, `overlay`, `proc`, `sysfs`, `cgroup`, and `cgroup2`. Aggregate disk I/O counters across physical devices, calculate disk/network rates by retaining the previous counters and monotonic sample time, and do not attach device I/O rates to mountpoints because that mapping is not reliable across LVM, RAID, and containers.

- [ ] **Step 4: Implement report sending and retry scheduling**

`ReportClient.Send` posts to `/api/agent/v1/report`, sets `Authorization: Bearer <token>`, `Content-Type: application/json`, and uses a 5-second request timeout.

`Runner.Run` collects immediately, then every 5 seconds. After send failures, use retry delays `5s, 10s, 20s, 40s, 60s`; a successful send resets the delay. The Runner sends only the newest report and never writes a local queue.

Environment variables for `cmd/agent`:

```text
TINYPROBE_SERVER_URL=https://monitor.example/api/agent/v1/report
TINYPROBE_AGENT_TOKEN=tp_secret
TINYPROBE_AGENT_VERSION=dev
```

The example URL is documentation text only; code must require a non-empty absolute `http` or `https` URL and must not default to an external host.

- [ ] **Step 5: Verify Agent behavior**

Run: `go test ./internal/agent ./internal/protocol -v`

Expected: PASS for filtering, default-route selection, first report, 5-second cadence, backoff cap, cancellation, and no replay queue.

- [ ] **Step 6: Commit the Agent**

```bash
git add go.mod go.sum cmd/agent internal/agent internal/protocol
git commit -m "feat: add linux metrics agent"
```

### Task 6: Ingest reports, track latest state, detect offline, and stream SSE

**Files:**
- Create: `internal/live/store.go`
- Create: `internal/live/hub.go`
- Create: `internal/live/sweeper.go`
- Create: `internal/live/handler.go`
- Create: `internal/live/ingest.go`
- Test: `internal/live/store_test.go`
- Test: `internal/live/ingest_test.go`
- Test: `internal/live/sse_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces: `live.Store.Upsert(server servers.Server, report protocol.AgentReport, receivedAt time.Time) Snapshot`
- Produces: `live.Store.MarkOffline(cutoff time.Time) []Snapshot`
- Produces: `live.Hub.Publish(Event)` and `live.Hub.Subscribe() (<-chan Event, cancel func())`
- HTTP: `POST /api/agent/v1/report`, `GET /api/live`, `GET /api/servers/status`, `GET /api/servers/{id}/status`

- [ ] **Step 1: Write store tests for online and offline transitions**

```go
func TestMarkOfflineAfterCutoff(t *testing.T) {
    store := live.NewStore()
    now := time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC)
    server := servers.Server{ID: 7, Name: "home-lab", Enabled: true}
    store.Upsert(server, protocol.AgentReport{}, now.Add(-31*time.Second))

    changed := store.MarkOffline(now.Add(-30 * time.Second))

    if len(changed) != 1 || changed[0].Online {
        t.Fatalf("changed = %+v", changed)
    }
    if again := store.MarkOffline(now.Add(-30 * time.Second)); len(again) != 0 {
        t.Fatalf("second sweep emitted duplicate transition: %+v", again)
    }
}
```

- [ ] **Step 2: Run the live tests and observe failure**

Run: `go test ./internal/live -run TestMarkOfflineAfterCutoff -v`

Expected: FAIL because the live package is absent.

- [ ] **Step 3: Implement snapshot storage and event publication**

```go
type Snapshot struct {
    ServerID       int64                `json:"serverId"`
    ServerName     string               `json:"serverName"`
    Online         bool                 `json:"online"`
    LastReceivedAt time.Time            `json:"lastReceivedAt"`
    SourceIP       string               `json:"sourceIp"`
    Report         protocol.AgentReport `json:"report"`
}

type Event struct {
    Type     string   `json:"type"`
    Snapshot Snapshot `json:"snapshot"`
}
```

Protect `Store` with `sync.RWMutex`. `Upsert` returns an online snapshot and emits no duplicate state semantics itself. `MarkOffline` changes only currently online snapshots whose `LastReceivedAt` is before the supplied cutoff.

Implement `Hub` as a bounded fan-out with channel capacity 8 per subscriber. If a subscriber is slow, drop the stale event for that subscriber rather than blocking ingestion; the browser will refetch `/api/servers/status` after reconnect.

- [ ] **Step 4: Implement authenticated ingestion and the 5-second sweeper**

Ingest flow:

1. Read Bearer Token.
2. Authenticate through `servers.Service.AuthenticateToken`.
3. Limit JSON body to 256 KiB.
4. Reject an empty hostname, no disks, negative percentages, percentages above 100, or a non-positive collection timestamp. Do not reject clock skew; online state and history buckets use the server receipt time.
5. Capture `RemoteAddr` as `SourceIP` after removing the port.
6. Update `servers.agent_version`.
7. Upsert the snapshot and publish `snapshot.updated`.
8. Return `204 No Content`.

Start a ticker every 5 seconds. Call `MarkOffline(now.Add(-30*time.Second))`; publish `snapshot.offline` for every returned transition.

- [ ] **Step 5: Implement authenticated status APIs and SSE**

`GET /api/servers/status` returns all registered servers, including an offline placeholder for registered servers that have never reported. Sort by online ascending, then server name ascending, so offline/abnormal items appear first.

`GET /api/live` writes:

```text
event: snapshot.updated
data: {"type":"snapshot.updated","snapshot":{"serverId":1}}

```

Send a comment heartbeat `: keepalive` every 15 seconds. Set `Content-Type: text/event-stream`, `Cache-Control: no-cache`, and `X-Accel-Buffering: no`.

- [ ] **Step 6: Verify ingestion, offline, and SSE**

Run: `go test ./internal/live ./internal/servers ./internal/httpapi -v`

Expected: PASS for valid Token, invalid Token, disabled server, payload validation, source IP capture, online event, single offline event, authenticated SSE, heartbeat, and slow-subscriber isolation.

- [ ] **Step 7: Commit live monitoring**

```bash
git add internal/live internal/httpapi internal/app internal/servers
git commit -m "feat: add live metric ingestion and sse"
```

### Task 7: Scaffold the React application and authentication flows

**Files:**
- Create: `web/package.json`
- Create: `web/tsconfig.json`
- Create: `web/vite.config.ts`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/app/App.tsx`
- Create: `web/src/api/client.ts`
- Create: `web/src/auth/SetupPage.tsx`
- Create: `web/src/auth/LoginPage.tsx`
- Create: `web/src/auth/AuthGate.tsx`
- Create: `web/src/styles/tokens.css`
- Create: `web/src/styles/base.css`
- Create: `web/src/test/setup.ts`
- Test: `web/src/auth/AuthGate.test.tsx`
- Create: `internal/webui/embed.go`
- Modify: `internal/httpapi/router.go`

**Interfaces:**
- Produces: `api.getSetupStatus()`, `api.setupAdmin()`, `api.login()`, `api.logout()`, `api.getMe()`
- Produces: `AuthGate` rendering setup, login, or authenticated application shell
- Server fallback: non-API GET requests return embedded `index.html`

- [ ] **Step 1: Create package metadata and the failing authentication gate test**

```json
{
  "name": "tinyprobe-web",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "vitest",
    "lint": "tsc -b --pretty false"
  },
  "dependencies": {
    "react": "^19.1.0",
    "react-dom": "^19.1.0"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.6.3",
    "@testing-library/react": "^16.3.0",
    "@types/react": "^19.1.0",
    "@types/react-dom": "^19.1.0",
    "@vitejs/plugin-react": "^4.6.0",
    "jsdom": "^26.1.0",
    "typescript": "^5.8.3",
    "vite": "^7.0.0",
    "vitest": "^3.2.4"
  }
}
```

```tsx
it("shows first-run setup when no administrator exists", async () => {
  server.use(http.get("/api/setup/status", () => HttpResponse.json({ setupRequired: true })));
  render(<AuthGate><div>private app</div></AuthGate>);
  expect(await screen.findByRole("heading", { name: "创建管理员" })).toBeInTheDocument();
  expect(screen.queryByText("private app")).not.toBeInTheDocument();
});
```

Use an explicit fetch mock in `web/src/test/setup.ts`; do not add MSW unless the implementation needs more than the five authentication endpoints.

- [ ] **Step 2: Install dependencies and observe failure**

Run: `pnpm --dir web install && pnpm --dir web test -- --run AuthGate.test.tsx`

Expected: FAIL because `AuthGate` and the API client do not exist.

- [ ] **Step 3: Implement the typed API client and authentication gate**

```ts
export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    credentials: "same-origin"
  });
  if (!response.ok) throw new ApiError(response.status, await response.text());
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
```

`AuthGate` first calls `/api/setup/status`. If setup is required, render `SetupPage`. Otherwise call `/api/me`; render `LoginPage` on `401`, and children on success. Setup and login forms require username and password, disable during submission, preserve visible labels, and show server errors inline.

- [ ] **Step 4: Implement the approved visual tokens and app shell**

```css
:root {
  --color-bg: oklch(1 0 0);
  --color-surface: oklch(.975 .006 330);
  --color-surface-strong: oklch(.945 .012 330);
  --color-ink: oklch(.20 .025 330);
  --color-muted: oklch(.50 .025 330);
  --color-primary: oklch(.47 .14 330);
  --color-primary-soft: oklch(.94 .035 330);
  --color-success: oklch(.67 .16 151);
  --color-warning: oklch(.72 .16 60);
  --color-danger: oklch(.61 .20 25);
  --radius-panel: 14px;
  --radius-control: 10px;
  --shadow-panel: 0 4px 8px oklch(.25 .02 330 / .07);
}
```

Use one system sans-serif stack. Keep panel radius at 14px, do not combine a visible border with a shadow wider than 8px, and add `prefers-reduced-motion` rules.

- [ ] **Step 5: Embed the production frontend**

Configure Vite output to `../internal/webui/dist`. `internal/webui/embed.go` embeds `dist/*` and exposes `Handler() http.Handler`. API paths must return JSON 404 rather than the SPA fallback.

- [ ] **Step 6: Verify authentication UI and production build**

Run: `pnpm --dir web test -- --run && pnpm --dir web build && go test ./internal/webui ./internal/httpapi -v`

Expected: all tests PASS and `internal/webui/dist/index.html` exists.

- [ ] **Step 7: Commit the frontend shell**

```bash
git add web pnpm-lock.yaml internal/webui internal/httpapi
git commit -m "feat: add admin setup and login ui"
```

### Task 8: Build the responsive live overview and server detail pages

**Files:**
- Create: `web/src/api/types.ts`
- Create: `web/src/live/useServerSnapshots.ts`
- Create: `web/src/components/AppNav.tsx`
- Create: `web/src/components/SummaryPanel.tsx`
- Create: `web/src/components/ServerRow.tsx`
- Create: `web/src/components/MetricBar.tsx`
- Create: `web/src/pages/OverviewPage.tsx`
- Create: `web/src/pages/ServerDetailPage.tsx`
- Create: `web/src/styles/dashboard.css`
- Test: `web/src/pages/OverviewPage.test.tsx`
- Test: `web/src/live/useServerSnapshots.test.tsx`
- Modify: `web/src/app/App.tsx`

**Interfaces:**
- Consumes: `GET /api/servers/status`, `GET /api/servers/{id}/status`, `GET /api/live`
- Produces: `useServerSnapshots(): { snapshots, connected, error, refresh }`
- Produces: overview with abnormal-first rows and a read-only detail route using browser history

- [ ] **Step 1: Write the overview sorting and live-update tests**

```tsx
it("places offline servers before online servers", async () => {
  mockStatusResponse([
    snapshot({ serverId: 1, serverName: "online", online: true }),
    snapshot({ serverId: 2, serverName: "offline", online: false })
  ]);
  render(<OverviewPage />);
  const rows = await screen.findAllByTestId("server-row");
  expect(rows[0]).toHaveTextContent("offline");
  expect(rows[1]).toHaveTextContent("online");
});
```

```tsx
it("merges an SSE snapshot by server id", async () => {
  const { result } = renderHook(() => useServerSnapshots());
  await waitFor(() => expect(result.current.snapshots).toHaveLength(1));
  fakeEventSource.emit("snapshot.updated", snapshot({ serverId: 1, online: true, cpuUsage: 42 }));
  await waitFor(() => expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(42));
});
```

- [ ] **Step 2: Run the frontend tests and observe failure**

Run: `pnpm --dir web test -- --run OverviewPage.test.tsx useServerSnapshots.test.tsx`

Expected: FAIL because the hook and page do not exist.

- [ ] **Step 3: Implement API types and SSE state merging**

Mirror the Go JSON contract exactly in `web/src/api/types.ts`. `useServerSnapshots` performs an initial fetch, opens one `EventSource('/api/live')`, parses `snapshot.updated` and `snapshot.offline`, replaces snapshots by `serverId`, sorts offline first and then by `serverName`, and refetches after `EventSource` reconnects.

The hook must close `EventSource` on unmount and expose a visible connection state without blocking current data.

- [ ] **Step 4: Implement the approved overview**

The page must contain:

- Brand and top navigation.
- Friendly status copy based on online/offline counts.
- One continuous summary panel for total, online, and abnormal counts.
- One visually distinct network summary panel.
- Horizontal server rows with status, OS, uptime, CPU, memory, highest disk usage, upload, download, and cumulative traffic.
- A footer note stating the 5-second update cadence.

Use semantic status colors only. Metric bars use green below 60%, orange from 60–84.99%, and red from 85% upward. Do not infer alert state from these display colors; alert rules arrive in Phase 3.

- [ ] **Step 5: Implement the read-only server detail page**

Use paths `/servers/:id` interpreted from `window.location.pathname` to avoid adding a router dependency. The detail page shows back navigation, status, last report, system fields, current CPU/load/memory/Swap/disk/network values, and a disabled history-range strip labeled `实时`, `1天`, `7天`, `30天`. Only `实时` is active in Phase 1; Phase 2 activates history ranges.

- [ ] **Step 6: Verify desktop and mobile behavior**

Run: `pnpm --dir web test -- --run && pnpm --dir web build`

Expected: PASS. Manually verify at widths 1440, 980, 640, and 390 pixels: no horizontal overflow; tablet hides cumulative fields; mobile retains name, status, CPU, memory, disk, and current network.

- [ ] **Step 7: Commit the live UI**

```bash
git add web/src
git commit -m "feat: add live monitoring dashboard"
```

### Task 9: Build server management and one-time Agent installation flow

**Files:**
- Create: `web/src/servers/api.ts`
- Create: `web/src/components/ServerForm.tsx`
- Create: `web/src/components/ServerInstallPanel.tsx`
- Create: `web/src/pages/ServersPage.tsx`
- Test: `web/src/components/ServerInstallPanel.test.tsx`
- Test: `web/src/pages/ServersPage.test.tsx`
- Create: `internal/httpapi/downloads.go`
- Test: `internal/httpapi/downloads_test.go`
- Modify: `internal/httpapi/router.go`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/components/AppNav.tsx`
- Modify: `web/src/styles/dashboard.css`

**Interfaces:**
- Consumes: `GET/POST /api/servers`, `PATCH/DELETE /api/servers/{id}`, `POST /api/servers/{id}/token`
- Produces: `/downloads/tinyprobe-agent-linux-amd64` and `/downloads/tinyprobe-agent-linux-arm64`
- Produces: create, rename, enable/disable, delete, Token rotation, and one-time install instructions

- [ ] **Step 1: Write tests for one-time Token display and destructive-action confirmation**

```tsx
it("shows the token only in the create response panel", async () => {
  createServer.mockResolvedValue({ server: serverFixture, token: "tp_secret" });
  render(<ServersPage />);
  await userEvent.click(screen.getByRole("button", { name: "添加服务器" }));
  await userEvent.type(screen.getByLabelText("服务器名称"), "home-lab");
  await userEvent.click(screen.getByRole("button", { name: "创建" }));
  expect(await screen.findByText("tp_secret")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "我已保存 Token" }));
  expect(screen.queryByText("tp_secret")).not.toBeInTheDocument();
});
```

```tsx
it("requires an inline confirmation before deleting a server", async () => {
  render(<ServersPage />);
  await userEvent.click(await screen.findByRole("button", { name: "删除 home-lab" }));
  expect(screen.getByText("删除后该 Agent Token 将立即失效。")).toBeInTheDocument();
  expect(deleteServer).not.toHaveBeenCalled();
});
```

- [ ] **Step 2: Run management UI tests and observe failure**

Run: `pnpm --dir web test -- --run ServerInstallPanel.test.tsx ServersPage.test.tsx`

Expected: FAIL because the management page does not exist.

- [ ] **Step 3: Implement the Agent download handler**

```go
func AgentDownloadHandler(files fs.FS) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        name := strings.TrimPrefix(r.URL.Path, "/downloads/")
        if name != "tinyprobe-agent-linux-amd64" && name != "tinyprobe-agent-linux-arm64" {
            http.NotFound(w, r)
            return
        }
        body, err := fs.ReadFile(files, name)
        if err != nil { http.NotFound(w, r); return }
        w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
        http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
    })
}
```

The test uses `fstest.MapFS`, asserts both exact names return `200`, an unknown name returns `404`, and `Content-Disposition` contains the expected filename. Add the handler to the public router; do not expose directory listing.

- [ ] **Step 4: Implement typed server-management requests**

```ts
export type ServerCreateResult = { server: ServerRecord; token: string };

export const serverApi = {
  list: () => request<ServerRecord[]>("/api/servers"),
  create: (name: string) => request<ServerCreateResult>("/api/servers", { method: "POST", body: JSON.stringify({ name }) }),
  update: (id: number, input: { name?: string; enabled?: boolean }) => request<ServerRecord>(`/api/servers/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  remove: (id: number) => request<void>(`/api/servers/${id}`, { method: "DELETE" }),
  rotateToken: (id: number) => request<{ token: string }>(`/api/servers/${id}/token`, { method: "POST" })
};
```

- [ ] **Step 5: Implement the inline installation panel**

After create or rotation, keep the raw Token only in component memory. Show architecture tabs `amd64` and `arm64`, a copyable binary download command, environment file, and systemd unit. Build URLs from `window.location.origin`; never place a sample external hostname in the generated command.

```text
sudo install -m 0755 tinyprobe-agent-linux-amd64 /usr/local/bin/tinyprobe-agent
sudo sh -c 'printf "%s\n" "TINYPROBE_SERVER_URL=<current-origin>/api/agent/v1/report" "TINYPROBE_AGENT_TOKEN=<one-time-token>" > /etc/tinyprobe-agent.env'
sudo systemctl enable --now tinyprobe-agent
```

The displayed systemd unit must set `EnvironmentFile=/etc/tinyprobe-agent.env`, `ExecStart=/usr/local/bin/tinyprobe-agent`, `Restart=always`, `RestartSec=5`, `NoNewPrivileges=true`, and `ProtectSystem=strict` with the minimum read-only exceptions required for `/proc` and `/sys` collection.

- [ ] **Step 6: Implement server list actions**

Use inline editing and confirmation rows, not modals. Disable/enable updates the row immediately after success. Delete explains that live and future historical data for that server will be removed by database cascade. Token rotation requires confirmation and reopens the one-time install panel.

- [ ] **Step 7: Verify management UX**

Run: `pnpm --dir web test -- --run && pnpm --dir web build`

Expected: PASS for Agent downloads, create, rename, disable, enable, delete confirmation, rotate confirmation, Token disappearance, architecture switch, command copy, and API failure messages.

- [ ] **Step 8: Commit server management UI**

```bash
git add web/src internal/httpapi
git commit -m "feat: add server and agent setup ui"
```

### Task 10: Compose the application, container build, and acceptance smoke test

**Files:**
- Modify: `internal/app/app.go`
- Modify: `cmd/server/main.go`
- Create: `deploy/Dockerfile`
- Create: `docker-compose.yml`
- Create: `cmd/loadgen/main.go`
- Create: `README.md`
- Test: `internal/app/app_test.go`

**Interfaces:**
- Produces: one `tinyprobe` container on port `8080`
- Produces: Agent downloads at `/downloads/tinyprobe-agent-linux-amd64` and `/downloads/tinyprobe-agent-linux-arm64`
- Produces: `loadgen` that simulates 10 authenticated Agents without writing production data formats outside the public Agent protocol

- [ ] **Step 1: Write the application composition smoke test**

```go
func TestApplicationStartsWithFreshDatabase(t *testing.T) {
    instance, err := app.New(app.Config{
        DatabasePath: filepath.Join(t.TempDir(), "monitor.db"),
        AgentFiles: fstest.MapFS{
            "tinyprobe-agent-linux-amd64": {Data: []byte("amd64")},
            "tinyprobe-agent-linux-arm64": {Data: []byte("arm64")},
        },
    })
    if err != nil { t.Fatal(err) }
    defer instance.Close()

    rec := httptest.NewRecorder()
    instance.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
    if rec.Code != http.StatusOK { t.Fatalf("status=%d", rec.Code) }
}
```

- [ ] **Step 2: Run the application test and observe failure**

Run: `go test ./internal/app -run TestApplicationStartsWithFreshDatabase -v`

Expected: FAIL because `app.New`, dependency composition, and Agent downloads are not complete.

- [ ] **Step 3: Centralize application lifecycle**

`app.New(Config)` must:

1. Open SQLite and apply migrations.
2. Construct auth, servers, live store, SSE hub, ingest handler, and sweeper.
3. Construct the final router and embedded Web UI.
4. Return an `Application` with `Handler()`, `RunBackground(ctx)`, and `Close()`.

`cmd/server` reads:

```text
TINYPROBE_ADDR=:8080
TINYPROBE_DB_PATH=/data/tinyprobe.db
TINYPROBE_SECURE_COOKIES=false
```

Do not require a secret environment variable because sessions and Agent tokens are random database-backed values, not signed stateless cookies.

- [ ] **Step 4: Add deterministic multi-stage container builds**

`deploy/Dockerfile` stages:

1. Node stage: `pnpm install --frozen-lockfile`, `pnpm build`.
2. Go stage: copy Web distribution, run tests, build server for Linux, build Agent for Linux `amd64` and `arm64` with `CGO_ENABLED=0`.
3. Alpine runtime: create a non-root user, copy server and Agent downloads, create `/data`, expose `8080`, run the server.

`docker-compose.yml` must contain one service, one named volume, `restart: unless-stopped`, and a health check against `/api/health`.

- [ ] **Step 5: Implement the 10-Agent load generator**

`cmd/loadgen` accepts `-base-url`, `-token-file`, `-agents`, and `-duration`. The token file contains one token per line. For each token, send a deterministic valid `protocol.AgentReport` every 5 seconds with a distinct hostname and bounded metric values. Exit non-zero on any non-2xx response.

- [ ] **Step 6: Run the Phase 1 verification suite**

Run:

```bash
go test ./...
pnpm --dir web test -- --run
pnpm --dir web build
docker compose build
docker compose up -d
curl --fail http://localhost:8080/api/health
```

Expected: all tests PASS, build succeeds, container becomes healthy, and health returns `{"status":"ok"}`.

Perform the acceptance flow manually:

1. Create the administrator.
2. Add one server and copy its Token.
3. Start the Agent against the local service.
4. Confirm live values update within 10 seconds.
5. Stop the Agent.
6. Confirm the server becomes offline within 30–45 seconds.

- [ ] **Step 7: Commit Phase 1 delivery**

```bash
git add internal/app cmd/server cmd/loadgen deploy docker-compose.yml README.md
git commit -m "feat: deliver realtime monitoring core"
```
