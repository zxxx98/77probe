package live

import (
	"context"
	"sync"
	"time"

	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

type serverRegistry interface {
	AuthenticateToken(context.Context, string) (servers.Server, error)
	UpdateAgentVersion(context.Context, int64, string) error
}

type eventPublisher interface {
	Publish(Event)
}

type Coordinator struct {
	mu            sync.Mutex
	serverService *servers.Service
	registry      serverRegistry
	store         *Store
	hub           *Hub
	publisher     eventPublisher
	beforeAccept  func(string)
}

func NewCoordinator(registry *servers.Service, store *Store, hub *Hub) *Coordinator {
	if registry == nil || store == nil || hub == nil {
		panic("live coordinator requires server service, store, and hub")
	}
	coordinator := newCoordinator(registry, store, hub)
	coordinator.serverService = registry
	coordinator.hub = hub
	return coordinator
}

func newCoordinator(registry serverRegistry, store *Store, publisher eventPublisher) *Coordinator {
	return &Coordinator{registry: registry, store: store, publisher: publisher}
}

func (c *Coordinator) Accept(ctx context.Context, token string, report protocol.AgentReport, receivedAt time.Time, sourceIP string) (Snapshot, error) {
	if c.beforeAccept != nil {
		c.beforeAccept(token)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	server, err := c.registry.AuthenticateToken(ctx, token)
	if err != nil {
		return Snapshot{}, err
	}
	if err := c.registry.UpdateAgentVersion(ctx, server.ID, report.AgentVersion); err != nil {
		return Snapshot{}, err
	}
	snapshot := c.store.UpsertFrom(server, report, receivedAt, sourceIP)
	c.publisher.Publish(Event{Type: "snapshot.updated", Snapshot: snapshot})
	return snapshot, nil
}

func (c *Coordinator) Sweep(cutoff time.Time) []Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := c.store.MarkOffline(cutoff)
	for _, snapshot := range changed {
		c.publisher.Publish(Event{Type: "snapshot.offline", Snapshot: snapshot})
	}
	return changed
}

func (c *Coordinator) ReconcileUpdate(mutate func() (servers.Server, error)) (servers.Server, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	server, err := mutate()
	if err != nil {
		return servers.Server{}, err
	}
	c.store.rename(server.ID, server.Name)
	return server, nil
}

func (c *Coordinator) ReconcileDelete(serverID int64, mutate func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := mutate(); err != nil {
		return err
	}
	c.store.delete(serverID)
	return nil
}
