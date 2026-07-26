package live_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"probe.local/monitor/internal/auth"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/servers"
)

func TestSSEStreamsFullEventWithExactHeaders(t *testing.T) {
	router, cookie, hub, _, started := newSSERouter(t)
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/live", nil)
	req.AddCookie(cookie)
	responseCh := make(chan *http.Response, 1)
	errorCh := make(chan error, 1)
	go func() {
		response, err := server.Client().Do(req)
		if err != nil {
			errorCh <- err
			return
		}
		responseCh <- response
	}()
	<-started
	want := live.Event{Type: "snapshot.updated", Snapshot: live.Snapshot{ServerID: 7, ServerName: "home-lab", Online: true}}
	hub.Publish(want)
	var response *http.Response
	select {
	case response = <-responseCh:
	case err := <-errorCh:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("SSE response did not flush")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-cache" || response.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("status=%d headers=%v", response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	eventLine, _ := reader.ReadString('\n')
	dataLine, _ := reader.ReadString('\n')
	blankLine, _ := reader.ReadString('\n')
	if eventLine != "event: snapshot.updated\n" || blankLine != "\n" || !strings.HasPrefix(dataLine, "data: ") {
		t.Fatalf("stream=%q%q%q", eventLine, dataLine, blankLine)
	}
	var got live.Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(dataLine, "data: "))), &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != want.Type || got.Snapshot.ServerID != want.Snapshot.ServerID || got.Snapshot.ServerName != want.Snapshot.ServerName || !got.Snapshot.Online {
		t.Fatalf("event=%+v", got)
	}
}

func TestSSEHeartbeatUsesFifteenSecondTickerAndFlushes(t *testing.T) {
	router, cookie, _, ticker, started := newSSERouter(t)
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/live", nil)
	req.AddCookie(cookie)
	responseCh := make(chan *http.Response, 1)
	go func() {
		response, _ := server.Client().Do(req)
		responseCh <- response
	}()
	interval := <-started
	if interval != 15*time.Second {
		t.Fatalf("heartbeat interval=%s", interval)
	}
	ticker.ch <- time.Now()
	response := <-responseCh
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	line, _ := reader.ReadString('\n')
	blank, _ := reader.ReadString('\n')
	if line != ": keepalive\n" || blank != "\n" {
		t.Fatalf("heartbeat=%q%q", line, blank)
	}
}

func TestSSEExitsPromptlyOnRequestCancellation(t *testing.T) {
	hub := live.NewHub()
	ticker := &fakeTicker{ch: make(chan time.Time)}
	started := make(chan time.Duration, 1)
	handler := live.NewHandler(nil, live.NewStore(), hub, live.WithHeartbeatTicker(func(interval time.Duration) live.Ticker {
		started <- interval
		return ticker
	}))
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/live", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.SSE(rec, req)
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not exit after cancellation")
	}
	if !ticker.stopped.Load() {
		t.Fatal("heartbeat ticker was not stopped")
	}
}

func newSSERouter(t *testing.T) (http.Handler, *http.Cookie, *live.Hub, *fakeTicker, <-chan time.Duration) {
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
	hub := live.NewHub()
	ticker := &fakeTicker{ch: make(chan time.Time, 1)}
	started := make(chan time.Duration, 1)
	handler := live.NewHandler(serverService, live.NewStore(), hub, live.WithHeartbeatTicker(func(interval time.Duration) live.Ticker {
		started <- interval
		return ticker
	}))
	router := httpapi.NewRouter(httpapi.Dependencies{Auth: authService, Servers: serverService, Live: handler})
	return router, &http.Cookie{Name: "tinyprobe_session", Value: session}, hub, ticker, started
}
