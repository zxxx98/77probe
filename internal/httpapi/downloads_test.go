package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"probe.local/monitor/internal/httpapi"
)

func TestAgentDownloadHandlerServesOnlyAllowlistedBinariesAsAttachments(t *testing.T) {
	files := fstest.MapFS{
		"tinyprobe-agent-linux-amd64": {Data: []byte("amd64-binary")},
		"tinyprobe-agent-linux-arm64": {Data: []byte("arm64-binary")},
	}

	for _, test := range []struct {
		name string
		body string
	}{
		{name: "tinyprobe-agent-linux-amd64", body: "amd64-binary"},
		{name: "tinyprobe-agent-linux-arm64", body: "arm64-binary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/downloads/"+test.name, nil)
			rec := httptest.NewRecorder()

			httpapi.AgentDownloadHandler(files).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
			}
			if rec.Body.String() != test.body {
				t.Fatalf("body=%q, want %q", rec.Body.String(), test.body)
			}
			wantDisposition := `attachment; filename="` + test.name + `"`
			if got := rec.Header().Get("Content-Disposition"); got != wantDisposition {
				t.Fatalf("Content-Disposition=%q, want %q", got, wantDisposition)
			}
		})
	}
}

func TestAgentDownloadHandlerRejectsUnknownNamesWithoutListing(t *testing.T) {
	files := fstest.MapFS{
		"tinyprobe-agent-linux-amd64": {Data: []byte("amd64-binary")},
	}
	for _, path := range []string{
		"/downloads/",
		"/downloads/not-an-agent",
		"/downloads/tinyprobe-agent-linux-amd64/extra",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		httpapi.AgentDownloadHandler(files).ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("path=%q status=%d body=%q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestAgentDownloadRoutesArePublicAndMissingFilesAreSafe404(t *testing.T) {
	files := fstest.MapFS{
		"tinyprobe-agent-linux-amd64": {Data: []byte("amd64-binary")},
	}
	router := httpapi.NewRouter(httpapi.Dependencies{AgentFiles: files})

	public := httptest.NewRecorder()
	router.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/downloads/tinyprobe-agent-linux-amd64", nil))
	if public.Code != http.StatusOK || public.Body.String() != "amd64-binary" {
		t.Fatalf("public download: status=%d body=%q", public.Code, public.Body.String())
	}

	missingRouter := httpapi.NewRouter(httpapi.Dependencies{})
	missing := httptest.NewRecorder()
	missingRouter.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/downloads/tinyprobe-agent-linux-arm64", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing binary: status=%d body=%q", missing.Code, missing.Body.String())
	}
}
