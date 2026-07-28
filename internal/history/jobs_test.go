package history

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestJobsRunStartupRetentionAndScheduledWork(t *testing.T) {
	clock := newJobsClock(time.Date(2026, time.July, 28, 3, 4, 59, 0, time.FixedZone("UTC+2", 2*60*60)))
	flusher := newRecordingFlusher()
	retention := newRecordingRetention()
	tickers := newManualJobTickers()
	jobs := newJobs(flusher, retention, jobsConfig{
		now:         clock.Now,
		newTicker:   tickers.New,
		reportError: func(error) {},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		jobs.Run(ctx)
		close(done)
	}()

	startup := receiveRetentionCall(t, retention.calls)
	wantStartupCutoff := clock.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(time.Minute).Unix()
	if startup.cutoffUnix != wantStartupCutoff {
		t.Fatalf("startup cutoff = %d, want %d", startup.cutoffUnix, wantStartupCutoff)
	}
	tickers.WaitFor(t, 5*time.Second)
	tickers.WaitFor(t, 6*time.Hour)

	next := time.Date(2026, time.July, 28, 5, 6, 42, 0, time.FixedZone("UTC-3", -3*60*60))
	clock.Set(next)
	tickers.Tick(5 * time.Second)
	flush := receiveFlushCall(t, flusher.calls)
	if want := next.UTC().Truncate(time.Minute); !flush.minute.Equal(want) {
		t.Fatalf("flush boundary = %s, want %s", flush.minute, want)
	}

	tickers.Tick(6 * time.Hour)
	periodic := receiveRetentionCall(t, retention.calls)
	wantPeriodicCutoff := next.Add(-30 * 24 * time.Hour).UTC().Truncate(time.Minute).Unix()
	if periodic.cutoffUnix != wantPeriodicCutoff {
		t.Fatalf("periodic cutoff = %d, want %d", periodic.cutoffUnix, wantPeriodicCutoff)
	}

	cancel()
	shutdown := receiveFlushCall(t, flusher.calls)
	if !shutdown.minute.Equal(next.UTC().Truncate(time.Minute)) {
		t.Fatalf("shutdown boundary = %s, want %s", shutdown.minute, next.UTC().Truncate(time.Minute))
	}
	waitForJobsDone(t, done)
	if !tickers.Stopped(5*time.Second) || !tickers.Stopped(6*time.Hour) {
		t.Fatal("job tickers were not stopped")
	}
}

func TestJobsShutdownFlushUsesFreshBoundedContext(t *testing.T) {
	clock := newJobsClock(time.Date(2026, time.July, 28, 1, 2, 59, 0, time.UTC))
	flusher := newRecordingFlusher()
	retention := newRecordingRetention()
	tickers := newManualJobTickers()
	jobs := newJobs(flusher, retention, jobsConfig{
		now:             clock.Now,
		newTicker:       tickers.New,
		reportError:     func(error) {},
		shutdownTimeout: 250 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		jobs.Run(ctx)
		close(done)
	}()
	_ = receiveRetentionCall(t, retention.calls)
	tickers.WaitFor(t, 5*time.Second)
	tickers.WaitFor(t, 6*time.Hour)

	cancel()
	call := receiveFlushCall(t, flusher.calls)
	if call.contextErr != nil {
		t.Fatalf("shutdown context error = %v, want nil", call.contextErr)
	}
	if !call.hasDeadline {
		t.Fatal("shutdown context has no deadline")
	}
	remaining := time.Until(call.deadline)
	if remaining <= 0 || remaining > 250*time.Millisecond {
		t.Fatalf("shutdown deadline remaining = %s", remaining)
	}
	if want := clock.Now().UTC().Truncate(time.Minute); !call.minute.Equal(want) {
		t.Fatalf("shutdown boundary = %s, want %s", call.minute, want)
	}
	waitForJobsDone(t, done)
}

func TestJobsIsolateErrorsAndContinueScheduling(t *testing.T) {
	startupErr := errors.New("startup retention failed")
	flushErr := errors.New("periodic flush failed")
	retentionErr := errors.New("periodic retention failed")
	shutdownErr := errors.New("shutdown flush failed")
	clock := newJobsClock(time.Date(2026, time.July, 28, 1, 2, 3, 0, time.UTC))
	flusher := newRecordingFlusher(flushErr, nil, shutdownErr)
	retention := newRecordingRetention(startupErr, retentionErr)
	tickers := newManualJobTickers()
	errorsReported := make(chan error, 4)
	jobs := newJobs(flusher, retention, jobsConfig{
		now:         clock.Now,
		newTicker:   tickers.New,
		reportError: func(err error) { errorsReported <- err },
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		jobs.Run(ctx)
		close(done)
	}()
	_ = receiveRetentionCall(t, retention.calls)
	assertReportedError(t, errorsReported, startupErr)
	tickers.WaitFor(t, 5*time.Second)
	tickers.WaitFor(t, 6*time.Hour)

	tickers.Tick(5 * time.Second)
	_ = receiveFlushCall(t, flusher.calls)
	assertReportedError(t, errorsReported, flushErr)

	tickers.Tick(5 * time.Second)
	_ = receiveFlushCall(t, flusher.calls)

	tickers.Tick(6 * time.Hour)
	_ = receiveRetentionCall(t, retention.calls)
	assertReportedError(t, errorsReported, retentionErr)

	cancel()
	_ = receiveFlushCall(t, flusher.calls)
	assertReportedError(t, errorsReported, shutdownErr)
	waitForJobsDone(t, done)
}

func TestJobsTreatsRunCancellationAsNormal(t *testing.T) {
	clock := newJobsClock(time.Date(2026, time.July, 28, 1, 2, 3, 0, time.UTC))
	flusher := newRecordingFlusher()
	retention := newRecordingRetention(context.Canceled)
	tickers := newManualJobTickers()
	errorsReported := make(chan error, 1)
	jobs := newJobs(flusher, retention, jobsConfig{
		now:         clock.Now,
		newTicker:   tickers.New,
		reportError: func(err error) { errorsReported <- err },
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	jobs.Run(ctx)

	select {
	case err := <-errorsReported:
		t.Fatalf("reported normal cancellation as an error: %v", err)
	default:
	}
}

type jobsClock struct {
	mu  sync.Mutex
	now time.Time
}

func newJobsClock(now time.Time) *jobsClock {
	return &jobsClock{now: now}
}

func (c *jobsClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *jobsClock) Set(now time.Time) {
	c.mu.Lock()
	c.now = now
	c.mu.Unlock()
}

type flushCall struct {
	minute      time.Time
	contextErr  error
	hasDeadline bool
	deadline    time.Time
}

type recordingFlusher struct {
	mu      sync.Mutex
	results []error
	calls   chan flushCall
}

func newRecordingFlusher(results ...error) *recordingFlusher {
	return &recordingFlusher{results: results, calls: make(chan flushCall, 16)}
}

func (f *recordingFlusher) FlushBefore(ctx context.Context, minute time.Time) error {
	deadline, hasDeadline := ctx.Deadline()
	f.calls <- flushCall{minute: minute, contextErr: ctx.Err(), hasDeadline: hasDeadline, deadline: deadline}
	f.mu.Lock()
	defer f.mu.Unlock()
	index := len(f.results)
	if index == 0 {
		return nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result
}

type retentionCall struct {
	cutoffUnix int64
}

type recordingRetention struct {
	mu      sync.Mutex
	results []error
	calls   chan retentionCall
}

func newRecordingRetention(results ...error) *recordingRetention {
	return &recordingRetention{results: results, calls: make(chan retentionCall, 16)}
}

func (r *recordingRetention) DeleteBefore(_ context.Context, cutoffUnix int64) (int64, error) {
	r.calls <- retentionCall{cutoffUnix: cutoffUnix}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.results) == 0 {
		return 0, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return 0, result
}

type manualJobTicker struct {
	ch      chan time.Time
	mu      sync.Mutex
	stopped bool
}

func (t *manualJobTicker) C() <-chan time.Time {
	return t.ch
}

func (t *manualJobTicker) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

type manualJobTickers struct {
	mu      sync.Mutex
	tickers map[time.Duration]*manualJobTicker
	created chan time.Duration
}

func newManualJobTickers() *manualJobTickers {
	return &manualJobTickers{
		tickers: make(map[time.Duration]*manualJobTicker),
		created: make(chan time.Duration, 2),
	}
}

func (f *manualJobTickers) New(interval time.Duration) jobTicker {
	ticker := &manualJobTicker{ch: make(chan time.Time, 1)}
	f.mu.Lock()
	f.tickers[interval] = ticker
	f.mu.Unlock()
	f.created <- interval
	return ticker
}

func (f *manualJobTickers) WaitFor(t *testing.T, interval time.Duration) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		f.mu.Lock()
		_, ok := f.tickers[interval]
		f.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-f.created:
		case <-deadline:
			t.Fatalf("ticker %s was not created", interval)
		}
	}
}

func (f *manualJobTickers) Tick(interval time.Duration) {
	f.mu.Lock()
	ticker := f.tickers[interval]
	f.mu.Unlock()
	ticker.ch <- time.Time{}
}

func (f *manualJobTickers) Stopped(interval time.Duration) bool {
	f.mu.Lock()
	ticker := f.tickers[interval]
	f.mu.Unlock()
	ticker.mu.Lock()
	defer ticker.mu.Unlock()
	return ticker.stopped
}

func receiveFlushCall(t *testing.T, calls <-chan flushCall) flushCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for flush call")
		return flushCall{}
	}
}

func receiveRetentionCall(t *testing.T, calls <-chan retentionCall) retentionCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for retention call")
		return retentionCall{}
	}
}

func assertReportedError(t *testing.T, reported <-chan error, want error) {
	t.Helper()
	select {
	case err := <-reported:
		if !errors.Is(err, want) {
			t.Fatalf("reported error = %v, want wrapped %v", err, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for reported error %v", want)
	}
}

func waitForJobsDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("jobs did not stop")
	}
}
