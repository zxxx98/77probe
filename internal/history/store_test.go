package history_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"probe.local/monitor/internal/history"
	"probe.local/monitor/internal/servers"
)

const thirtyDaysSeconds = int64(30 * 24 * 60 * 60)

func TestStoreUpsertMinuteRoundTripsJSONAndOverwrites(t *testing.T) {
	store, _, serverID := newHistoryStore(t)
	ctx := context.Background()
	record := history.MinuteRecord{
		ServerID:   serverID,
		MinuteUnix: 600,
		Payload: history.MinutePayload{
			CPUUsage:      history.Pair{Average: 12.5, Maximum: 25},
			Load1:         history.Pair{Average: 1, Maximum: 2},
			Load5:         history.Pair{Average: 3, Maximum: 4},
			Load15:        history.Pair{Average: 5, Maximum: 6},
			MemoryUsage:   history.Pair{Average: 50, Maximum: 75},
			SwapUsage:     history.Pair{Average: 10, Maximum: 20},
			Disks:         []history.DiskMinute{{Mountpoint: "/", Usage: history.Pair{Average: 40, Maximum: 60}, TotalBytes: 1000, UsedBytes: 600}},
			DiskReadBPS:   history.Pair{Average: 100, Maximum: 200},
			DiskWriteBPS:  history.Pair{Average: 300, Maximum: 400},
			UploadBPS:     history.Pair{Average: 500, Maximum: 600},
			DownloadBPS:   history.Pair{Average: 700, Maximum: 800},
			TotalUpload:   900,
			TotalDownload: 1000,
		},
	}
	if err := store.UpsertMinute(ctx, record); err != nil {
		t.Fatal(err)
	}

	overwrite := record
	overwrite.Payload.CPUUsage = history.Pair{Average: 33, Maximum: 44}
	overwrite.Payload.Disks[0].UsedBytes = 700
	if err := store.UpsertMinute(ctx, overwrite); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(ctx, serverID, 600, 600)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if !reflect.DeepEqual(records[0], overwrite) {
		t.Fatalf("record = %+v, want %+v", records[0], overwrite)
	}
}

func TestStoreQueryScopesServerOrdersMinutesAndAllocatesEmptySlice(t *testing.T) {
	store, conn, firstServerID := newHistoryStore(t)
	secondServerID := createHistoryServer(t, conn, "second")
	ctx := context.Background()

	insertMinute(t, store, firstServerID, 120)
	insertMinute(t, store, secondServerID, 60)
	insertMinute(t, store, firstServerID, 60)

	records, err := store.Query(ctx, firstServerID, 0, 180)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := minuteValues(records), []int64{60, 120}; !reflect.DeepEqual(got, want) {
		t.Fatalf("minutes = %v, want %v", got, want)
	}

	empty, err := store.Query(ctx, firstServerID, 121, 180)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty query = %#v, want allocated empty slice", empty)
	}
}

func TestStoreQueryRangeBoundaries(t *testing.T) {
	store, _, serverID := newHistoryStore(t)
	ctx := context.Background()

	if _, err := store.Query(ctx, serverID, 0, thirtyDaysSeconds); err != nil {
		t.Fatalf("exactly 30 days rejected: %v", err)
	}

	for _, test := range []struct {
		name string
		from int64
		to   int64
	}{
		{name: "reversed", from: 2, to: 1},
		{name: "over thirty days", from: 0, to: thirtyDaysSeconds + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Query(ctx, serverID, test.from, test.to)
			if !errors.Is(err, history.ErrInvalidRange) {
				t.Fatalf("Query() error = %v, want %v", err, history.ErrInvalidRange)
			}
			var rangeErr *history.InvalidRangeError
			if !errors.As(err, &rangeErr) || rangeErr.FromUnix != test.from || rangeErr.ToUnix != test.to {
				t.Fatalf("Query() error = %#v, want typed range error for %d..%d", err, test.from, test.to)
			}
		})
	}
}

func TestStoreQueryCapsResultsAtThirtyDaysOfMinutes(t *testing.T) {
	store, conn, serverID := newHistoryStore(t)
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`
		INSERT INTO metric_minutes(server_id, minute_unix, payload_json, created_at)
		VALUES (?, ?, '{}', '2026-07-28T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}
	for minute := int64(0); minute < 43_202; minute++ {
		if _, err := statement.Exec(serverID, minute); err != nil {
			statement.Close()
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	records, err := store.Query(context.Background(), serverID, 0, thirtyDaysSeconds)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 43_201 {
		t.Fatalf("records = %d, want 43201", len(records))
	}
	if records[len(records)-1].MinuteUnix != 43_200 {
		t.Fatalf("last minute = %d, want 43200", records[len(records)-1].MinuteUnix)
	}
}

func TestDeleteBeforeKeepsCutoffMinute(t *testing.T) {
	store, _, serverID := newHistoryStore(t)
	insertMinute(t, store, serverID, 99)
	insertMinute(t, store, serverID, 100)

	deleted, err := store.DeleteBefore(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	records, err := store.Query(context.Background(), serverID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := minuteValues(records), []int64{100}; !reflect.DeepEqual(got, want) {
		t.Fatalf("minutes = %v, want %v", got, want)
	}
}

func newHistoryStore(t *testing.T) (*history.Store, *sql.DB, int64) {
	t.Helper()
	conn := migratedDatabase(t)
	return history.NewStore(conn), conn, createHistoryServer(t, conn, "history fixture")
}

func createHistoryServer(t *testing.T, conn *sql.DB, name string) int64 {
	t.Helper()
	server, _, err := servers.NewService(conn).Create(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return server.ID
}

func insertMinute(t *testing.T, store *history.Store, serverID, minuteUnix int64) {
	t.Helper()
	record := history.MinuteRecord{
		ServerID:   serverID,
		MinuteUnix: minuteUnix,
		Payload: history.MinutePayload{
			CPUUsage: history.Pair{Average: float64(minuteUnix), Maximum: float64(minuteUnix) + 1},
			Disks:    []history.DiskMinute{},
		},
	}
	if err := store.UpsertMinute(context.Background(), record); err != nil {
		t.Fatalf("insert minute %d: %v", minuteUnix, err)
	}
}

func minuteValues(records []history.MinuteRecord) []int64 {
	minutes := make([]int64, 0, len(records))
	for _, record := range records {
		minutes = append(minutes, record.MinuteUnix)
	}
	return minutes
}

func ExampleInvalidRangeError() {
	err := &history.InvalidRangeError{FromUnix: 10, ToUnix: 5}
	fmt.Println(errors.Is(err, history.ErrInvalidRange))
	// Output: true
}
