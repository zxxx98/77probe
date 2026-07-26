package live_test

import (
	"fmt"
	"testing"

	"probe.local/monitor/internal/live"
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
