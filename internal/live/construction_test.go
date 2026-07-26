package live_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/servers"
)

func TestHandlerAndSweeperConstructorsRequireCoordinator(t *testing.T) {
	assertPanics(t, func() { live.NewHandler(nil) })
	assertPanics(t, func() { live.NewSweeper(nil) })
}

func TestExplicitAssemblySharesCoordinatorAcrossHandlerSweeperAndRegistry(t *testing.T) {
	serverService := servers.NewService(newLiveDB(t))
	store := live.NewStore()
	hub := live.NewHub()
	coordinator := live.NewCoordinator(serverService, store, hub)
	if err := serverService.AttachRegistryObserver(coordinator); err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC)
	handler := live.NewHandler(coordinator, live.WithHandlerClock(func() time.Time { return receivedAt }))
	ticker := &fakeTicker{ch: make(chan time.Time, 1)}
	sweeper := live.NewSweeper(coordinator,
		live.WithSweeperClock(func() time.Time { return receivedAt.Add(31 * time.Second) }),
		live.WithSweeperTicker(func(time.Duration) live.Ticker { return ticker }),
	)
	server, token, err := serverService.Create(context.Background(), "old-name")
	if err != nil {
		t.Fatal(err)
	}
	events, cancelSubscription := hub.Subscribe()
	defer cancelSubscription()
	body, _ := json.Marshal(validReport())
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.Ingest(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	updatedEvent := <-events
	if updatedEvent.Type != "snapshot.updated" || updatedEvent.Snapshot.ServerID != server.ID {
		t.Fatalf("updated event=%+v", updatedEvent)
	}
	name := "new-name"
	if _, err := serverService.Update(context.Background(), server.ID, &name, nil); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sweeper.Run(ctx)
		close(done)
	}()
	ticker.ch <- receivedAt.Add(31 * time.Second)
	offlineEvent := <-events
	if offlineEvent.Type != "snapshot.offline" || offlineEvent.Snapshot.ServerName != name {
		t.Fatalf("offline event=%+v", offlineEvent)
	}
	cancel()
	<-done
}

func assertPanics(t *testing.T, operation func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("operation did not panic")
		}
	}()
	operation()
}

func coordinatorForTest(t *testing.T, store *live.Store, hub *live.Hub) *live.Coordinator {
	t.Helper()
	return live.NewCoordinator(servers.NewService(newLiveDB(t)), store, hub)
}
