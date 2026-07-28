package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/protocol"
)

func TestRuntimeWiresLiveIngestionToHistoryAndStopsBackground(t *testing.T) {
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntime(conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	server, token, err := runtime.servers.Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	report := protocol.AgentReport{
		CollectedAtUnix: 1,
		Host:            protocol.HostInfo{Hostname: "probe-host"},
		Disks:           []protocol.DiskStats{{Mountpoint: "/"}},
	}
	body, _ := json.Marshal(report)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	runtime.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	if snapshot, ok := runtime.store.Get(server.ID); !ok || !snapshot.Online {
		t.Fatalf("snapshot=%+v ok=%v", snapshot, ok)
	}
	snapshot, _ := runtime.store.Get(server.ID)
	minute := snapshot.LastReceivedAt.UTC().Truncate(time.Minute)
	if err := runtime.historyAggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	records, err := runtime.historyStore.Query(context.Background(), server.ID, minute.Unix(), minute.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ServerID != server.ID || records[0].MinuteUnix != minute.Unix() {
		t.Fatalf("history records=%+v", records)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := runtime.startBackground(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime background work did not stop after cancellation")
	}
}

func TestRuntimeBackgroundDoneWaitsForEveryRunner(t *testing.T) {
	first := newBlockingBackgroundRunner()
	second := newBlockingBackgroundRunner()
	runtime := &runtime{background: []backgroundRunner{first, second}}
	ctx, cancel := context.WithCancel(context.Background())
	done := runtime.startBackground(ctx)
	first.waitStarted(t)
	second.waitStarted(t)

	cancel()
	first.waitCanceled(t)
	second.waitCanceled(t)
	select {
	case <-done:
		t.Fatal("background done closed before runners returned")
	case <-time.After(20 * time.Millisecond):
	}

	close(first.release)
	select {
	case <-done:
		t.Fatal("background done closed while second runner was blocked")
	case <-time.After(20 * time.Millisecond):
	}
	close(second.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background done did not close after every runner returned")
	}
}

type blockingBackgroundRunner struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func newBlockingBackgroundRunner() *blockingBackgroundRunner {
	return &blockingBackgroundRunner{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (r *blockingBackgroundRunner) Run(ctx context.Context) {
	close(r.started)
	<-ctx.Done()
	close(r.canceled)
	<-r.release
}

func (r *blockingBackgroundRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("background runner did not start")
	}
}

func (r *blockingBackgroundRunner) waitCanceled(t *testing.T) {
	t.Helper()
	select {
	case <-r.canceled:
	case <-time.After(time.Second):
		t.Fatal("background runner did not observe cancellation")
	}
}
