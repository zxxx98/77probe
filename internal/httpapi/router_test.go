package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"probe.local/monitor/internal/auth"
	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/history"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/servers"
)

func TestHistoryRouteRequiresAdministratorSessionAndReachesHandler(t *testing.T) {
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx := context.Background()
	if err := monitorDB.ApplyMigrations(ctx, conn); err != nil {
		t.Fatal(err)
	}
	authService := auth.NewService(conn)
	if err := authService.CreateAdmin(ctx, "xiaodi", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	session, _, err := authService.Login(ctx, "xiaodi", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	serverService := servers.NewService(conn)
	server, _, err := serverService.Create(ctx, "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Date(2026, time.July, 28, 12, 34, 56, 0, time.UTC)
	store := &routerHistoryStore{}
	historyHandler := history.NewHandler(store, serverService, history.WithClock(func() time.Time { return fixedNow }))
	router := httpapi.NewRouter(httpapi.Dependencies{
		Auth:    authService,
		Servers: serverService,
		History: historyHandler,
	})
	target := "/api/servers/" + strconv.FormatInt(server.ID, 10) + "/history?range=1d"

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, target, nil))
	if unauthenticated.Code != http.StatusUnauthorized || unauthenticated.Body.String() != "{\"error\":\"unauthenticated\"}\n" {
		t.Fatalf("unauthenticated status/body = %d/%q", unauthenticated.Code, unauthenticated.Body.String())
	}
	if store.calls != 0 {
		t.Fatalf("unauthenticated request reached history store %d times", store.calls)
	}

	authenticatedRequest := httptest.NewRequest(http.MethodGet, target, nil)
	authenticatedRequest.AddCookie(&http.Cookie{Name: "tinyprobe_session", Value: session})
	authenticated := httptest.NewRecorder()
	router.ServeHTTP(authenticated, authenticatedRequest)
	if authenticated.Code != http.StatusOK {
		t.Fatalf("authenticated status/body = %d/%q", authenticated.Code, authenticated.Body.String())
	}
	if store.calls != 1 || store.serverID != server.ID {
		t.Fatalf("history store calls/server = %d/%d, want 1/%d", store.calls, store.serverID, server.ID)
	}
}

func TestUnknownAPIRouteReturnsJSONNotSPA(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	rec := httptest.NewRecorder()

	httpapi.NewRouter(httpapi.Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if body := rec.Body.String(); body != "{\"error\":\"not found\"}\n" {
		t.Fatalf("body = %q", body)
	}
	if strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatal("API 404 returned the SPA index")
	}
}

func TestUnknownPageRouteReturnsSPAIndex(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/servers/1", nil)
	rec := httptest.NewRecorder()

	httpapi.NewRouter(httpapi.Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, `<div id="root"></div>`) {
		t.Fatalf("body does not contain React root: %q", body)
	}
}

type routerHistoryStore struct {
	calls    int
	serverID int64
}

func (s *routerHistoryStore) Query(_ context.Context, serverID, _, _ int64) ([]history.MinuteRecord, error) {
	s.calls++
	s.serverID = serverID
	return []history.MinuteRecord{}, nil
}
