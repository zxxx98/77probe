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

func TestAggregatorSerializesFlushBeforeCalls(t *testing.T) {
	writer := newConcurrencyWriter()
	aggregator := NewAggregator(writer)
	minute := time.Date(2026, time.July, 28, 1, 2, 0, 0, time.UTC)
	aggregator.Accept(1, protocol.AgentReport{}, minute)

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- aggregator.FlushBefore(context.Background(), minute.Add(time.Minute))
	}()
	<-writer.firstStarted

	aggregator.Accept(2, protocol.AgentReport{}, minute)
	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondEntered)
		secondDone <- aggregator.FlushBefore(context.Background(), minute.Add(time.Minute))
	}()
	<-secondEntered

	select {
	case err := <-secondDone:
		close(writer.releaseFirst)
		firstErr := <-firstDone
		t.Fatalf("second FlushBefore returned while first was blocked: second error %v, first error %v", err, firstErr)
	case <-time.After(100 * time.Millisecond):
	}

	close(writer.releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first FlushBefore() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second FlushBefore() error = %v", err)
	}
	if got := writer.maximumConcurrency(); got != 1 {
		t.Fatalf("writer maximum concurrency = %d, want 1", got)
	}
	assertWrittenKeys(t, writer.snapshot(), []writtenKey{
		{ServerID: 1, MinuteUnix: minute.Unix()},
		{ServerID: 2, MinuteUnix: minute.Unix()},
	})
}

func TestAggregatorLastValuesUseLatestReceivedAt(t *testing.T) {
	writer := &recordingWriter{}
	aggregator := NewAggregator(writer)
	minute := time.Date(2026, time.July, 28, 1, 2, 0, 0, time.UTC)

	aggregator.Accept(7, protocol.AgentReport{
		CPU: protocol.CPUStats{UsagePercent: 30},
		Disks: []protocol.DiskStats{
			{Mountpoint: "/shared", TotalBytes: 300, UsedBytes: 150},
			{Mountpoint: "/newer", TotalBytes: 400, UsedBytes: 100},
		},
		Network: protocol.NetworkStats{
			UploadBytesPerSecond:   300,
			DownloadBytesPerSecond: 600,
			TotalUploadBytes:       3000,
			TotalDownloadBytes:     6000,
		},
	}, minute.Add(50*time.Second))
	aggregator.Accept(7, protocol.AgentReport{
		CPU: protocol.CPUStats{UsagePercent: 10},
		Disks: []protocol.DiskStats{
			{Mountpoint: "/shared", TotalBytes: 100, UsedBytes: 10},
			{Mountpoint: "/older", TotalBytes: 200, UsedBytes: 100},
		},
		Network: protocol.NetworkStats{
			UploadBytesPerSecond:   100,
			DownloadBytesPerSecond: 200,
			TotalUploadBytes:       1000,
			TotalDownloadBytes:     2000,
		},
	}, minute.Add(10*time.Second))

	if err := aggregator.FlushBefore(context.Background(), minute.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	records := writer.snapshot()
	if len(records) != 1 {
		t.Fatalf("writes = %d, want 1", len(records))
	}
	record := records[0]

	if got, want := record.Payload.CPUUsage, (Pair{Average: 20, Maximum: 30}); got != want {
		t.Errorf("CPU usage = %+v, want %+v", got, want)
	}
	if got, want := record.Payload.UploadBPS, (Pair{Average: 200, Maximum: 300}); got != want {
		t.Errorf("upload BPS = %+v, want %+v", got, want)
	}
	if got, want := record.Payload.TotalUpload, uint64(3000); got != want {
		t.Errorf("total upload = %d, want %d", got, want)
	}
	if got, want := record.Payload.TotalDownload, uint64(6000); got != want {
		t.Errorf("total download = %d, want %d", got, want)
	}
	assertDiskMinute(t, record.Payload.Disks, DiskMinute{
		Mountpoint: "/shared",
		Usage:      Pair{Average: 30, Maximum: 50},
		TotalBytes: 300,
		UsedBytes:  150,
	})
	assertDiskMinute(t, record.Payload.Disks, DiskMinute{
		Mountpoint: "/newer",
		Usage:      Pair{Average: 25, Maximum: 25},
		TotalBytes: 400,
		UsedBytes:  100,
	})
	assertDiskMinute(t, record.Payload.Disks, DiskMinute{
		Mountpoint: "/older",
		Usage:      Pair{Average: 50, Maximum: 50},
		TotalBytes: 200,
		UsedBytes:  100,
	})
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

type concurrencyWriter struct {
	mu           sync.Mutex
	records      []MinuteRecord
	active       int
	maxActive    int
	calls        int
	firstStarted chan struct{}
	releaseFirst chan struct{}
}

func newConcurrencyWriter() *concurrencyWriter {
	return &concurrencyWriter{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
}

func (w *concurrencyWriter) UpsertMinute(_ context.Context, record MinuteRecord) error {
	w.mu.Lock()
	w.calls++
	call := w.calls
	w.active++
	if w.active > w.maxActive {
		w.maxActive = w.active
	}
	w.records = append(w.records, record)
	if call == 1 {
		close(w.firstStarted)
	}
	w.mu.Unlock()

	if call == 1 {
		<-w.releaseFirst
	}

	w.mu.Lock()
	w.active--
	w.mu.Unlock()
	return nil
}

func (w *concurrencyWriter) maximumConcurrency() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxActive
}

func (w *concurrencyWriter) snapshot() []MinuteRecord {
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

func assertDiskMinute(t *testing.T, disks []DiskMinute, want DiskMinute) {
	t.Helper()
	for _, disk := range disks {
		if disk.Mountpoint == want.Mountpoint {
			if disk != want {
				t.Errorf("disk %q = %+v, want %+v", want.Mountpoint, disk, want)
			}
			return
		}
	}
	t.Errorf("disk %q not found in %+v", want.Mountpoint, disks)
}
