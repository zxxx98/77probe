package alerting

import (
	"context"
	"testing"
	"time"

	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
)

type evaluatorStore struct {
	rules  []Rule
	states map[int64]State
	events []Event
}

func (s *evaluatorStore) ListEnabledRulesForServer(_ context.Context, _ int64) ([]Rule, error) {
	return s.rules, nil
}
func (s *evaluatorStore) GetState(_ context.Context, id int64) (State, error) {
	state, ok := s.states[id]
	if !ok {
		return State{}, ErrNotFound
	}
	return state, nil
}
func (s *evaluatorStore) SaveStateAndEvent(_ context.Context, state State, event *Event) (Event, error) {
	s.states[state.RuleID] = state
	if event == nil {
		return Event{}, nil
	}
	event.ID = int64(len(s.events) + 1)
	s.events = append(s.events, *event)
	return *event, nil
}
func (s *evaluatorStore) GetWebhook(context.Context) (WebhookConfig, error) {
	return WebhookConfig{}, ErrNotFound
}

type evaluatorSnapshots struct{}

func (evaluatorSnapshots) Get(int64) (live.Snapshot, bool) { return live.Snapshot{}, false }

type evaluatorDeliveries struct{}

func (evaluatorDeliveries) Enqueue(DeliveryJob) error { return nil }

func TestEvaluatorFiresOfflineAndRecovers(t *testing.T) {
	store := &evaluatorStore{rules: []Rule{{ID: 1, ServerID: 9, Metric: MetricOffline, Operator: OperatorGreaterThan, Threshold: 0, Enabled: true}}, states: map[int64]State{}}
	evaluator := NewEvaluator(store, evaluatorSnapshots{}, evaluatorDeliveries{})
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	evaluator.now = func() time.Time { return now }
	offline := live.Snapshot{ServerID: 9, ServerName: "home-lab", Online: false, Report: protocol.AgentReport{}}
	evaluator.EvaluateNow(context.Background(), live.Event{Type: "snapshot.offline", Snapshot: offline})
	if len(store.events) != 1 || store.events[0].Status != StatusFiring {
		t.Fatalf("events=%+v", store.events)
	}
	now = now.Add(time.Minute)
	online := offline
	online.Online = true
	evaluator.EvaluateNow(context.Background(), live.Event{Type: "snapshot.updated", Snapshot: online})
	if len(store.events) != 2 || store.events[1].Status != StatusRecovered {
		t.Fatalf("events=%+v", store.events)
	}
}

func TestMetricValueUsesDiskExtremes(t *testing.T) {
	snapshot := live.Snapshot{Report: protocol.AgentReport{Disks: []protocol.DiskStats{{TotalBytes: 100, UsedBytes: 20}, {TotalBytes: 100, UsedBytes: 90}}}}
	if got := MetricValue(snapshot, MetricDiskUsage); got != 90 {
		t.Fatalf("usage=%v", got)
	}
	if got := MetricValue(snapshot, MetricDiskFreeBytes); got != 10 {
		t.Fatalf("free=%v", got)
	}
}
