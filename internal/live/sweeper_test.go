package live_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

type fakeTicker struct {
	ch      chan time.Time
	stopped atomic.Bool
}

func (t *fakeTicker) C() <-chan time.Time { return t.ch }
func (t *fakeTicker) Stop()               { t.stopped.Store(true) }

func TestSweeperPublishesOneOfflineTransition(t *testing.T) {
	now := time.Date(2026, 7, 26, 4, 0, 31, 0, time.UTC)
	store := live.NewStore()
	store.Upsert(servers.Server{ID: 7, Name: "home-lab"}, protocol.AgentReport{}, now.Add(-31*time.Second))
	hub := live.NewHub()
	events, cancelSubscription := hub.Subscribe()
	defer cancelSubscription()
	ticker := &fakeTicker{ch: make(chan time.Time, 2)}
	var interval time.Duration
	sweeper := live.NewSweeper(store, hub,
		live.WithSweeperClock(func() time.Time { return now }),
		live.WithSweeperTicker(func(got time.Duration) live.Ticker {
			interval = got
			return ticker
		}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sweeper.Run(ctx)
		close(done)
	}()

	ticker.ch <- now
	select {
	case event := <-events:
		if event.Type != "snapshot.offline" || event.Snapshot.Online || event.Snapshot.ServerID != 7 {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("offline event was not published")
	}
	ticker.ch <- now
	select {
	case event := <-events:
		t.Fatalf("duplicate event = %+v", event)
	case <-time.After(20 * time.Millisecond):
	}
	if interval != 5*time.Second {
		t.Fatalf("interval=%s", interval)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sweeper did not stop after cancellation")
	}
	if !ticker.stopped.Load() {
		t.Fatal("ticker was not stopped")
	}
}

func TestSweeperStopsPromptlyWithoutTick(t *testing.T) {
	ticker := &fakeTicker{ch: make(chan time.Time)}
	sweeper := live.NewSweeper(live.NewStore(), live.NewHub(), live.WithSweeperTicker(func(time.Duration) live.Ticker { return ticker }))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sweeper.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sweeper did not stop after cancellation")
	}
}
