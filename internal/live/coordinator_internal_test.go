package live

import (
	"context"
	"sync"
	"testing"
	"time"

	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

type barrierRegistry struct {
	versionOneEntered chan struct{}
	releaseVersionOne chan struct{}
	mu                sync.Mutex
	versions          []string
}

func (r *barrierRegistry) AuthenticateToken(context.Context, string) (servers.Server, error) {
	return servers.Server{ID: 7, Name: "home-lab", Enabled: true}, nil
}

func (r *barrierRegistry) UpdateAgentVersion(_ context.Context, _ int64, version string) error {
	if version == "v1" {
		close(r.versionOneEntered)
		<-r.releaseVersionOne
	}
	r.mu.Lock()
	r.versions = append(r.versions, version)
	r.mu.Unlock()
	return nil
}

type recordingPublisher struct {
	events chan Event
}

func (p *recordingPublisher) Publish(event Event) {
	p.events <- cloneEvent(event)
}

func TestCoordinatorSerializesConcurrentReportsThroughPublication(t *testing.T) {
	registry := &barrierRegistry{versionOneEntered: make(chan struct{}), releaseVersionOne: make(chan struct{})}
	store := NewStore()
	publisher := &recordingPublisher{events: make(chan Event, 2)}
	coordinator := newCoordinator(registry, store, publisher)
	attempted := make(chan string, 2)
	coordinator.beforeAccept = func(token string) { attempted <- token }
	done := make(chan error, 2)

	go func() {
		_, err := coordinator.Accept(context.Background(), "first", protocol.AgentReport{AgentVersion: "v1"}, time.Unix(1, 0), "192.0.2.1")
		done <- err
	}()
	if token := <-attempted; token != "first" {
		t.Fatalf("first attempt token=%q", token)
	}
	<-registry.versionOneEntered
	go func() {
		_, err := coordinator.Accept(context.Background(), "second", protocol.AgentReport{AgentVersion: "v2"}, time.Unix(2, 0), "192.0.2.2")
		done <- err
	}()
	if token := <-attempted; token != "second" {
		t.Fatalf("second attempt token=%q", token)
	}
	close(registry.releaseVersionOne)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	firstEvent := <-publisher.events
	secondEvent := <-publisher.events
	if firstEvent.Snapshot.Report.AgentVersion != "v1" || secondEvent.Snapshot.Report.AgentVersion != "v2" {
		t.Fatalf("events=%+v then %+v", firstEvent, secondEvent)
	}
	registry.mu.Lock()
	versions := append([]string(nil), registry.versions...)
	registry.mu.Unlock()
	if len(versions) != 2 || versions[0] != "v1" || versions[1] != "v2" {
		t.Fatalf("versions=%v", versions)
	}
	stored, _ := store.Get(7)
	if stored.Report.AgentVersion != "v2" || !stored.LastReceivedAt.Equal(time.Unix(2, 0)) {
		t.Fatalf("stored=%+v", stored)
	}
}

type offlineBarrierPublisher struct {
	offlineEntered chan struct{}
	releaseOffline chan struct{}
	events         chan Event
}

func (p *offlineBarrierPublisher) Publish(event Event) {
	if event.Type == "snapshot.offline" {
		close(p.offlineEntered)
		<-p.releaseOffline
	}
	p.events <- cloneEvent(event)
}

func TestCoordinatorPreventsDelayedOfflineAfterNewerOnlineEvent(t *testing.T) {
	registry := &barrierRegistry{versionOneEntered: make(chan struct{}), releaseVersionOne: make(chan struct{})}
	store := NewStore()
	store.Upsert(servers.Server{ID: 7, Name: "home-lab"}, protocol.AgentReport{AgentVersion: "old"}, time.Unix(1, 0))
	publisher := &offlineBarrierPublisher{
		offlineEntered: make(chan struct{}),
		releaseOffline: make(chan struct{}),
		events:         make(chan Event, 2),
	}
	coordinator := newCoordinator(registry, store, publisher)
	attempted := make(chan string, 1)
	coordinator.beforeAccept = func(token string) { attempted <- token }
	sweepDone := make(chan struct{})
	go func() {
		coordinator.Sweep(time.Unix(2, 0))
		close(sweepDone)
	}()
	<-publisher.offlineEntered
	acceptDone := make(chan error, 1)
	go func() {
		_, err := coordinator.Accept(context.Background(), "new", protocol.AgentReport{AgentVersion: "new"}, time.Unix(3, 0), "192.0.2.3")
		acceptDone <- err
	}()
	if token := <-attempted; token != "new" {
		t.Fatalf("attempt token=%q", token)
	}
	close(publisher.releaseOffline)
	<-sweepDone
	if err := <-acceptDone; err != nil {
		t.Fatal(err)
	}

	firstEvent := <-publisher.events
	secondEvent := <-publisher.events
	if firstEvent.Type != "snapshot.offline" || secondEvent.Type != "snapshot.updated" || secondEvent.Snapshot.Report.AgentVersion != "new" {
		t.Fatalf("events=%+v then %+v", firstEvent, secondEvent)
	}
	stored, _ := store.Get(7)
	if !stored.Online || stored.Report.AgentVersion != "new" {
		t.Fatalf("stored=%+v", stored)
	}
}
