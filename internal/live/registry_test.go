package live_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

func TestRenameReconcilesStoredSnapshotBeforeOfflineEvent(t *testing.T) {
	serverService := servers.NewService(newLiveDB(t))
	store := live.NewStore()
	hub := live.NewHub()
	coordinator := live.NewCoordinator(serverService, store, hub)
	serverService.SetRegistryObserver(coordinator)
	server, _, err := serverService.Create(context.Background(), "old-name")
	if err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Unix(1, 0)
	store.Upsert(server, protocol.AgentReport{}, receivedAt)
	events, cancel := hub.Subscribe()
	defer cancel()
	name := "new-name"

	if _, err := serverService.Update(context.Background(), server.ID, &name, nil); err != nil {
		t.Fatal(err)
	}
	coordinator.Sweep(receivedAt.Add(time.Second))

	event := <-events
	if event.Type != "snapshot.offline" || event.Snapshot.ServerName != name {
		t.Fatalf("event=%+v", event)
	}
	stored, _ := store.Get(server.ID)
	if stored.ServerName != name {
		t.Fatalf("stored name=%q", stored.ServerName)
	}
}

func TestDeleteRemovesSnapshotBeforeSweepAndStatusRead(t *testing.T) {
	serverService := servers.NewService(newLiveDB(t))
	store := live.NewStore()
	hub := live.NewHub()
	coordinator := live.NewCoordinator(serverService, store, hub)
	serverService.SetRegistryObserver(coordinator)
	handler := live.NewHandler(serverService, store, hub, live.WithCoordinator(coordinator))
	server, _, err := serverService.Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Unix(1, 0)
	store.Upsert(server, protocol.AgentReport{}, receivedAt)
	events, cancel := hub.Subscribe()
	defer cancel()

	if err := serverService.Delete(context.Background(), server.ID); err != nil {
		t.Fatal(err)
	}
	coordinator.Sweep(receivedAt.Add(time.Second))

	if _, ok := store.Get(server.ID); ok {
		t.Fatal("deleted server snapshot remains in store")
	}
	select {
	case event := <-events:
		t.Fatalf("ghost event=%+v", event)
	default:
	}
	rec := httptest.NewRecorder()
	handler.ListStatus(rec, httptest.NewRequest(http.MethodGet, "/api/servers/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status code=%d body=%q", rec.Code, rec.Body.String())
	}
	var statuses []live.Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&statuses); err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 0 {
		t.Fatalf("statuses=%+v", statuses)
	}
}

type gatedReader struct {
	reader  *bytes.Reader
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *gatedReader) Read(destination []byte) (int, error) {
	r.once.Do(func() {
		close(r.started)
		<-r.release
	})
	return r.reader.Read(destination)
}

func TestDeleteWinningDuringInFlightIngestCannotRecreateSnapshot(t *testing.T) {
	serverService := servers.NewService(newLiveDB(t))
	store := live.NewStore()
	hub := live.NewHub()
	coordinator := live.NewCoordinator(serverService, store, hub)
	serverService.SetRegistryObserver(coordinator)
	handler := live.NewHandler(serverService, store, hub, live.WithCoordinator(coordinator))
	server, token, err := serverService.Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(validReport())
	reader := &gatedReader{reader: bytes.NewReader(body), started: make(chan struct{}), release: make(chan struct{})}
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/report", reader)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	events, cancel := hub.Subscribe()
	defer cancel()
	done := make(chan struct{})
	go func() {
		handler.Ingest(rec, req)
		close(done)
	}()

	<-reader.started
	if err := serverService.Delete(context.Background(), server.ID); err != nil {
		t.Fatal(err)
	}
	close(reader.release)
	<-done

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
	if _, ok := store.Get(server.ID); ok {
		t.Fatal("in-flight ingest recreated deleted snapshot")
	}
	select {
	case event := <-events:
		t.Fatalf("ghost event=%+v", event)
	default:
	}
	listed, err := serverService.List(context.Background())
	if err != nil || !reflect.DeepEqual(listed, []servers.Server{}) {
		t.Fatalf("servers=%+v err=%v", listed, err)
	}
}
