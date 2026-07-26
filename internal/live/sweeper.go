package live

import (
	"context"
	"time"
)

const (
	sweepInterval = 5 * time.Second
	offlineAfter  = 30 * time.Second
)

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type SweeperOption func(*Sweeper)

type Sweeper struct {
	store     *Store
	hub       *Hub
	now       func() time.Time
	newTicker func(time.Duration) Ticker
}

func NewSweeper(store *Store, hub *Hub, options ...SweeperOption) *Sweeper {
	sweeper := &Sweeper{
		store: store,
		hub:   hub,
		now:   time.Now,
		newTicker: func(interval time.Duration) Ticker {
			return realTicker{Ticker: time.NewTicker(interval)}
		},
	}
	for _, option := range options {
		option(sweeper)
	}
	return sweeper
}

func WithSweeperClock(now func() time.Time) SweeperOption {
	return func(sweeper *Sweeper) { sweeper.now = now }
}

func WithSweeperTicker(factory func(time.Duration) Ticker) SweeperOption {
	return func(sweeper *Sweeper) { sweeper.newTicker = factory }
}

func (s *Sweeper) Run(ctx context.Context) {
	ticker := s.newTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			for _, snapshot := range s.store.MarkOffline(s.now().Add(-offlineAfter)) {
				s.hub.Publish(Event{Type: "snapshot.offline", Snapshot: snapshot})
			}
		}
	}
}

type realTicker struct {
	*time.Ticker
}

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }
