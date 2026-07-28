package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/servers"
)

func TestStoreQueryReleasesConnectionWhileDecoding(t *testing.T) {
	store, conn, serverID := newInternalHistoryStore(t)
	if err := store.UpsertMinute(context.Background(), MinuteRecord{
		ServerID:   serverID,
		MinuteUnix: 100,
		Payload: MinutePayload{
			CPUUsage: Pair{Average: 1, Maximum: 2},
			Disks:    []DiskMinute{},
		},
	}); err != nil {
		t.Fatal(err)
	}

	decodeStarted := make(chan struct{})
	releaseDecode := make(chan struct{})
	unblockDecode := sync.OnceFunc(func() { close(releaseDecode) })
	defer unblockDecode()
	store.decodePayload = func(data []byte, payload *MinutePayload) error {
		close(decodeStarted)
		<-releaseDecode
		return json.Unmarshal(data, payload)
	}

	type queryResult struct {
		records []MinuteRecord
		err     error
	}
	queryDone := make(chan queryResult, 1)
	go func() {
		records, err := store.Query(context.Background(), serverID, 100, 100)
		queryDone <- queryResult{records: records, err: err}
	}()

	select {
	case <-decodeStarted:
	case <-time.After(time.Second):
		t.Fatal("history query did not begin JSON decoding")
	}

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 2*time.Second)
	writeErr := servers.NewService(conn).UpdateAgentVersion(writeCtx, serverID, "2.0.0")
	cancelWrite()
	unblockDecode()

	var result queryResult
	select {
	case result = <-queryDone:
	case <-time.After(time.Second):
		t.Fatal("history query did not finish after decoder release")
	}
	if result.err != nil {
		t.Fatalf("Query() error = %v", result.err)
	}
	if len(result.records) != 1 {
		t.Fatalf("Query() records = %d, want 1", len(result.records))
	}
	if writeErr != nil {
		t.Fatalf("agent version write while history decoded = %v, want completion before timeout", writeErr)
	}
}

func TestStoreQueryCancellationDuringDecodeReturnsNoPartialResults(t *testing.T) {
	store, _, serverID := newInternalHistoryStore(t)
	if err := store.UpsertMinute(context.Background(), MinuteRecord{
		ServerID:   serverID,
		MinuteUnix: 100,
		Payload:    MinutePayload{Disks: []DiskMinute{}},
	}); err != nil {
		t.Fatal(err)
	}

	decodeStarted := make(chan struct{})
	releaseDecode := make(chan struct{})
	unblockDecode := sync.OnceFunc(func() { close(releaseDecode) })
	defer unblockDecode()
	store.decodePayload = func(data []byte, payload *MinutePayload) error {
		close(decodeStarted)
		<-releaseDecode
		return json.Unmarshal(data, payload)
	}

	ctx, cancel := context.WithCancel(context.Background())
	type queryResult struct {
		records []MinuteRecord
		err     error
	}
	queryDone := make(chan queryResult, 1)
	go func() {
		records, err := store.Query(ctx, serverID, 100, 100)
		queryDone <- queryResult{records: records, err: err}
	}()

	select {
	case <-decodeStarted:
	case <-time.After(time.Second):
		t.Fatal("history query did not begin JSON decoding")
	}
	cancel()
	unblockDecode()

	select {
	case result := <-queryDone:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("Query() error = %v, want context cancellation", result.err)
		}
		if result.records != nil {
			t.Fatalf("Query() records = %#v, want no partial results", result.records)
		}
	case <-time.After(time.Second):
		t.Fatal("history query did not return after cancellation")
	}
}

func newInternalHistoryStore(t *testing.T) (*Store, *sql.DB, int64) {
	t.Helper()

	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Errorf("close database: %v", err)
		}
	})
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	server, _, err := servers.NewService(conn).Create(context.Background(), "history isolation fixture")
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(conn), conn, server.ID
}
