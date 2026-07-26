package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"probe.local/monitor/internal/httpapi"
)

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
