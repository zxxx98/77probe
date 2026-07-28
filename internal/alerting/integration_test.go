package alerting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
)

func TestAlertEpisodeEndToEnd(t *testing.T) {
	repository, server := newRepository(t)
	var deliveries atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		deliveries.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer endpoint.Close()
	if _, err := repository.UpsertWebhook(context.Background(), WebhookConfig{URL: endpoint.URL, Headers: map[string]string{}, BodyTemplate: `{"status":"{{.Status}}"}`, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	rule, err := repository.CreateRule(context.Background(), Rule{ServerID: server.ID, Metric: MetricCPUUsage, Operator: OperatorGreaterThan, Threshold: 80, DurationSeconds: 2, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(repository, NewWebhookClient())
	ctx, cancel := context.WithCancel(context.Background())
	dispatcherDone := make(chan struct{})
	go func() { dispatcher.Run(ctx); close(dispatcherDone) }()
	defer func() { cancel(); <-dispatcherDone }()
	evaluator := NewEvaluator(repository, live.NewStore(), dispatcher)
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	evaluator.now = func() time.Time { return now }
	snapshot := func(cpu float64) live.Snapshot {
		return live.Snapshot{ServerID: server.ID, ServerName: server.Name, Online: true, Report: protocol.AgentReport{CPU: protocol.CPUStats{UsagePercent: cpu}}}
	}
	evaluator.EvaluateNow(ctx, live.Event{Type: "snapshot.updated", Snapshot: snapshot(10)})
	now = now.Add(time.Second)
	evaluator.EvaluateNow(ctx, live.Event{Type: "snapshot.updated", Snapshot: snapshot(95)})
	now = now.Add(2 * time.Second)
	evaluator.EvaluateNow(ctx, live.Event{Type: "snapshot.updated", Snapshot: snapshot(95)})
	waitForDelivery(t, &deliveries, 1)
	now = now.Add(time.Second)
	evaluator.EvaluateNow(ctx, live.Event{Type: "snapshot.updated", Snapshot: snapshot(95)})
	if got := deliveries.Load(); got != 1 {
		t.Fatalf("duplicate firing delivery count=%d", got)
	}
	now = now.Add(time.Second)
	evaluator.EvaluateNow(ctx, live.Event{Type: "snapshot.updated", Snapshot: snapshot(10)})
	waitForDelivery(t, &deliveries, 2)
	events, err := repository.ListEvents(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Status != StatusRecovered || events[1].Status != StatusFiring {
		t.Fatalf("events=%+v", events)
	}
	if events[0].RuleID != rule.ID || len(events[0].Attempts) != 1 || len(events[1].Attempts) != 1 {
		t.Fatalf("events with attempts=%+v", events)
	}
}

func waitForDelivery(t *testing.T, deliveries *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if deliveries.Load() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("deliveries=%d, want %d", deliveries.Load(), want)
}
