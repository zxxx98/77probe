package live_test

import (
	"fmt"
	"sync"
	"testing"

	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
)

func TestHubOverflowDropsOldestAndKeepsNewest(t *testing.T) {
	hub := live.NewHub()
	events, cancel := hub.Subscribe()
	defer cancel()

	for i := range 10 {
		hub.Publish(live.Event{Type: fmt.Sprintf("event-%d", i)})
	}

	for want := 2; want < 10; want++ {
		got := <-events
		if got.Type != fmt.Sprintf("event-%d", want) {
			t.Fatalf("event=%q want=event-%d", got.Type, want)
		}
	}
}

func TestHubSlowSubscriberDoesNotBlockAnotherSubscriber(t *testing.T) {
	hub := live.NewHub()
	_, cancelSlow := hub.Subscribe()
	defer cancelSlow()
	fast, cancelFast := hub.Subscribe()
	defer cancelFast()

	for i := range 20 {
		hub.Publish(live.Event{Type: fmt.Sprintf("event-%d", i)})
	}

	for want := 12; want < 20; want++ {
		if got := <-fast; got.Type != fmt.Sprintf("event-%d", want) {
			t.Fatalf("event=%q want=event-%d", got.Type, want)
		}
	}
}

func TestHubDeliversDetachedEventDisksToEachSubscriber(t *testing.T) {
	hub := live.NewHub()
	first, cancelFirst := hub.Subscribe()
	defer cancelFirst()
	second, cancelSecond := hub.Subscribe()
	defer cancelSecond()
	event := live.Event{Snapshot: live.Snapshot{Report: protocol.AgentReport{Disks: []protocol.DiskStats{{Mountpoint: "/original"}}}}}

	hub.Publish(event)
	event.Snapshot.Report.Disks[0].Mountpoint = "/input-mutated"
	firstEvent := <-first
	firstEvent.Snapshot.Report.Disks[0].Mountpoint = "/subscriber-mutated"
	secondEvent := <-second

	if secondEvent.Snapshot.Report.Disks[0].Mountpoint != "/original" {
		t.Fatalf("second subscriber mountpoint=%q", secondEvent.Snapshot.Report.Disks[0].Mountpoint)
	}
}

func TestHubCloseClosesCurrentAndFutureSubscribersIdempotently(t *testing.T) {
	hub := live.NewHub()
	events, cancel := hub.Subscribe()

	hub.Close()
	hub.Close()
	cancel()
	hub.Publish(live.Event{Type: "ignored"})

	if event, open := <-events; open {
		t.Fatalf("existing subscriber remained open with event %+v", event)
	}
	future, cancelFuture := hub.Subscribe()
	defer cancelFuture()
	if event, open := <-future; open {
		t.Fatalf("future subscriber remained open with event %+v", event)
	}
}

func TestHubCloseIsSafeAgainstConcurrentCancelAndPublish(t *testing.T) {
	hub := live.NewHub()
	cancels := make([]func(), 0, 32)
	for range 32 {
		_, cancel := hub.Subscribe()
		cancels = append(cancels, cancel)
	}

	var wait sync.WaitGroup
	wait.Add(len(cancels) + 2)
	for _, cancel := range cancels {
		cancel := cancel
		go func() {
			defer wait.Done()
			cancel()
		}()
	}
	go func() {
		defer wait.Done()
		hub.Publish(live.Event{Type: "racing"})
	}()
	go func() {
		defer wait.Done()
		hub.Close()
	}()
	wait.Wait()
	hub.Close()
}
