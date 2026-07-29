package agent

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

const testRouteTable = "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
	"eth0\t00000000\t0108A8C0\t0003\t0\t0\t100\t00000000\n"

func TestGopsutilSourceHostCPUAndMemory(t *testing.T) {
	source := newGopsutilSource(gopsutilDeps{
		openRoute: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(testRouteTable)), nil },
		hostInfo: func(context.Context) (*host.InfoStat, error) {
			return &host.InfoStat{Hostname: "tiny", OS: "linux", Platform: "debian", PlatformVersion: "12", KernelVersion: "6.1", KernelArch: "x86_64", BootTime: 100, Uptime: 200}, nil
		},
		cpuInfo: func(context.Context) ([]cpu.InfoStat, error) {
			return []cpu.InfoStat{{ModelName: "Tiny CPU"}}, nil
		},
		cpuCounts: func(context.Context, bool) (int, error) { return 4, nil },
		cpuPercent: func(context.Context, time.Duration, bool) ([]float64, error) {
			return []float64{23.5}, nil
		},
		loadAvg: func(context.Context) (*load.AvgStat, error) {
			return &load.AvgStat{Load1: 1, Load5: 2, Load15: 3}, nil
		},
		virtualMemory: func(context.Context) (*mem.VirtualMemoryStat, error) {
			return &mem.VirtualMemoryStat{Total: 1_000, Used: 600}, nil
		},
		swapMemory: func(context.Context) (*mem.SwapMemoryStat, error) {
			return &mem.SwapMemoryStat{Total: 500, Used: 100}, nil
		},
		netInterfaces: func(context.Context) (gnet.InterfaceStatList, error) {
			return gnet.InterfaceStatList{{Name: "eth0", Addrs: gnet.InterfaceAddrList{{Addr: "192.0.2.10/24"}, {Addr: "2001:db8::1/64"}}}}, nil
		},
	})

	hostStats, err := source.Host(context.Background())
	if err != nil {
		t.Fatalf("Host() error = %v", err)
	}
	if hostStats.Hostname != "tiny" || hostStats.Architecture != "x86_64" || hostStats.CPUModel != "Tiny CPU" || hostStats.CPUCores != 4 || hostStats.PrimaryIP != "192.0.2.10" {
		t.Fatalf("Host() = %+v", hostStats)
	}
	cpuStats, err := source.CPU(context.Background())
	if err != nil {
		t.Fatalf("CPU() error = %v", err)
	}
	if cpuStats.UsagePercent != 23.5 || cpuStats.Load1 != 1 || cpuStats.Load5 != 2 || cpuStats.Load15 != 3 {
		t.Fatalf("CPU() = %+v", cpuStats)
	}
	memoryStats, err := source.Memory(context.Background())
	if err != nil {
		t.Fatalf("Memory() error = %v", err)
	}
	if memoryStats.TotalBytes != 1_000 || memoryStats.UsedBytes != 600 || memoryStats.SwapTotalBytes != 500 || memoryStats.SwapUsedBytes != 100 {
		t.Fatalf("Memory() = %+v", memoryStats)
	}
}

func TestGopsutilSourceFiltersPersistentDisks(t *testing.T) {
	requestedAll := false
	var usageRequests []string
	source := newGopsutilSource(gopsutilDeps{
		partitions: func(_ context.Context, all bool) ([]disk.PartitionStat, error) {
			requestedAll = all
			return []disk.PartitionStat{
				{Mountpoint: "/", Fstype: "ext4"},
				{Mountpoint: "/data", Fstype: "xfs"},
				{Mountpoint: "/net/nfs", Fstype: "nfs"},
				{Mountpoint: "/net/cifs", Fstype: "cifs"},
				{Mountpoint: "/net/sshfs", Fstype: "fuse.sshfs"},
				{Mountpoint: "/boot", Fstype: "ext4"},
				{Mountpoint: "/boot/efi", Fstype: "vfat"},
				{Mountpoint: "/boot/firmware", Fstype: "vfat"},
				{Mountpoint: "/bootdata", Fstype: "ext4"},
				{Mountpoint: "/run/credentials/unit", Fstype: "ramfs"},
				{Mountpoint: "/sys/kernel/debug", Fstype: "debugfs"},
				{Mountpoint: "/run", Fstype: "tmpfs"},
				{Mountpoint: "/container", Fstype: "overlay"},
				{Mountpoint: "/sys/fs/cgroup", Fstype: "cgroup2"},
			}, nil
		},
		diskUsage: func(_ context.Context, mountpoint string) (*disk.UsageStat, error) {
			usageRequests = append(usageRequests, mountpoint)
			return &disk.UsageStat{Total: 1_000, Used: 500}, nil
		},
	})

	disks, err := source.PersistentDisks(context.Background())
	if err != nil {
		t.Fatalf("PersistentDisks() error = %v", err)
	}
	if !requestedAll {
		t.Fatal("PersistentDisks() requested partitions with all=false, want all=true")
	}
	gotMountpoints := make([]string, 0, len(disks))
	for _, disk := range disks {
		gotMountpoints = append(gotMountpoints, disk.Mountpoint)
	}
	const want = "/,/data,/net/nfs,/net/cifs,/net/sshfs,/bootdata"
	if got := strings.Join(gotMountpoints, ","); got != want {
		t.Fatalf("PersistentDisks() = %+v", disks)
	}
	if got := strings.Join(usageRequests, ","); got != want {
		t.Fatalf("diskUsage() mountpoints = %q, want %q", got, want)
	}
}

func TestGopsutilSourceSamplesAggregatedDiskAndDefaultRouteNetworkRates(t *testing.T) {
	now := time.Unix(100, 0)
	diskRead, diskWrite := uint64(1_000), uint64(2_000)
	netSent, netRecv := uint64(3_000), uint64(4_000)
	source := newGopsutilSource(gopsutilDeps{
		now:       func() time.Time { return now },
		openRoute: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(testRouteTable)), nil },
		diskIOCounters: func(context.Context, ...string) (map[string]disk.IOCountersStat, error) {
			return map[string]disk.IOCountersStat{"sda": {Name: "sda", ReadBytes: diskRead, WriteBytes: diskWrite}}, nil
		},
		isPhysicalBlockDevice: func(name string) bool { return name == "sda" },
		netIOCounters: func(context.Context, bool) ([]gnet.IOCountersStat, error) {
			return []gnet.IOCountersStat{{Name: "eth0", BytesSent: netSent, BytesRecv: netRecv}}, nil
		},
	})

	firstDisk, err := source.DiskIO(context.Background())
	if err != nil {
		t.Fatalf("first DiskIO() error = %v", err)
	}
	firstNetwork, err := source.DefaultRouteNetwork(context.Background())
	if err != nil {
		t.Fatalf("first DefaultRouteNetwork() error = %v", err)
	}
	if firstDisk.ReadBytesPerSecond != 0 || firstDisk.WriteBytesPerSecond != 0 || firstNetwork.UploadBytesPerSecond != 0 || firstNetwork.DownloadBytesPerSecond != 0 {
		t.Fatalf("first rates = disk %+v network %+v, want zero", firstDisk, firstNetwork)
	}

	now = now.Add(2 * time.Second)
	diskRead, diskWrite = 1_400, 2_600
	netSent, netRecv = 3_800, 5_000
	secondDisk, err := source.DiskIO(context.Background())
	if err != nil {
		t.Fatalf("second DiskIO() error = %v", err)
	}
	secondNetwork, err := source.DefaultRouteNetwork(context.Background())
	if err != nil {
		t.Fatalf("second DefaultRouteNetwork() error = %v", err)
	}
	if secondDisk.ReadBytesPerSecond != 200 || secondDisk.WriteBytesPerSecond != 300 {
		t.Fatalf("second DiskIO() = %+v", secondDisk)
	}
	if secondNetwork.Interface != "eth0" || secondNetwork.UploadBytesPerSecond != 400 || secondNetwork.DownloadBytesPerSecond != 500 || secondNetwork.TotalUploadBytes != 3_800 || secondNetwork.TotalDownloadBytes != 5_000 {
		t.Fatalf("second DefaultRouteNetwork() = %+v", secondNetwork)
	}
}

func TestGopsutilSourceReportsZeroRateWhenDefaultRouteInterfaceChanges(t *testing.T) {
	now := time.Unix(100, 0)
	interfaceName := "eth0"
	source := newGopsutilSource(gopsutilDeps{
		now: func() time.Time { return now },
		openRoute: func() (io.ReadCloser, error) {
			route := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
				interfaceName + "\t00000000\t0108A8C0\t0003\t0\t0\t100\t00000000\n"
			return io.NopCloser(strings.NewReader(route)), nil
		},
		netIOCounters: func(context.Context, bool) ([]gnet.IOCountersStat, error) {
			return []gnet.IOCountersStat{
				{Name: "eth0", BytesSent: 1_000, BytesRecv: 2_000},
				{Name: "eth1", BytesSent: 5_000, BytesRecv: 6_000},
			}, nil
		},
	})

	if _, err := source.DefaultRouteNetwork(context.Background()); err != nil {
		t.Fatalf("first DefaultRouteNetwork() error = %v", err)
	}
	now = now.Add(2 * time.Second)
	interfaceName = "eth1"
	stats, err := source.DefaultRouteNetwork(context.Background())
	if err != nil {
		t.Fatalf("second DefaultRouteNetwork() error = %v", err)
	}
	if stats.UploadBytesPerSecond != 0 || stats.DownloadBytesPerSecond != 0 {
		t.Fatalf("rates after interface change = (%d, %d), want zero", stats.UploadBytesPerSecond, stats.DownloadBytesPerSecond)
	}
}

func TestDefaultRouteInterfaceSelectsZeroDestinationAndMask(t *testing.T) {
	routes := strings.NewReader("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
		"eth1\t0008A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\n" +
		"eth0\t00000000\t0108A8C0\t0003\t0\t0\t100\t00000000\n")

	interfaceName, err := defaultRouteInterface(routes)
	if err != nil {
		t.Fatalf("defaultRouteInterface() error = %v", err)
	}
	if interfaceName != "eth0" {
		t.Fatalf("defaultRouteInterface() = %q, want eth0", interfaceName)
	}
}

func TestDefaultRouteInterfaceIgnoresDownRejectedAndMalformedCandidates(t *testing.T) {
	routes := strings.NewReader("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
		"down0\t00000000\t0108A8C0\t0000\t0\t0\t1\t00000000\n" +
		"reject0\t00000000\t0108A8C0\t0201\t0\t0\t2\t00000000\n" +
		"badflags0\t00000000\t0108A8C0\tZZZZ\t0\t0\t0\t00000000\n" +
		"badmetric0\t00000000\t0108A8C0\t0001\t0\t0\tinvalid\t00000000\n" +
		"eth0\t00000000\t0108A8C0\t0003\t0\t0\t50\t00000000\n")

	interfaceName, err := defaultRouteInterface(routes)
	if err != nil {
		t.Fatalf("defaultRouteInterface() error = %v", err)
	}
	if interfaceName != "eth0" {
		t.Fatalf("defaultRouteInterface() = %q, want eth0", interfaceName)
	}
}

func TestDefaultRouteInterfaceSelectsLowestMetricWithStableTie(t *testing.T) {
	routes := strings.NewReader("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
		"eth-high\t00000000\t0108A8C0\t0003\t0\t0\t200\t00000000\n" +
		"eth-low-first\t00000000\t0108A8C0\t0003\t0\t0\t10\t00000000\n" +
		"eth-low-second\t00000000\t0108A8C0\t0003\t0\t0\t10\t00000000\n")

	interfaceName, err := defaultRouteInterface(routes)
	if err != nil {
		t.Fatalf("defaultRouteInterface() error = %v", err)
	}
	if interfaceName != "eth-low-first" {
		t.Fatalf("defaultRouteInterface() = %q, want eth-low-first", interfaceName)
	}
}

func TestDefaultRouteInterfaceReturnsClearErrorWhenNoValidRouteExists(t *testing.T) {
	routes := strings.NewReader("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\n" +
		"down0\t00000000\t0108A8C0\t0000\t0\t0\t1\t00000000\n" +
		"reject0\t00000000\t0108A8C0\t0201\t0\t0\t2\t00000000\n")

	_, err := defaultRouteInterface(routes)
	if err == nil || !strings.Contains(err.Error(), "valid default route") {
		t.Fatalf("defaultRouteInterface() error = %v, want clear no-valid-route error", err)
	}
}

func TestPersistentFilesystemFilter(t *testing.T) {
	for _, filesystem := range []string{"tmpfs", "devtmpfs", "squashfs", "overlay", "proc", "sysfs", "cgroup", "cgroup2", "ramfs", "debugfs", "securityfs", "tracefs", "devpts", "nsfs", "pstore"} {
		if isPersistentFilesystem(filesystem) {
			t.Errorf("isPersistentFilesystem(%q) = true, want false", filesystem)
		}
	}
	for _, filesystem := range []string{"ext4", "xfs", "btrfs", "zfs"} {
		if !isPersistentFilesystem(filesystem) {
			t.Errorf("isPersistentFilesystem(%q) = false, want true", filesystem)
		}
	}
	if !shouldReportFilesystem("/bootdata", "ext4") {
		t.Fatal("/bootdata must remain reportable")
	}
	for _, mountpoint := range []string{"/boot", "/boot/efi", "/boot/firmware"} {
		if shouldReportFilesystem(mountpoint, "ext4") {
			t.Errorf("shouldReportFilesystem(%q, ext4) = true, want false", mountpoint)
		}
	}
}

func TestAggregatePhysicalDiskIOCounters(t *testing.T) {
	counters := map[string]disk.IOCountersStat{
		"sda":   {Name: "sda", ReadBytes: 100, WriteBytes: 200},
		"sdb":   {Name: "sdb", ReadBytes: 300, WriteBytes: 400},
		"sda1":  {Name: "sda1", ReadBytes: 50, WriteBytes: 60},
		"loop0": {Name: "loop0", ReadBytes: 900, WriteBytes: 900},
	}

	readBytes, writeBytes := aggregatePhysicalDiskIO(counters, func(name string) bool {
		return name == "sda" || name == "sdb"
	})
	if readBytes != 400 || writeBytes != 600 {
		t.Fatalf("aggregatePhysicalDiskIO() = (%d, %d), want (400, 600)", readBytes, writeBytes)
	}
}

func TestRateStateFirstSampleIsZero(t *testing.T) {
	var state rateState

	firstA, firstB := state.sample(time.Unix(100, 0), 1_000, 2_000)
	if firstA != 0 || firstB != 0 {
		t.Fatalf("first sample rates = (%d, %d), want (0, 0)", firstA, firstB)
	}
	secondA, secondB := state.sample(time.Unix(102, 0), 1_400, 2_600)
	if secondA != 200 || secondB != 300 {
		t.Fatalf("second sample rates = (%d, %d), want (200, 300)", secondA, secondB)
	}
}

func TestRateStateGuardsCounterResetAndNonPositiveElapsedTime(t *testing.T) {
	var state rateState
	state.sample(time.Unix(100, 0), 1_000, 2_000)

	resetA, resetB := state.sample(time.Unix(102, 0), 900, 2_400)
	if resetA != 0 || resetB != 200 {
		t.Fatalf("reset sample rates = (%d, %d), want (0, 200)", resetA, resetB)
	}
	nonPositiveA, nonPositiveB := state.sample(time.Unix(102, 0), 1_100, 2_600)
	if nonPositiveA != 0 || nonPositiveB != 0 {
		t.Fatalf("non-positive elapsed rates = (%d, %d), want (0, 0)", nonPositiveA, nonPositiveB)
	}
}
