package alerting

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"probe.local/monitor/internal/live"
)

const evaluationQueueCapacity = 128

type evaluationStore interface {
	ListEnabledRulesForServer(context.Context, int64) ([]Rule, error)
	GetState(context.Context, int64) (State, error)
	SaveStateAndEvent(context.Context, State, *Event) (Event, error)
	GetWebhook(context.Context) (WebhookConfig, error)
}

type snapshotStore interface {
	Get(int64) (live.Snapshot, bool)
}

type deliveryEnqueuer interface {
	Enqueue(DeliveryJob) error
}

type Evaluator struct {
	store      evaluationStore
	snapshots  snapshotStore
	deliveries deliveryEnqueuer
	queue      chan live.Event
	now        func() time.Time
	retryAfter time.Duration
	pendingMu  sync.Mutex
	pending    map[int64]struct{}
}

func NewEvaluator(store evaluationStore, snapshots snapshotStore, deliveries deliveryEnqueuer) *Evaluator {
	if store == nil || snapshots == nil || deliveries == nil {
		panic("alert evaluator requires store, snapshots, and deliveries")
	}
	return &Evaluator{
		store: store, snapshots: snapshots, deliveries: deliveries,
		queue: make(chan live.Event, evaluationQueueCapacity), now: time.Now,
		retryAfter: 250 * time.Millisecond, pending: make(map[int64]struct{}),
	}
}

func (e *Evaluator) Publish(event live.Event) { e.Submit(event) }

func (e *Evaluator) Submit(event live.Event) {
	select {
	case e.queue <- event:
	default:
		e.scheduleLatest(event.Snapshot.ServerID)
	}
}

func (e *Evaluator) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-e.queue:
			e.EvaluateNow(ctx, event)
		}
	}
}

func (e *Evaluator) EvaluateNow(ctx context.Context, event live.Event) {
	if event.Snapshot.ServerID < 1 {
		return
	}
	rules, err := e.store.ListEnabledRulesForServer(ctx, event.Snapshot.ServerID)
	if err != nil {
		log.Printf("alert evaluator: list rules for server %d: %v", event.Snapshot.ServerID, err)
		return
	}
	for _, rule := range rules {
		if event.Type == "snapshot.offline" && rule.Metric != MetricOffline {
			continue
		}
		value := MetricValue(event.Snapshot, rule.Metric)
		state, err := e.store.GetState(ctx, rule.ID)
		if err == ErrNotFound {
			state = State{RuleID: rule.ID, Status: StatusNormal}
		} else if err != nil {
			log.Printf("alert evaluator: get state for rule %d: %v", rule.ID, err)
			continue
		}
		result := Evaluate(EvaluationInput{
			State: state, Breached: breached(value, rule), CurrentValue: value,
			Duration: durationForRule(rule), RepeatInterval: time.Duration(rule.RepeatSeconds) * time.Second,
			Now: e.now(),
		})
		var created *Event
		if result.Notify {
			created = eventFor(rule, event.Snapshot, result.State, value)
		}
		storedEvent, err := e.store.SaveStateAndEvent(ctx, result.State, created)
		if err != nil {
			log.Printf("alert evaluator: save state for rule %d: %v", rule.ID, err)
			continue
		}
		if created != nil {
			e.enqueueDelivery(ctx, storedEvent, event.Snapshot, rule)
		}
	}
}

func (e *Evaluator) scheduleLatest(serverID int64) {
	if serverID < 1 {
		return
	}
	e.pendingMu.Lock()
	if _, alreadyPending := e.pending[serverID]; alreadyPending {
		e.pendingMu.Unlock()
		return
	}
	e.pending[serverID] = struct{}{}
	e.pendingMu.Unlock()
	time.AfterFunc(e.retryAfter, func() {
		snapshot, ok := e.snapshots.Get(serverID)
		if ok {
			select {
			case e.queue <- live.Event{Type: "snapshot.updated", Snapshot: snapshot}:
			default:
				e.pendingMu.Lock()
				delete(e.pending, serverID)
				e.pendingMu.Unlock()
				e.scheduleLatest(serverID)
				return
			}
		}
		e.pendingMu.Lock()
		delete(e.pending, serverID)
		e.pendingMu.Unlock()
	})
}

func (e *Evaluator) enqueueDelivery(ctx context.Context, event Event, snapshot live.Snapshot, rule Rule) {
	config, err := e.store.GetWebhook(ctx)
	if err == ErrNotFound || !config.Enabled {
		return
	}
	if err != nil {
		log.Printf("alert evaluator: get webhook: %v", err)
		return
	}
	current, threshold := 0.0, rule.Threshold
	if event.CurrentValue != nil {
		current = *event.CurrentValue
	}
	if event.Threshold != nil {
		threshold = *event.Threshold
	}
	err = e.deliveries.Enqueue(DeliveryJob{
		Event: event, Config: config,
		Data: TemplateData{EventID: event.ID, ServerID: event.ServerID, ServerName: snapshot.ServerName, Metric: rule.Metric,
			Status: event.Status, CurrentValue: current, Threshold: threshold, StartedAt: event.StartedAt, EndedAt: event.EndedAt,
			DetailURL: fmt.Sprintf("/servers/%d", event.ServerID)},
	})
	if err != nil {
		log.Printf("alert evaluator: enqueue webhook event %d: %v", event.ID, err)
	}
}

func durationForRule(rule Rule) time.Duration {
	if rule.Metric == MetricOffline {
		return 0
	}
	return time.Duration(rule.DurationSeconds) * time.Second
}

func eventFor(rule Rule, snapshot live.Snapshot, state State, value float64) *Event {
	startedAt := state.FiringSince
	if startedAt == nil {
		now := state.UpdatedAt
		startedAt = &now
	}
	event := &Event{RuleID: rule.ID, ServerID: snapshot.ServerID, Status: state.Status, CurrentValue: &value, Threshold: &rule.Threshold, StartedAt: *startedAt}
	if state.Status == StatusRecovered {
		ended := state.UpdatedAt
		event.EndedAt = &ended
	}
	return event
}
