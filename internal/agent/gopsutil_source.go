package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"

	"probe.local/monitor/internal/protocol"
)

type gopsutilDeps struct {
	now                   func() time.Time
	openRoute             func() (io.ReadCloser, error)
	hostInfo              func(context.Context) (*host.InfoStat, error)
	cpuInfo               func(context.Context) ([]cpu.InfoStat, error)
	cpuCounts             func(context.Context, bool) (int, error)
	cpuPercent            func(context.Context, time.Duration, bool) ([]float64, error)
	loadAvg               func(context.Context) (*load.AvgStat, error)
	virtualMemory         func(context.Context) (*mem.VirtualMemoryStat, error)
	swapMemory            func(context.Context) (*mem.SwapMemoryStat, error)
	partitions            func(context.Context, bool) ([]disk.PartitionStat, error)
	diskUsage             func(context.Context, string) (*disk.UsageStat, error)
	diskIOCounters        func(context.Context, ...string) (map[string]disk.IOCountersStat, error)
	isPhysicalBlockDevice func(string) bool
	netIOCounters         func(context.Context, bool) ([]gnet.IOCountersStat, error)
	netInterfaces         func(context.Context) (gnet.InterfaceStatList, error)
}

type GopsutilSource struct {
	deps             gopsutilDeps
	mu               sync.Mutex
	diskRate         rateState
	networkInterface string
	networkRate      rateState
}

func NewGopsutilSource() *GopsutilSource {
	return newGopsutilSource(gopsutilDeps{
		now:                   time.Now,
		openRoute:             func() (io.ReadCloser, error) { return os.Open("/proc/net/route") },
		hostInfo:              host.InfoWithContext,
		cpuInfo:               cpu.InfoWithContext,
		cpuCounts:             cpu.CountsWithContext,
		cpuPercent:            cpu.PercentWithContext,
		loadAvg:               load.AvgWithContext,
		virtualMemory:         mem.VirtualMemoryWithContext,
		swapMemory:            mem.SwapMemoryWithContext,
		partitions:            disk.PartitionsWithContext,
		diskUsage:             disk.UsageWithContext,
		diskIOCounters:        disk.IOCountersWithContext,
		isPhysicalBlockDevice: linuxPhysicalBlockDevice,
		netIOCounters:         gnet.IOCountersWithContext,
		netInterfaces:         gnet.InterfacesWithContext,
	})
}

func newGopsutilSource(deps gopsutilDeps) *GopsutilSource {
	if deps.now == nil {
		deps.now = time.Now
	}
	return &GopsutilSource{deps: deps}
}

func (s *GopsutilSource) Host(ctx context.Context) (protocol.HostInfo, error) {
	info, err := s.deps.hostInfo(ctx)
	if err != nil {
		return protocol.HostInfo{}, fmt.Errorf("read host info: %w", err)
	}
	cpuInfo, err := s.deps.cpuInfo(ctx)
	if err != nil {
		return protocol.HostInfo{}, fmt.Errorf("read cpu info: %w", err)
	}
	cores, err := s.deps.cpuCounts(ctx, true)
	if err != nil {
		return protocol.HostInfo{}, fmt.Errorf("read cpu count: %w", err)
	}
	primaryIP, err := s.primaryIP(ctx)
	if err != nil {
		return protocol.HostInfo{}, err
	}
	model := ""
	if len(cpuInfo) > 0 {
		model = cpuInfo[0].ModelName
	}
	return protocol.HostInfo{
		Hostname:        info.Hostname,
		OS:              info.OS,
		Platform:        info.Platform,
		PlatformVersion: info.PlatformVersion,
		KernelVersion:   info.KernelVersion,
		Architecture:    info.KernelArch,
		CPUModel:        model,
		CPUCores:        cores,
		PrimaryIP:       primaryIP,
		BootTimeUnix:    int64(info.BootTime),
		UptimeSeconds:   info.Uptime,
	}, nil
}

func (s *GopsutilSource) CPU(ctx context.Context) (protocol.CPUStats, error) {
	percent, err := s.deps.cpuPercent(ctx, 0, false)
	if err != nil {
		return protocol.CPUStats{}, fmt.Errorf("read cpu usage: %w", err)
	}
	if len(percent) == 0 {
		return protocol.CPUStats{}, fmt.Errorf("read cpu usage: no samples")
	}
	average, err := s.deps.loadAvg(ctx)
	if err != nil {
		return protocol.CPUStats{}, fmt.Errorf("read load average: %w", err)
	}
	return protocol.CPUStats{UsagePercent: percent[0], Load1: average.Load1, Load5: average.Load5, Load15: average.Load15}, nil
}

func (s *GopsutilSource) Memory(ctx context.Context) (protocol.MemoryStats, error) {
	virtual, err := s.deps.virtualMemory(ctx)
	if err != nil {
		return protocol.MemoryStats{}, fmt.Errorf("read memory: %w", err)
	}
	swap, err := s.deps.swapMemory(ctx)
	if err != nil {
		return protocol.MemoryStats{}, fmt.Errorf("read swap: %w", err)
	}
	return protocol.MemoryStats{
		TotalBytes: virtual.Total, UsedBytes: virtual.Used,
		SwapTotalBytes: swap.Total, SwapUsedBytes: swap.Used,
	}, nil
}

func (s *GopsutilSource) PersistentDisks(ctx context.Context) ([]protocol.DiskStats, error) {
	partitions, err := s.deps.partitions(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("read partitions: %w", err)
	}
	disks := make([]protocol.DiskStats, 0, len(partitions))
	for _, partition := range partitions {
		if !isPersistentFilesystem(partition.Fstype) {
			continue
		}
		usage, err := s.deps.diskUsage(ctx, partition.Mountpoint)
		if err != nil {
			return nil, fmt.Errorf("read disk usage for %s: %w", partition.Mountpoint, err)
		}
		disks = append(disks, protocol.DiskStats{Mountpoint: partition.Mountpoint, TotalBytes: usage.Total, UsedBytes: usage.Used})
	}
	return disks, nil
}

func (s *GopsutilSource) DiskIO(ctx context.Context) (protocol.DiskIOStats, error) {
	counters, err := s.deps.diskIOCounters(ctx)
	if err != nil {
		return protocol.DiskIOStats{}, fmt.Errorf("read disk io: %w", err)
	}
	readBytes, writeBytes := aggregatePhysicalDiskIO(counters, s.deps.isPhysicalBlockDevice)
	s.mu.Lock()
	readRate, writeRate := s.diskRate.sample(s.deps.now(), readBytes, writeBytes)
	s.mu.Unlock()
	return protocol.DiskIOStats{ReadBytesPerSecond: readRate, WriteBytesPerSecond: writeRate}, nil
}

func (s *GopsutilSource) DefaultRouteNetwork(ctx context.Context) (protocol.NetworkStats, error) {
	interfaceName, err := s.defaultRouteName()
	if err != nil {
		return protocol.NetworkStats{}, err
	}
	counters, err := s.deps.netIOCounters(ctx, true)
	if err != nil {
		return protocol.NetworkStats{}, fmt.Errorf("read network io: %w", err)
	}
	for _, counter := range counters {
		if counter.Name != interfaceName {
			continue
		}
		s.mu.Lock()
		if s.networkInterface != interfaceName {
			s.networkInterface = interfaceName
			s.networkRate = rateState{}
		}
		uploadRate, downloadRate := s.networkRate.sample(s.deps.now(), counter.BytesSent, counter.BytesRecv)
		s.mu.Unlock()
		return protocol.NetworkStats{
			Interface: interfaceName, UploadBytesPerSecond: uploadRate, DownloadBytesPerSecond: downloadRate,
			TotalUploadBytes: counter.BytesSent, TotalDownloadBytes: counter.BytesRecv,
		}, nil
	}
	return protocol.NetworkStats{}, fmt.Errorf("default route interface %q has no counters", interfaceName)
}

func (s *GopsutilSource) defaultRouteName() (string, error) {
	route, err := s.deps.openRoute()
	if err != nil {
		return "", fmt.Errorf("open route table: %w", err)
	}
	defer route.Close()
	interfaceName, err := defaultRouteInterface(route)
	if err != nil {
		return "", err
	}
	return interfaceName, nil
}

func (s *GopsutilSource) primaryIP(ctx context.Context) (string, error) {
	interfaceName, err := s.defaultRouteName()
	if err != nil {
		return "", err
	}
	interfaces, err := s.deps.netInterfaces(ctx)
	if err != nil {
		return "", fmt.Errorf("read network interfaces: %w", err)
	}
	for _, networkInterface := range interfaces {
		if networkInterface.Name != interfaceName {
			continue
		}
		for _, address := range networkInterface.Addrs {
			ip, _, err := net.ParseCIDR(address.Addr)
			if err == nil && ip.To4() != nil {
				return ip.String(), nil
			}
		}
		return "", fmt.Errorf("default route interface %q has no IPv4 address", interfaceName)
	}
	return "", fmt.Errorf("default route interface %q not found", interfaceName)
}

func linuxPhysicalBlockDevice(name string) bool {
	if filepath.Base(name) != name || name == "." || name == "" {
		return false
	}
	_, err := os.Stat(filepath.Join("/sys/class/block", name, "device"))
	return err == nil
}

var temporaryFilesystems = map[string]struct{}{
	"tmpfs": {}, "devtmpfs": {}, "squashfs": {}, "overlay": {},
	"proc": {}, "sysfs": {}, "cgroup": {}, "cgroup2": {},
}

func isPersistentFilesystem(filesystem string) bool {
	_, temporary := temporaryFilesystems[strings.ToLower(filesystem)]
	return !temporary
}

func defaultRouteInterface(reader io.Reader) (string, error) {
	const (
		routeFlagUp     = 0x1
		routeFlagReject = 0x200
	)
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read route header: %w", err)
		}
		return "", fmt.Errorf("route table is empty")
	}
	var selectedInterface string
	var selectedMetric uint64
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(fields[3], 16, 64)
		if err != nil || flags&routeFlagUp == 0 || flags&routeFlagReject != 0 {
			continue
		}
		metric, err := strconv.ParseUint(fields[6], 10, 64)
		if err != nil {
			continue
		}
		if selectedInterface == "" || metric < selectedMetric {
			selectedInterface = fields[0]
			selectedMetric = metric
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read route table: %w", err)
	}
	if selectedInterface == "" {
		return "", fmt.Errorf("valid default route not found")
	}
	return selectedInterface, nil
}

func aggregatePhysicalDiskIO(counters map[string]disk.IOCountersStat, physical func(string) bool) (uint64, uint64) {
	var readBytes uint64
	var writeBytes uint64
	for name, counter := range counters {
		if !physical(name) {
			continue
		}
		readBytes += counter.ReadBytes
		writeBytes += counter.WriteBytes
	}
	return readBytes, writeBytes
}

type rateState struct {
	initialized bool
	at          time.Time
	first       uint64
	second      uint64
}

func (s *rateState) sample(now time.Time, first, second uint64) (uint64, uint64) {
	if !s.initialized {
		s.initialized = true
		s.at = now
		s.first = first
		s.second = second
		return 0, 0
	}
	elapsed := now.Sub(s.at)
	previousFirst := s.first
	previousSecond := s.second
	s.at = now
	s.first = first
	s.second = second
	if elapsed <= 0 {
		return 0, 0
	}
	return bytesPerSecond(previousFirst, first, elapsed), bytesPerSecond(previousSecond, second, elapsed)
}

func bytesPerSecond(previous, current uint64, elapsed time.Duration) uint64 {
	if current < previous || elapsed <= 0 {
		return 0
	}
	return uint64(float64(current-previous) / elapsed.Seconds())
}
