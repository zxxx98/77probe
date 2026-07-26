package live_test

import (
	"reflect"
	"testing"
	"time"

	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

func TestUpsertStoresLatestOnlineSnapshot(t *testing.T) {
	store := live.NewStore()
	now := time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC)
	server := servers.Server{ID: 7, Name: "home-lab", Enabled: true}
	report := protocol.AgentReport{CollectedAtUnix: 123, AgentVersion: "1.2.3"}

	got := store.Upsert(server, report, now)

	if got.ServerID != server.ID || got.ServerName != server.Name || !got.Online || !got.LastReceivedAt.Equal(now) || got.Report.AgentVersion != "1.2.3" {
		t.Fatalf("snapshot = %+v", got)
	}
	stored, ok := store.Get(server.ID)
	if !ok || !reflect.DeepEqual(stored, got) {
		t.Fatalf("stored=%+v ok=%v", stored, ok)
	}
}

func TestStoreClonesReportDisksOnInsertion(t *testing.T) {
	store := live.NewStore()
	report := protocol.AgentReport{Disks: []protocol.DiskStats{{Mountpoint: "/original"}}}
	store.Upsert(servers.Server{ID: 7, Name: "home-lab"}, report, time.Now())

	report.Disks[0].Mountpoint = "/mutated"

	stored, _ := store.Get(7)
	if stored.Report.Disks[0].Mountpoint != "/original" {
		t.Fatalf("stored mountpoint=%q", stored.Report.Disks[0].Mountpoint)
	}
}

func TestStoreReturnsDetachedSnapshotDisks(t *testing.T) {
	newStoredSnapshot := func() (*live.Store, servers.Server, time.Time) {
		store := live.NewStore()
		server := servers.Server{ID: 7, Name: "home-lab"}
		receivedAt := time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC)
		store.Upsert(server, protocol.AgentReport{Disks: []protocol.DiskStats{{Mountpoint: "/original"}}}, receivedAt)
		return store, server, receivedAt
	}

	t.Run("upsert return", func(t *testing.T) {
		store := live.NewStore()
		returned := store.Upsert(servers.Server{ID: 7}, protocol.AgentReport{Disks: []protocol.DiskStats{{Mountpoint: "/original"}}}, time.Now())
		returned.Report.Disks[0].Mountpoint = "/mutated"
		stored, _ := store.Get(7)
		if stored.Report.Disks[0].Mountpoint != "/original" {
			t.Fatalf("stored mountpoint=%q", stored.Report.Disks[0].Mountpoint)
		}
	})

	t.Run("get return", func(t *testing.T) {
		store, _, _ := newStoredSnapshot()
		returned, _ := store.Get(7)
		returned.Report.Disks[0].Mountpoint = "/mutated"
		again, _ := store.Get(7)
		if again.Report.Disks[0].Mountpoint != "/original" {
			t.Fatalf("stored mountpoint=%q", again.Report.Disks[0].Mountpoint)
		}
	})

	t.Run("offline return", func(t *testing.T) {
		store, _, receivedAt := newStoredSnapshot()
		returned := store.MarkOffline(receivedAt.Add(time.Second))
		returned[0].Report.Disks[0].Mountpoint = "/mutated"
		again, _ := store.Get(7)
		if again.Report.Disks[0].Mountpoint != "/original" {
			t.Fatalf("stored mountpoint=%q", again.Report.Disks[0].Mountpoint)
		}
	})
}

func TestMarkOfflineAfterCutoff(t *testing.T) {
	store := live.NewStore()
	now := time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC)
	server := servers.Server{ID: 7, Name: "home-lab", Enabled: true}
	store.Upsert(server, protocol.AgentReport{}, now.Add(-31*time.Second))

	changed := store.MarkOffline(now.Add(-30 * time.Second))

	if len(changed) != 1 || changed[0].Online {
		t.Fatalf("changed = %+v", changed)
	}
	if again := store.MarkOffline(now.Add(-30 * time.Second)); len(again) != 0 {
		t.Fatalf("second sweep emitted duplicate transition: %+v", again)
	}
}

func TestMarkOfflineKeepsSnapshotAtCutoffOnline(t *testing.T) {
	store := live.NewStore()
	cutoff := time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC)
	server := servers.Server{ID: 7, Name: "home-lab", Enabled: true}
	store.Upsert(server, protocol.AgentReport{}, cutoff)

	if changed := store.MarkOffline(cutoff); len(changed) != 0 {
		t.Fatalf("changed = %+v", changed)
	}
}
