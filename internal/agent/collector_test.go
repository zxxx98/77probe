package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"probe.local/monitor/internal/protocol"
)

type fixtureSource struct {
	host       protocol.HostInfo
	cpu        protocol.CPUStats
	memory     protocol.MemoryStats
	disks      []protocol.DiskStats
	diskIO     protocol.DiskIOStats
	network    protocol.NetworkStats
	networkErr error
}

func (s fixtureSource) Host(context.Context) (protocol.HostInfo, error) { return s.host, nil }
func (s fixtureSource) CPU(context.Context) (protocol.CPUStats, error)  { return s.cpu, nil }
func (s fixtureSource) Memory(context.Context) (protocol.MemoryStats, error) {
	return s.memory, nil
}
func (s fixtureSource) PersistentDisks(context.Context) ([]protocol.DiskStats, error) {
	return s.disks, nil
}
func (s fixtureSource) DiskIO(context.Context) (protocol.DiskIOStats, error) {
	return s.diskIO, nil
}
func (s fixtureSource) DefaultRouteNetwork(context.Context) (protocol.NetworkStats, error) {
	return s.network, s.networkErr
}

func TestCollectorReturnsNoPartialReportWhenSourceFails(t *testing.T) {
	source := fixtureSource{
		host:       protocol.HostInfo{Hostname: "must-not-leak"},
		networkErr: errors.New("route unavailable"),
	}

	report, err := (Collector{Source: source, AgentVersion: "test", Now: time.Now}).Collect(context.Background())
	if err == nil {
		t.Fatal("Collect() error = nil, want source error")
	}
	if !reflect.DeepEqual(report, protocol.AgentReport{}) {
		t.Fatalf("Collect() report = %+v, want zero report", report)
	}
}

func TestCollectorUsesPersistentMountsAndDefaultRoute(t *testing.T) {
	source := fixtureSource{
		host:   protocol.HostInfo{Hostname: "tiny-host"},
		cpu:    protocol.CPUStats{UsagePercent: 12.5},
		memory: protocol.MemoryStats{TotalBytes: 8_000, UsedBytes: 4_000},
		disks: []protocol.DiskStats{
			{Mountpoint: "/", TotalBytes: 1_000, UsedBytes: 500},
			{Mountpoint: "/data", TotalBytes: 2_000, UsedBytes: 1_500},
		},
		diskIO: protocol.DiskIOStats{ReadBytesPerSecond: 101, WriteBytesPerSecond: 202},
		network: protocol.NetworkStats{
			Interface:              "eth0",
			UploadBytesPerSecond:   303,
			DownloadBytesPerSecond: 404,
			TotalUploadBytes:       505,
			TotalDownloadBytes:     606,
		},
	}
	now := time.Unix(1_700_000_000, 0)

	report, err := (Collector{Source: source, AgentVersion: "test", Now: func() time.Time { return now }}).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(report.Disks) != 2 {
		t.Fatalf("len(report.Disks) = %d, want 2", len(report.Disks))
	}
	for _, disk := range report.Disks {
		if disk.Mountpoint == "/run" {
			t.Fatal("temporary mount /run was included")
		}
	}
	highest := report.Disks[0]
	for _, disk := range report.Disks[1:] {
		if float64(disk.UsedBytes)/float64(disk.TotalBytes) > float64(highest.UsedBytes)/float64(highest.TotalBytes) {
			highest = disk
		}
	}
	if highest.Mountpoint != "/data" {
		t.Fatalf("highest-usage mount = %q, want /data", highest.Mountpoint)
	}
	if report.Network.Interface != "eth0" {
		t.Fatalf("network interface = %q, want eth0", report.Network.Interface)
	}
	if report.CollectedAtUnix != now.Unix() || report.AgentVersion != "test" {
		t.Fatalf("collection metadata = (%d, %q), want (%d, test)", report.CollectedAtUnix, report.AgentVersion, now.Unix())
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, decimal := range []string{"101.0", "202.0", "303.0", "404.0", "505.0", "606.0"} {
		if strings.Contains(string(encoded), decimal) {
			t.Fatalf("byte value was encoded as a decimal: %s", encoded)
		}
	}
}
