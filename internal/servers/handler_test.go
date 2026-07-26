package servers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"probe.local/monitor/internal/auth"
	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/servers"
)

func TestRegistryRequiresAdministratorSession(t *testing.T) {
	router, _ := newRegistryRouter(t)
	rec := serveRegistry(router, http.MethodGet, "/api/servers", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRegistryLifecycleReturnsTokenOnlyOnCreateAndRotate(t *testing.T) {
	router, session := newRegistryRouter(t)

	created := serveRegistry(router, http.MethodPost, "/api/servers", `{"name":"home-lab"}`, session)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: code=%d body=%q", created.Code, created.Body.String())
	}
	var createResponse struct {
		Server servers.Server `json:"server"`
		Token  string         `json:"token"`
	}
	if err := json.NewDecoder(created.Body).Decode(&createResponse); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(createResponse.Token, "tp_") {
		t.Fatalf("token=%q", createResponse.Token)
	}

	listed := serveRegistry(router, http.MethodGet, "/api/servers", "", session)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), createResponse.Token) || strings.Contains(listed.Body.String(), "token") {
		t.Fatalf("list: code=%d body=%q", listed.Code, listed.Body.String())
	}

	renamed := serveRegistry(router, http.MethodPatch, "/api/servers/1", `{"name":"office-lab","enabled":false}`, session)
	if renamed.Code != http.StatusOK || !strings.Contains(renamed.Body.String(), `"name":"office-lab"`) || !strings.Contains(renamed.Body.String(), `"enabled":false`) {
		t.Fatalf("patch: code=%d body=%q", renamed.Code, renamed.Body.String())
	}

	rotated := serveRegistry(router, http.MethodPost, "/api/servers/1/token", "", session)
	if rotated.Code != http.StatusCreated {
		t.Fatalf("rotate: code=%d body=%q", rotated.Code, rotated.Body.String())
	}
	var rotateResponse struct {
		Server servers.Server `json:"server"`
		Token  string         `json:"token"`
	}
	if err := json.NewDecoder(rotated.Body).Decode(&rotateResponse); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rotateResponse.Token, "tp_") || rotateResponse.Token == createResponse.Token {
		t.Fatalf("rotated token=%q", rotateResponse.Token)
	}

	deleted := serveRegistry(router, http.MethodDelete, "/api/servers/1", "", session)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete: code=%d body=%q", deleted.Code, deleted.Body.String())
	}
}

func TestCreateServerMapsLimitToConflict(t *testing.T) {
	router, session := newRegistryRouter(t)
	for range 10 {
		rec := serveRegistry(router, http.MethodPost, "/api/servers", `{"name":"server"}`, session)
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed: code=%d body=%q", rec.Code, rec.Body.String())
		}
	}
	rec := serveRegistry(router, http.MethodPost, "/api/servers", `{"name":"eleven"}`, session)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestUpdateServerAllowsPartialFieldsAndRejectsEmptyPatch(t *testing.T) {
	router, session := newRegistryRouter(t)
	created := serveRegistry(router, http.MethodPost, "/api/servers", `{"name":"home-lab"}`, session)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: code=%d body=%q", created.Code, created.Body.String())
	}

	renamed := serveRegistry(router, http.MethodPatch, "/api/servers/1", `{"name":"office-lab"}`, session)
	if renamed.Code != http.StatusOK || !strings.Contains(renamed.Body.String(), `"name":"office-lab"`) || !strings.Contains(renamed.Body.String(), `"enabled":true`) {
		t.Fatalf("rename: code=%d body=%q", renamed.Code, renamed.Body.String())
	}

	disabled := serveRegistry(router, http.MethodPatch, "/api/servers/1", `{"enabled":false}`, session)
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"name":"office-lab"`) || !strings.Contains(disabled.Body.String(), `"enabled":false`) {
		t.Fatalf("disable: code=%d body=%q", disabled.Code, disabled.Body.String())
	}

	empty := serveRegistry(router, http.MethodPatch, "/api/servers/1", `{}`, session)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty patch: code=%d body=%q", empty.Code, empty.Body.String())
	}
}

func newRegistryRouter(t *testing.T) (http.Handler, *http.Cookie) {
	t.Helper()
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
	token, _, err := authService.Login(ctx, "xiaodi", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.NewRouter(httpapi.Dependencies{Auth: authService, Servers: servers.NewService(conn)}), &http.Cookie{Name: "tinyprobe_session", Value: token}
}

func serveRegistry(handler http.Handler, method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
