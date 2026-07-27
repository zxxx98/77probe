package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"probe.local/monitor/internal/app"
)

func TestApplicationStartsWithFreshDatabase(t *testing.T) {
	instance, err := app.New(app.Config{
		DatabasePath: filepath.Join(t.TempDir(), "monitor.db"),
		AgentFiles: fstest.MapFS{
			"tinyprobe-agent-linux-amd64": {Data: []byte("amd64")},
			"tinyprobe-agent-linux-arm64": {Data: []byte("arm64")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()

	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/api/health", body: `{"status":"ok"}` + "\n"},
		{path: "/downloads/tinyprobe-agent-linux-amd64", body: "amd64"},
		{path: "/downloads/tinyprobe-agent-linux-arm64", body: "arm64"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		instance.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%q", test.path, recorder.Code, recorder.Body.String())
		}
		if recorder.Body.String() != test.body {
			t.Fatalf("%s body=%q, want %q", test.path, recorder.Body.String(), test.body)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := instance.RunBackground(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("application background work did not stop")
	}
}
