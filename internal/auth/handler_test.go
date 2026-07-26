package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"probe.local/monitor/internal/auth"
	"probe.local/monitor/internal/httpapi"
)

func TestSetupLoginMeAndLogout(t *testing.T) {
	t.Setenv("TINYPROBE_SECURE_COOKIES", "true")
	svc, _ := newTestService(t)
	router := httpapi.NewRouter(httpapi.Dependencies{Auth: svc})

	status := serve(router, http.MethodGet, "/api/setup/status", "", nil)
	if status.Code != http.StatusOK || status.Body.String() != "{\"setupRequired\":true}\n" {
		t.Fatalf("setup status: code=%d body=%q", status.Code, status.Body.String())
	}

	setup := serve(router, http.MethodPost, "/api/setup", `{"username":"xiaodi","password":"correct horse battery staple"}`, nil)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup: code=%d body=%q", setup.Code, setup.Body.String())
	}

	second := serve(router, http.MethodPost, "/api/setup", `{"username":"second","password":"another secure password"}`, nil)
	if second.Code != http.StatusConflict {
		t.Fatalf("second setup: code=%d body=%q", second.Code, second.Body.String())
	}

	wrong := serve(router, http.MethodPost, "/api/login", `{"username":"xiaodi","password":"wrong password"}`, nil)
	if wrong.Code != http.StatusUnauthorized || len(wrong.Result().Cookies()) != 0 {
		t.Fatalf("wrong login: code=%d cookieCount=%d body=%q", wrong.Code, len(wrong.Result().Cookies()), wrong.Body.String())
	}

	login := serve(router, http.MethodPost, "/api/login", `{"username":"xiaodi","password":"correct horse battery staple"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login: code=%d body=%q", login.Code, login.Body.String())
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookie count = %d, want 1", len(cookies))
	}
	session := cookies[0]
	if session.Name != "tinyprobe_session" || session.Value == "" || session.Path != "/" || session.MaxAge != 604800 || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie attributes: name=%q valuePresent=%v path=%q maxAge=%d httpOnly=%v secure=%v sameSite=%d", session.Name, session.Value != "", session.Path, session.MaxAge, session.HttpOnly, session.Secure, session.SameSite)
	}

	unauthenticated := serve(router, http.MethodGet, "/api/me", "", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated me: code=%d body=%q", unauthenticated.Code, unauthenticated.Body.String())
	}

	me := serve(router, http.MethodGet, "/api/me", "", session)
	if me.Code != http.StatusOK || me.Body.String() != "{\"id\":1,\"username\":\"xiaodi\"}\n" {
		t.Fatalf("me: code=%d body=%q", me.Code, me.Body.String())
	}

	logout := serve(router, http.MethodPost, "/api/logout", "", session)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout: code=%d body=%q", logout.Code, logout.Body.String())
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != "tinyprobe_session" || cleared[0].MaxAge >= 0 || cleared[0].Value != "" {
		t.Fatalf("cleared cookie count=%d", len(cleared))
	}
	if _, err := svc.Authenticate(context.Background(), session.Value); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("authenticate after HTTP logout error = %v", err)
	}
}

func TestSetupRejectsInvalidInput(t *testing.T) {
	svc, _ := newTestService(t)
	router := httpapi.NewRouter(httpapi.Dependencies{Auth: svc})

	rec := serve(router, http.MethodPost, "/api/setup", `{"username":"xiaodi","password":"short"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSetupRequiresJSONContentTypeAndSingleDocument(t *testing.T) {
	t.Run("text plain", func(t *testing.T) {
		svc, _ := newTestService(t)
		router := httpapi.NewRouter(httpapi.Dependencies{Auth: svc})
		req := httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(`{"username":"xiaodi","password":"correct horse battery staple"}`))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
		}
		required, err := svc.SetupRequired(context.Background())
		if err != nil || !required {
			t.Fatalf("required=%v err=%v", required, err)
		}
	})

	t.Run("trailing document", func(t *testing.T) {
		svc, _ := newTestService(t)
		router := httpapi.NewRouter(httpapi.Dependencies{Auth: svc})
		rec := serve(router, http.MethodPost, "/api/setup", `{"username":"xiaodi","password":"correct horse battery staple"}{}`, nil)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
		}
		required, err := svc.SetupRequired(context.Background())
		if err != nil || !required {
			t.Fatalf("required=%v err=%v", required, err)
		}
	})
}

func serve(handler http.Handler, method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
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
