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

func TestRuntimeWiresLiveIngestionAndStopsSweeper(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	done := runtime.startSweeper(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime sweeper did not stop after cancellation")
	}
}
