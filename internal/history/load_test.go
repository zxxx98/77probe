package history_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"probe.local/monitor/internal/history"
	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

func TestTenAgentsMinuteRowGrowth(t *testing.T) {
	ctx := context.Background()
	conn := migratedDatabase(t)
	serverService := servers.NewService(conn)
	store := history.NewStore(conn)
	aggregator := history.NewAggregator(store)
	start := time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)

	serverIDs := make([]int64, 0, 10)
	for agent := range 10 {
		server, _, err := serverService.Create(ctx, fmt.Sprintf("load-agent-%02d", agent+1))
		if err != nil {
			t.Fatalf("create server %d: %v", agent+1, err)
		}
		serverIDs = append(serverIDs, server.ID)
	}

	for minuteOffset := range 120 {
		minute := start.Add(time.Duration(minuteOffset) * time.Minute)
		for agent, serverID := range serverIDs {
			aggregator.Accept(serverID, tenAgentReport(agent, minute), minute.Add(15*time.Second))
		}
		if err := aggregator.FlushBefore(ctx, minute.Add(time.Minute)); err != nil {
			t.Fatalf("flush minute %d: %v", minuteOffset, err)
		}
	}

	assertMetricMinuteShape(t, conn, 1_200, 120, start.Unix(), start.Add(119*time.Minute).Unix())
	for _, serverID := range serverIDs {
		records, err := store.Query(ctx, serverID, start.Unix(), start.Add(119*time.Minute).Unix())
		if err != nil {
			t.Fatalf("query server %d before retention: %v", serverID, err)
		}
		assertConsecutiveMinutes(t, serverID, records, start, 120)
	}

	retentionNow := start.Add(30*24*time.Hour + 60*time.Minute)
	cutoff := retentionNow.Add(-30 * 24 * time.Hour)
	deleted, err := store.DeleteBefore(ctx, cutoff.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 600 {
		t.Fatalf("deleted rows = %d, want 600", deleted)
	}

	assertMetricMinuteShape(t, conn, 600, 60, cutoff.Unix(), start.Add(119*time.Minute).Unix())
	for _, serverID := range serverIDs {
		records, err := store.Query(ctx, serverID, start.Unix(), start.Add(119*time.Minute).Unix())
		if err != nil {
			t.Fatalf("query server %d after retention: %v", serverID, err)
		}
		assertConsecutiveMinutes(t, serverID, records, cutoff, 60)
	}
}

func tenAgentReport(agent int, minute time.Time) protocol.AgentReport {
	sequence := uint64(agent + 1)
	return protocol.AgentReport{
		CollectedAtUnix: minute.Add(15 * time.Second).Unix(),
		AgentVersion:    "load-test",
		Host: protocol.HostInfo{
			Hostname:      fmt.Sprintf("load-agent-%02d", agent+1),
			CPUModel:      "bounded-growth fixture",
			CPUCores:      agent + 1,
			UptimeSeconds: sequence * 3600,
		},
		CPU: protocol.CPUStats{
			UsagePercent: float64(10 + agent),
			Load1:        float64(sequence) / 10,
			Load5:        float64(sequence) / 20,
			Load15:       float64(sequence) / 30,
		},
		Memory: protocol.MemoryStats{
			TotalBytes:     8 * 1024 * 1024 * 1024,
			UsedBytes:      sequence * 512 * 1024 * 1024,
			SwapTotalBytes: 2 * 1024 * 1024 * 1024,
			SwapUsedBytes:  sequence * 32 * 1024 * 1024,
		},
		Disks: []protocol.DiskStats{{
			Mountpoint: "/",
			TotalBytes: 128 * 1024 * 1024 * 1024,
			UsedBytes:  sequence * 8 * 1024 * 1024 * 1024,
		}},
		DiskIO: protocol.DiskIOStats{
			ReadBytesPerSecond:  sequence * 1024,
			WriteBytesPerSecond: sequence * 2048,
		},
		Network: protocol.NetworkStats{
			Interface:              "eth0",
			UploadBytesPerSecond:   sequence * 4096,
			DownloadBytesPerSecond: sequence * 8192,
			TotalUploadBytes:       sequence * 1024 * 1024,
			TotalDownloadBytes:     sequence * 2 * 1024 * 1024,
		},
	}
}

func assertMetricMinuteShape(t *testing.T, conn *sql.DB, wantTotal, wantPerServer int, wantMin, wantMax int64) {
	t.Helper()

	var total, servers, minimumCount, maximumCount int
	var minimum, maximum int64
	err := conn.QueryRow(`
		SELECT COUNT(*), COUNT(DISTINCT server_id), MIN(minute_unix), MAX(minute_unix),
		       MIN(per_server_count), MAX(per_server_count)
		FROM metric_minutes
		JOIN (
			SELECT server_id AS grouped_server_id, COUNT(*) AS per_server_count
			FROM metric_minutes
			GROUP BY server_id
		) ON grouped_server_id = server_id
	`).Scan(&total, &servers, &minimum, &maximum, &minimumCount, &maximumCount)
	if err != nil {
		t.Fatal(err)
	}
	if total != wantTotal || servers != 10 || minimum != wantMin || maximum != wantMax || minimumCount != wantPerServer || maximumCount != wantPerServer {
		t.Fatalf(
			"metric_minutes shape = total %d, servers %d, minutes %d..%d, per-server %d..%d; want total %d, servers 10, minutes %d..%d, per-server %d",
			total, servers, minimum, maximum, minimumCount, maximumCount,
			wantTotal, wantMin, wantMax, wantPerServer,
		)
	}
}

func assertConsecutiveMinutes(t *testing.T, serverID int64, records []history.MinuteRecord, start time.Time, want int) {
	t.Helper()
	if len(records) != want {
		t.Fatalf("server %d rows = %d, want %d", serverID, len(records), want)
	}
	for offset, record := range records {
		wantMinute := start.Add(time.Duration(offset) * time.Minute).Unix()
		if record.ServerID != serverID || record.MinuteUnix != wantMinute {
			t.Fatalf("server %d row %d = server %d minute %d, want minute %d", serverID, offset, record.ServerID, record.MinuteUnix, wantMinute)
		}
	}
}
