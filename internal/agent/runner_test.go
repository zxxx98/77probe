package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"probe.local/monitor/internal/protocol"
)

type collectResult struct {
	report protocol.AgentReport
	err    error
}

type sequenceCollector struct {
	results []collectResult
	calls   int
}

func (c *sequenceCollector) Collect(context.Context) (protocol.AgentReport, error) {
	result := c.results[c.calls]
	c.calls++
	return result.report, result.err
}

type sequenceSender struct {
	errors   []error
	received []protocol.AgentReport
}

func (s *sequenceSender) Send(_ context.Context, report protocol.AgentReport) error {
	s.received = append(s.received, report)
	if len(s.errors) == 0 {
		return nil
	}
	err := s.errors[0]
	s.errors = s.errors[1:]
	return err
}

func TestRunnerCollectsAndSendsImmediatelyThenWaitsFiveSeconds(t *testing.T) {
	collector := &sequenceCollector{results: []collectResult{{report: protocol.AgentReport{CollectedAtUnix: 1}}}}
	sender := &sequenceSender{}
	var waits []time.Duration
	runner := Runner{
		Collector: collector,
		Client:    sender,
		Wait: func(context.Context, time.Duration) error {
			return context.Canceled
		},
	}
	runner.Wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return context.Canceled
	}

	err := runner.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if collector.calls != 1 || len(sender.received) != 1 {
		t.Fatalf("immediate attempt calls = collect %d send %d, want 1 each", collector.calls, len(sender.received))
	}
	if !reflect.DeepEqual(waits, []time.Duration{5 * time.Second}) {
		t.Fatalf("waits = %v, want [5s]", waits)
	}
}

func TestRunnerBackoffCapsAtSixtySeconds(t *testing.T) {
	collector := &sequenceCollector{results: make([]collectResult, 6)}
	sender := &sequenceSender{errors: []error{
		errors.New("send 1"), errors.New("send 2"), errors.New("send 3"),
		errors.New("send 4"), errors.New("send 5"), errors.New("send 6"),
	}}
	var waits []time.Duration
	runner := Runner{Collector: collector, Client: sender}
	runner.Wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		if len(waits) == 6 {
			return context.Canceled
		}
		return nil
	}

	err := runner.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second}
	if !reflect.DeepEqual(waits, want) {
		t.Fatalf("waits = %v, want %v", waits, want)
	}
	if collector.calls != 6 || len(sender.received) != 6 {
		t.Fatalf("attempt calls = collect %d send %d, want 6 each", collector.calls, len(sender.received))
	}
}

func TestRunnerSuccessfulSendResetsBackoff(t *testing.T) {
	collector := &sequenceCollector{results: make([]collectResult, 4)}
	sender := &sequenceSender{errors: []error{errors.New("send 1"), errors.New("send 2"), nil, errors.New("send 4")}}
	var waits []time.Duration
	runner := Runner{Collector: collector, Client: sender}
	runner.Wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		if len(waits) == 4 {
			return context.Canceled
		}
		return nil
	}

	err := runner.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	want := []time.Duration{5 * time.Second, 10 * time.Second, 5 * time.Second, 5 * time.Second}
	if !reflect.DeepEqual(waits, want) {
		t.Fatalf("waits = %v, want %v", waits, want)
	}
}

func TestRunnerCollectsFreshReportInsteadOfReplayingFailure(t *testing.T) {
	collector := &sequenceCollector{results: []collectResult{
		{report: protocol.AgentReport{CollectedAtUnix: 1}},
		{report: protocol.AgentReport{CollectedAtUnix: 2}},
	}}
	sender := &sequenceSender{errors: []error{errors.New("send failed"), nil}}
	waits := 0
	runner := Runner{Collector: collector, Client: sender}
	runner.Wait = func(context.Context, time.Duration) error {
		waits++
		if waits == 2 {
			return context.Canceled
		}
		return nil
	}

	err := runner.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := []int64{sender.received[0].CollectedAtUnix, sender.received[1].CollectedAtUnix}; !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("sent report timestamps = %v, want [1 2]", got)
	}
}

func TestRunnerRetriesCollectorFailureWithoutSendingPartialReport(t *testing.T) {
	collector := &sequenceCollector{results: []collectResult{
		{err: errors.New("source failed")},
		{report: protocol.AgentReport{CollectedAtUnix: 2}},
	}}
	sender := &sequenceSender{}
	var waits []time.Duration
	runner := Runner{Collector: collector, Client: sender}
	runner.Wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		if len(waits) == 2 {
			return context.Canceled
		}
		return nil
	}

	err := runner.Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if len(sender.received) != 1 || sender.received[0].CollectedAtUnix != 2 {
		t.Fatalf("sent reports = %+v, want only fresh report 2", sender.received)
	}
	if !reflect.DeepEqual(waits, []time.Duration{5 * time.Second, 5 * time.Second}) {
		t.Fatalf("waits = %v, want [5s 5s]", waits)
	}
}

func TestRunnerWaitIsCancellationResponsive(t *testing.T) {
	collector := &sequenceCollector{results: []collectResult{{report: protocol.AgentReport{CollectedAtUnix: 1}}}}
	sender := &sequenceSender{}
	waiting := make(chan struct{})
	runner := Runner{Collector: collector, Client: sender}
	runner.Wait = func(ctx context.Context, _ time.Duration) error {
		close(waiting)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	<-waiting
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
}

func TestRunnerRejectsMissingDependencies(t *testing.T) {
	if err := (Runner{}).Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want missing dependency error")
	}
}
