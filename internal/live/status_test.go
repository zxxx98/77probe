package live_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"probe.local/monitor/internal/auth"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

func TestStatusRoutesRequireAdministratorSession(t *testing.T) {
	router, _, _, _ := newAuthenticatedLiveRouter(t)
	for _, target := range []string{"/api/servers/status", "/api/servers/1/status", "/api/live"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s code=%d body=%q", target, rec.Code, rec.Body.String())
		}
	}
}

func TestStatusListIncludesPlaceholdersAndSortsOfflineFirstByName(t *testing.T) {
	router, cookie, serverService, store := newAuthenticatedLiveRouter(t)
	ctx := context.Background()
	alpha, _, _ := serverService.Create(ctx, "alpha")
	beta, _, _ := serverService.Create(ctx, "beta")
	zulu, _, _ := serverService.Create(ctx, "zulu")
	now := time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC)
	store.Upsert(beta, validReport(), now)
	store.Upsert(zulu, protocol.AgentReport{CollectedAtUnix: 1}, now.Add(-31*time.Second))
	store.MarkOffline(now.Add(-30 * time.Second))

	req := httptest.NewRequest(http.MethodGet, "/api/servers/status", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	var got []live.Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ServerID != alpha.ID || got[1].ServerID != zulu.ID || got[2].ServerID != beta.ID {
		t.Fatalf("statuses=%+v", got)
	}
	if got[0].Online || !got[0].LastReceivedAt.IsZero() || got[0].ServerName != "alpha" {
		t.Fatalf("placeholder=%+v", got[0])
	}
	if got[1].Online || !got[2].Online {
		t.Fatalf("statuses=%+v", got)
	}
}

func TestStatusDetailReturnsPlaceholderAndUnknownOnlyIsNotFound(t *testing.T) {
	router, cookie, serverService, _ := newAuthenticatedLiveRouter(t)
	server, _, err := serverService.Create(context.Background(), "never-reported")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/servers/1/status", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	var got live.Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ServerID != server.ID || got.ServerName != server.Name || got.Online || !got.LastReceivedAt.IsZero() {
		t.Fatalf("snapshot=%+v", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/servers/999/status", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func newAuthenticatedLiveRouter(t *testing.T) (http.Handler, *http.Cookie, *servers.Service, *live.Store) {
	t.Helper()
	conn := newLiveDB(t)
	ctx := context.Background()
	authService := auth.NewService(conn)
	if err := authService.CreateAdmin(ctx, "xiaodi", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	session, _, err := authService.Login(ctx, "xiaodi", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	serverService := servers.NewService(conn)
	store := live.NewStore()
	hub := live.NewHub()
	coordinator := live.NewCoordinator(serverService, store, hub)
	if err := serverService.AttachRegistryObserver(coordinator); err != nil {
		t.Fatal(err)
	}
	handler := live.NewHandler(coordinator)
	router := httpapi.NewRouter(httpapi.Dependencies{Auth: authService, Servers: serverService, Live: handler})
	return router, &http.Cookie{Name: "tinyprobe_session", Value: session}, serverService, store
}
