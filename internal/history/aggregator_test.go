package history

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"probe.local/monitor/internal/protocol"
)

func TestAggregatorBucketsByUTCReceivedMinute(t *testing.T) {
	writer := &recordingWriter{}
	aggregator := NewAggregator(writer)
	receivedAt := time.Date(2026, time.July, 28, 3, 4, 59, 999, time.FixedZone("UTC+2", 2*60*60))

	aggregator.Accept(7, protocol.AgentReport{Network: protocol.NetworkStats{TotalUploadBytes: 123}}, receivedAt)
	boundary := receivedAt.UTC().Truncate(time.Minute).Add(time.Minute)
	if err := aggregator.FlushBefore(context.Background(), boundary); err != nil {
		t.Fatal(err)
	}

	records := writer.snapshot()
	if len(records) != 1 {
		t.Fatalf("writes = %d, want 1", len(records))
	}
	if got, want := records[0].MinuteUnix, receivedAt.UTC().Truncate(time.Minute).Unix(); got != want {
		t.Fatalf("minute unix = %d, want %d", got, want)
	}
}

func TestAggregatorFlushBeforeUsesStrictBoundaryAndRemovesSuccesses(t *testing.T) {
	writer := &recordingWriter{}
	aggregator := NewAggregator(writer)
	minute := time.Date(2026, time.July, 28, 1, 2, 0, 0, time.UTC)

	aggregator.Accept(7, protocol.AgentReport{Network: protocol.NetworkStats{TotalUploadBytes: 100}}, minute.Add(30*time.Second))
	aggregator.Accept(7, protocol.AgentReport{Network: protocol.NetworkStats{TotalUploadBytes: 200}}, minute.Add(time.Minute+30*time.Second))

	if err := aggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertWrittenKeys(t, writer.snapshot(), []writtenKey{{ServerID: 7, MinuteUnix: minute.Unix()}})

	if err := aggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertWrittenKeys(t, writer.snapshot(), []writtenKey{{ServerID: 7, MinuteUnix: minute.Unix()}})

	if err := aggregator.FlushBefore(context.Background(), minute.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertWrittenKeys(t, writer.snapshot(), []writtenKey{
		{ServerID: 7, MinuteUnix: minute.Unix()},
		{ServerID: 7, MinuteUnix: minute.Add(time.Minute).Unix()},
	})
}

func TestAggregatorFlushesInServerThenMinuteOrder(t *testing.T) {
	writer := &recordingWriter{}
	aggregator := NewAggregator(writer)
	minute := time.Date(2026, time.July, 28, 1, 2, 0, 0, time.UTC)

	aggregator.Accept(2, protocol.AgentReport{}, minute.Add(30*time.Second))
	aggregator.Accept(1, protocol.AgentReport{}, minute.Add(time.Minute+30*time.Second))
	aggregator.Accept(1, protocol.AgentReport{}, minute.Add(30*time.Second))

	if err := aggregator.FlushBefore(context.Background(), minute.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertWrittenKeys(t, writer.snapshot(), []writtenKey{
		{ServerID: 1, MinuteUnix: minute.Unix()},
		{ServerID: 1, MinuteUnix: minute.Add(time.Minute).Unix()},
		{ServerID: 2, MinuteUnix: minute.Unix()},
	})
}

func TestAggregatorRetainsFailedBucketForRetry(t *testing.T) {
	wantErr := errors.New("write failed")
	writer := &recordingWriter{results: []error{wantErr, nil}}
	aggregator := NewAggregator(writer)
	minute := time.Date(2026, time.July, 28, 1, 2, 0, 0, time.UTC)
	aggregator.Accept(7, protocol.AgentReport{Network: protocol.NetworkStats{TotalUploadBytes: 123}}, minute)

	if err := aggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); !errors.Is(err, wantErr) {
		t.Fatalf("first FlushBefore() error = %v, want %v", err, wantErr)
	}
	if err := aggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatalf("retry FlushBefore() error = %v", err)
	}
	if err := aggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatalf("post-success FlushBefore() error = %v", err)
	}

	records := writer.snapshot()
	if len(records) != 2 {
		t.Fatalf("writes = %d, want 2 attempts", len(records))
	}
	if !reflect.DeepEqual(records[0], records[1]) {
		t.Fatalf("retried record = %+v, want original %+v", records[1], records[0])
	}
}

func TestAggregatorAcceptDuringFlushIsNotBlockedOrLost(t *testing.T) {
	writer := newBlockingWriter()
	aggregator := NewAggregator(writer)
	minute := time.Date(2026, time.July, 28, 1, 2, 0, 0, time.UTC)
	aggregator.Accept(7, protocol.AgentReport{Network: protocol.NetworkStats{UploadBytesPerSecond: 100}}, minute)

	flushDone := make(chan error, 1)
	go func() {
		flushDone <- aggregator.FlushBefore(context.Background(), minute.Add(time.Minute))
	}()
	<-writer.started

	acceptDone := make(chan struct{})
	go func() {
		aggregator.Accept(7, protocol.AgentReport{Network: protocol.NetworkStats{UploadBytesPerSecond: 300}}, minute.Add(30*time.Second))
		close(acceptDone)
	}()
	select {
	case <-acceptDone:
	case <-time.After(time.Second):
		close(writer.release)
		t.Fatal("Accept blocked while writer was running")
	}

	close(writer.release)
	if err := <-flushDone; err != nil {
		t.Fatal(err)
	}
	if err := aggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	records := writer.snapshot()
	if len(records) != 2 {
		t.Fatalf("writes = %d, want 2", len(records))
	}
	if got, want := records[0].Payload.UploadBPS, (Pair{Average: 100, Maximum: 100}); got != want {
		t.Fatalf("first upload pair = %+v, want %+v", got, want)
	}
	if got, want := records[1].Payload.UploadBPS, (Pair{Average: 200, Maximum: 300}); got != want {
		t.Fatalf("concurrent upload pair = %+v, want %+v", got, want)
	}
}

type recordingWriter struct {
	mu      sync.Mutex
	records []MinuteRecord
	results []error
}

func (w *recordingWriter) UpsertMinute(_ context.Context, record MinuteRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.records = append(w.records, record)
	index := len(w.records) - 1
	if index < len(w.results) {
		return w.results[index]
	}
	return nil
}

func (w *recordingWriter) snapshot() []MinuteRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]MinuteRecord(nil), w.records...)
}

type blockingWriter struct {
	mu      sync.Mutex
	records []MinuteRecord
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingWriter) UpsertMinute(_ context.Context, record MinuteRecord) error {
	w.mu.Lock()
	w.records = append(w.records, record)
	w.mu.Unlock()

	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return nil
}

func (w *blockingWriter) snapshot() []MinuteRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]MinuteRecord(nil), w.records...)
}

type writtenKey struct {
	ServerID   int64
	MinuteUnix int64
}

func assertWrittenKeys(t *testing.T, records []MinuteRecord, want []writtenKey) {
	t.Helper()
	got := make([]writtenKey, 0, len(records))
	for _, record := range records {
		got = append(got, writtenKey{ServerID: record.ServerID, MinuteUnix: record.MinuteUnix})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("written keys = %+v, want %+v", got, want)
	}
}
