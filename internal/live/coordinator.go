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

type HistoryAccepter interface {
	Accept(serverID int64, report protocol.AgentReport, receivedAt time.Time)
}

type HistoryBucketRemover interface {
	RemoveServer(serverID int64)
}

type CoordinatorOption func(*Coordinator)

type Coordinator struct {
	mu            sync.Mutex
	serverService *servers.Service
	registry      serverRegistry
	store         *Store
	hub           *Hub
	publisher     eventPublisher
	history       HistoryAccepter
	historyRemove HistoryBucketRemover
	observers     []eventPublisher
	beforeAccept  func(string)
}

func NewCoordinator(registry *servers.Service, store *Store, hub *Hub, options ...CoordinatorOption) *Coordinator {
	if registry == nil || store == nil || hub == nil {
		panic("live coordinator requires server service, store, and hub")
	}
	coordinator := newCoordinator(registry, store, hub, options...)
	coordinator.serverService = registry
	coordinator.hub = hub
	return coordinator
}

func newCoordinator(registry serverRegistry, store *Store, publisher eventPublisher, options ...CoordinatorOption) *Coordinator {
	coordinator := &Coordinator{registry: registry, store: store, publisher: publisher}
	for _, option := range options {
		option(coordinator)
	}
	return coordinator
}

func WithHistory(accepter HistoryAccepter, remover HistoryBucketRemover) CoordinatorOption {
	return func(coordinator *Coordinator) {
		coordinator.history = accepter
		coordinator.historyRemove = remover
	}
}

func WithObserver(observer eventPublisher) CoordinatorOption {
	return func(coordinator *Coordinator) {
		if observer != nil {
			coordinator.observers = append(coordinator.observers, observer)
		}
	}
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
	c.publish(Event{Type: "snapshot.updated", Snapshot: snapshot})
	if c.history != nil {
		c.history.Accept(server.ID, report, receivedAt)
	}
	return snapshot, nil
}

func (c *Coordinator) Sweep(cutoff time.Time) []Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := c.store.MarkOffline(cutoff)
	for _, snapshot := range changed {
		c.publish(Event{Type: "snapshot.offline", Snapshot: snapshot})
	}
	return changed
}

func (c *Coordinator) publish(event Event) {
	c.publisher.Publish(event)
	for _, observer := range c.observers {
		observer.Publish(event)
	}
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
	if c.historyRemove != nil {
		c.historyRemove.RemoveServer(serverID)
	}
	return nil
}
