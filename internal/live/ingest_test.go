package live_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

func TestIngestAuthenticatesStoresPublishesAndUpdatesVersion(t *testing.T) {
	router, serverService, store, hub, server, token := newLiveRouter(t)
	events, cancel := hub.Subscribe()
	defer cancel()
	report := validReport()
	body, _ := json.Marshal(report)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "[2001:db8::7]:4567"
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	snapshot, ok := store.Get(server.ID)
	if !ok || !snapshot.Online || snapshot.SourceIP != "2001:db8::7" || snapshot.Report.Host.Hostname != "probe-host" {
		t.Fatalf("snapshot=%+v ok=%v", snapshot, ok)
	}
	if time.Since(snapshot.LastReceivedAt) > time.Second {
		t.Fatalf("last received=%s", snapshot.LastReceivedAt)
	}
	select {
	case event := <-events:
		if event.Type != "snapshot.updated" || event.Snapshot.ServerID != server.ID {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("updated event not published")
	}
	updated, err := serverService.Get(context.Background(), server.ID)
	if err != nil || updated.AgentVersion != report.AgentVersion {
		t.Fatalf("server=%+v err=%v", updated, err)
	}
}

func TestIngestRejectsMissingInvalidAndDisabledTokens(t *testing.T) {
	router, serverService, _, _, server, token := newLiveRouter(t)
	body, _ := json.Marshal(validReport())
	for _, test := range []struct {
		name   string
		header string
		want   int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic " + token, want: http.StatusUnauthorized},
		{name: "invalid", header: "Bearer invalid", want: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
			req.Header.Set("Authorization", test.header)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != test.want {
				t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
	if _, err := serverService.Update(context.Background(), server.ID, nil, boolPtr(false)); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestIngestRejectsMalformedOversizedAndInvalidReports(t *testing.T) {
	router, _, _, _, _, token := newLiveRouter(t)
	serve := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := serve([]byte(`{"host":`)); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed code=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec := serve([]byte(strings.Repeat(" ", 256*1024+1))); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized code=%d body=%q", rec.Code, rec.Body.String())
	}

	tests := []struct {
		name   string
		mutate func(*protocol.AgentReport)
	}{
		{name: "blank hostname", mutate: func(r *protocol.AgentReport) { r.Host.Hostname = "  " }},
		{name: "no disks", mutate: func(r *protocol.AgentReport) { r.Disks = nil }},
		{name: "nonpositive collected time", mutate: func(r *protocol.AgentReport) { r.CollectedAtUnix = 0 }},
		{name: "negative cpu", mutate: func(r *protocol.AgentReport) { r.CPU.UsagePercent = -1 }},
		{name: "cpu over one hundred", mutate: func(r *protocol.AgentReport) { r.CPU.UsagePercent = 101 }},
		{name: "memory used over total", mutate: func(r *protocol.AgentReport) { r.Memory.UsedBytes = r.Memory.TotalBytes + 1 }},
		{name: "swap used over total", mutate: func(r *protocol.AgentReport) { r.Memory.SwapUsedBytes = r.Memory.SwapTotalBytes + 1 }},
		{name: "blank mountpoint", mutate: func(r *protocol.AgentReport) { r.Disks[0].Mountpoint = " " }},
		{name: "disk used over total", mutate: func(r *protocol.AgentReport) { r.Disks[0].UsedBytes = r.Disks[0].TotalBytes + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := validReport()
			test.mutate(&report)
			body, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if rec := serve(body); rec.Code != http.StatusBadRequest {
				t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestIngestCapturesHostOnlyRemoteAddress(t *testing.T) {
	router, _, store, _, server, token := newLiveRouter(t)
	body, _ := json.Marshal(validReport())
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "agent-host"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	snapshot, _ := store.Get(server.ID)
	if snapshot.SourceIP != "agent-host" {
		t.Fatalf("source IP=%q", snapshot.SourceIP)
	}
}

func newLiveRouter(t *testing.T) (http.Handler, *servers.Service, *live.Store, *live.Hub, servers.Server, string) {
	t.Helper()
	conn := newLiveDB(t)
	serverService := servers.NewService(conn)
	server, token, err := serverService.Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	store := live.NewStore()
	hub := live.NewHub()
	liveHandler := live.NewHandler(serverService, store, hub)
	return httpapi.NewRouter(httpapi.Dependencies{Servers: serverService, Live: liveHandler}), serverService, store, hub, server, token
}

func newLiveDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

func validReport() protocol.AgentReport {
	return protocol.AgentReport{
		CollectedAtUnix: 1,
		AgentVersion:    "1.2.3",
		Host:            protocol.HostInfo{Hostname: "probe-host"},
		CPU:             protocol.CPUStats{UsagePercent: 12.5, Load1: 1, Load5: 2, Load15: 3},
		Memory:          protocol.MemoryStats{TotalBytes: 100, UsedBytes: 50, SwapTotalBytes: 20, SwapUsedBytes: 10},
		Disks:           []protocol.DiskStats{{Mountpoint: "/", TotalBytes: 1000, UsedBytes: 400}},
	}
}

func boolPtr(value bool) *bool { return &value }
