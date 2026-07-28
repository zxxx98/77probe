package alerting

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/servers"
)

func newRepository(t *testing.T) (*Repository, servers.Server) {
	t.Helper()
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	server, _, err := servers.NewService(conn).Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	return NewRepository(conn), server
}

func TestRepositoryCreatesRuleStateAndEvent(t *testing.T) {
	repository, server := newRepository(t)
	ctx := context.Background()
	rule, err := repository.CreateRule(ctx, Rule{ServerID: server.ID, Metric: MetricCPUUsage, Operator: OperatorGreaterThan, Threshold: 85, DurationSeconds: 300, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	value, threshold := 91.0, 85.0
	state := State{RuleID: rule.ID, Status: StatusFiring, FiringSince: &now, LastNotifiedAt: &now}
	event, err := repository.SaveStateAndEvent(ctx, state, &Event{RuleID: rule.ID, ServerID: server.ID, Status: StatusFiring, CurrentValue: &value, Threshold: &threshold, StartedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == 0 {
		t.Fatal("event ID was not assigned")
	}
	rules, err := repository.ListRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].State != StatusFiring {
		t.Fatalf("rules=%+v", rules)
	}
	events, err := repository.ListEvents(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != event.ID || events[0].ServerName != server.Name {
		t.Fatalf("events=%+v", events)
	}
}

func TestRepositoryMasksNothingAndCascadesRuleEvents(t *testing.T) {
	repository, server := newRepository(t)
	ctx := context.Background()
	rule, err := repository.CreateRule(ctx, Rule{ServerID: server.ID, Metric: MetricOffline, Operator: OperatorGreaterThan, DurationSeconds: 0, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	value, threshold := 1.0, 0.0
	if _, err := repository.SaveStateAndEvent(ctx, State{RuleID: rule.ID, Status: StatusFiring}, &Event{RuleID: rule.ID, ServerID: server.ID, Status: StatusFiring, CurrentValue: &value, Threshold: &threshold, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatal(err)
	}
	events, err := repository.ListEvents(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events after cascading rule delete=%+v", events)
	}
}
