package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"probe.local/monitor/internal/webui"
)

func TestHandlerServesIndexForClientRoutes(t *testing.T) {
	for _, target := range []string{"/", "/servers/1"} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()

			webui.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
				t.Fatalf("Content-Type = %q, want text/html", contentType)
			}
			if body := rec.Body.String(); !strings.Contains(body, `<div id="root"></div>`) {
				t.Fatalf("body does not contain React root: %q", body)
			}
		})
	}
}

func TestHandlerDoesNotUseIndexForNonGETRequests(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/servers/1", nil)
	rec := httptest.NewRecorder()

	webui.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
