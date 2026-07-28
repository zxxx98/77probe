package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/history"
	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

func TestRuntimeWiresLiveIngestionToHistoryAndStopsBackground(t *testing.T) {
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntime(conn, nil)
	if err != nil {
		t.Fatal(err)
	}
	server, token, err := runtime.servers.Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	report := protocol.AgentReport{
		CollectedAtUnix: 1,
		Host:            protocol.HostInfo{Hostname: "probe-host"},
		Disks:           []protocol.DiskStats{{Mountpoint: "/"}},
	}
	body, _ := json.Marshal(report)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	runtime.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	if snapshot, ok := runtime.store.Get(server.ID); !ok || !snapshot.Online {
		t.Fatalf("snapshot=%+v ok=%v", snapshot, ok)
	}
	snapshot, _ := runtime.store.Get(server.ID)
	minute := snapshot.LastReceivedAt.UTC().Truncate(time.Minute)
	if err := runtime.historyAggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	records, err := runtime.historyStore.Query(context.Background(), server.ID, minute.Unix(), minute.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ServerID != server.ID || records[0].MinuteUnix != minute.Unix() {
		t.Fatalf("history records=%+v", records)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := runtime.startBackground(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime background work did not stop after cancellation")
	}
}

func TestRuntimeBackgroundDoneWaitsForEveryRunner(t *testing.T) {
	first := newBlockingBackgroundRunner()
	second := newBlockingBackgroundRunner()
	runtime := &runtime{background: []backgroundRunner{first, second}}
	ctx, cancel := context.WithCancel(context.Background())
	done := runtime.startBackground(ctx)
	first.waitStarted(t)
	second.waitStarted(t)

	cancel()
	first.waitCanceled(t)
	second.waitCanceled(t)
	select {
	case <-done:
		t.Fatal("background done closed before runners returned")
	case <-time.After(20 * time.Millisecond):
	}

	close(first.release)
	select {
	case <-done:
		t.Fatal("background done closed while second runner was blocked")
	case <-time.After(20 * time.Millisecond):
	}
	close(second.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background done did not close after every runner returned")
	}
}

func TestApplicationRunBackgroundStartsOnlyOnce(t *testing.T) {
	runner := newLifecycleRunner()
	application, _ := newLifecycleApplication(t, runner)
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := application.RunBackground(ctx)
	runner.waitStarted(t)
	secondDone := application.RunBackground(context.Background())
	if firstDone != secondDone {
		t.Fatal("repeated RunBackground returned different completion channels")
	}
	if got := runner.starts.Load(); got != 1 {
		t.Fatalf("runner starts = %d, want 1", got)
	}

	cancel()
	runner.waitCanceled(t)
	close(runner.release)
	waitForDone(t, firstDone)
	if thirdDone := application.RunBackground(context.Background()); thirdDone != firstDone {
		t.Fatal("RunBackground restarted after the first run completed")
	}
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationCloseCancelsWaitsThenClosesDatabaseAndIsConcurrentSafe(t *testing.T) {
	runner := newLifecycleRunner()
	application, conn := newLifecycleApplication(t, runner)
	application.RunBackground(context.Background())
	runner.waitStarted(t)

	closeResults := make(chan error, 2)
	go func() { closeResults <- application.Close() }()
	go func() { closeResults <- application.Close() }()
	runner.waitCanceled(t)
	select {
	case err := <-closeResults:
		t.Fatalf("Close returned before background runner stopped: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := conn.Ping(); err != nil {
		t.Fatalf("database closed before background runner stopped: %v", err)
	}

	close(runner.release)
	for range 2 {
		if err := <-closeResults; err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.Ping(); err == nil {
		t.Fatal("database remained open after Close")
	}
	if err := application.Close(); err != nil {
		t.Fatalf("repeated Close() error = %v", err)
	}
}

func TestApplicationCloseBeforeStartPreventsBackgroundStart(t *testing.T) {
	runner := newLifecycleRunner()
	application, conn := newLifecycleApplication(t, runner)
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}
	done := application.RunBackground(context.Background())
	select {
	case <-done:
	default:
		t.Fatal("RunBackground after Close returned an open completion channel")
	}
	if got := runner.starts.Load(); got != 0 {
		t.Fatalf("runner starts = %d, want 0", got)
	}
	if err := conn.Ping(); err == nil {
		t.Fatal("database remained open after Close-before-start")
	}
}

func TestShutdownServerDrainsHistoryAcceptanceBeforeFinalFlushAndDatabaseClose(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "shutdown-order.db")
	conn, err := monitorDB.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	server, _, err := servers.NewService(conn).Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	historyStore := history.NewStore(conn)
	aggregator := history.NewAggregator(historyStore)
	receivedAt := time.Date(2026, time.July, 28, 1, 2, 59, 0, time.UTC)
	accepted := make(chan struct{})
	runner := newFinalFlushRunner(aggregator, receivedAt.Add(time.Minute), accepted)
	application := &Application{
		conn: conn,
		runtime: &runtime{
			background: []backgroundRunner{runner},
		},
	}
	application.RunBackground(context.Background())
	runner.waitStarted(t)

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
		aggregator.Accept(server.ID, protocol.AgentReport{CPU: protocol.CPUStats{UsagePercent: 42}}, receivedAt)
		close(accepted)
		w.WriteHeader(http.StatusNoContent)
	})
	serverBase, cancelServerBase := context.WithCancel(context.Background())
	httpServer := &http.Server{
		Handler: handler,
		BaseContext: func(net.Listener) context.Context {
			return serverBase
		},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- httpServer.Serve(listener) }()
	responseDone := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			response.Body.Close()
		}
		responseDone <- err
	}()
	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
	defer cancelShutdown()
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- shutdownServer(shutdownCtx, httpServer, application, cancelServerBase)
	}()
	if err := <-serveDone; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve() error = %v, want %v", err, http.ErrServerClosed)
	}
	select {
	case <-runner.flushed:
		t.Fatal("final history flush ran while HTTP handler was still in flight")
	default:
	}
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before HTTP handler drained: %v", err)
	default:
	}

	close(releaseHandler)
	if err := <-responseDone; err != nil {
		t.Fatal(err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}
	if !runner.acceptedBeforeFlush.Load() {
		t.Fatal("final history flush began before the handler accepted its report")
	}
	if runner.err != nil {
		t.Fatalf("final history flush error = %v", runner.err)
	}
	if err := conn.Ping(); err == nil {
		t.Fatal("database remained open after shutdown")
	}

	reopened, err := monitorDB.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	minuteUnix := receivedAt.UTC().Truncate(time.Minute).Unix()
	records, err := history.NewStore(reopened).Query(context.Background(), server.ID, minuteUnix, minuteUnix)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Payload.CPUUsage.Average != 42 {
		t.Fatalf("persisted records = %+v", records)
	}
}

type blockingBackgroundRunner struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

type lifecycleRunner struct {
	starts     atomic.Int32
	startOnce  sync.Once
	cancelOnce sync.Once
	started    chan struct{}
	canceled   chan struct{}
	release    chan struct{}
}

type finalFlushRunner struct {
	aggregator          *history.Aggregator
	boundary            time.Time
	accepted            <-chan struct{}
	started             chan struct{}
	flushed             chan struct{}
	acceptedBeforeFlush atomic.Bool
	err                 error
}

func newFinalFlushRunner(aggregator *history.Aggregator, boundary time.Time, accepted <-chan struct{}) *finalFlushRunner {
	return &finalFlushRunner{
		aggregator: aggregator,
		boundary:   boundary,
		accepted:   accepted,
		started:    make(chan struct{}),
		flushed:    make(chan struct{}),
	}
}

func (r *finalFlushRunner) Run(ctx context.Context) {
	close(r.started)
	<-ctx.Done()
	select {
	case <-r.accepted:
		r.acceptedBeforeFlush.Store(true)
	default:
	}
	r.err = r.aggregator.FlushBefore(context.Background(), r.boundary)
	close(r.flushed)
}

func (r *finalFlushRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("final flush runner did not start")
	}
}

func newLifecycleRunner() *lifecycleRunner {
	return &lifecycleRunner{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (r *lifecycleRunner) Run(ctx context.Context) {
	r.starts.Add(1)
	r.startOnce.Do(func() { close(r.started) })
	<-ctx.Done()
	r.cancelOnce.Do(func() { close(r.canceled) })
	<-r.release
}

func (r *lifecycleRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("lifecycle runner did not start")
	}
}

func (r *lifecycleRunner) waitCanceled(t *testing.T) {
	t.Helper()
	select {
	case <-r.canceled:
	case <-time.After(time.Second):
		t.Fatal("lifecycle runner did not observe cancellation")
	}
}

func newLifecycleApplication(t *testing.T, runners ...backgroundRunner) (*Application, *sql.DB) {
	t.Helper()
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	return &Application{conn: conn, runtime: &runtime{background: runners}}, conn
}

func waitForDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completion channel did not close")
	}
}

func newBlockingBackgroundRunner() *blockingBackgroundRunner {
	return &blockingBackgroundRunner{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (r *blockingBackgroundRunner) Run(ctx context.Context) {
	close(r.started)
	<-ctx.Done()
	close(r.canceled)
	<-r.release
}

func (r *blockingBackgroundRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("background runner did not start")
	}
}

func (r *blockingBackgroundRunner) waitCanceled(t *testing.T) {
	t.Helper()
	select {
	case <-r.canceled:
	case <-time.After(time.Second):
		t.Fatal("background runner did not observe cancellation")
	}
}
