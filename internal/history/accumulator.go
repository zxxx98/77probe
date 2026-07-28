package history

import (
	"sort"

	"probe.local/monitor/internal/protocol"
)

type numeric struct {
	Count uint64
	Sum   float64
	Max   float64
}

func (n *numeric) add(value float64) {
	n.Count++
	n.Sum += value
	if n.Count == 1 || value > n.Max {
		n.Max = value
	}
}

func (n numeric) pair() Pair {
	if n.Count == 0 {
		return Pair{}
	}
	return Pair{
		Average: n.Sum / float64(n.Count),
		Maximum: n.Max,
	}
}

type diskAccumulator struct {
	Usage      numeric
	TotalBytes uint64
	UsedBytes  uint64
}

type Accumulator struct {
	cpuUsage      numeric
	load1         numeric
	load5         numeric
	load15        numeric
	memoryUsage   numeric
	swapUsage     numeric
	disks         map[string]*diskAccumulator
	diskReadBPS   numeric
	diskWriteBPS  numeric
	uploadBPS     numeric
	downloadBPS   numeric
	totalUpload   uint64
	totalDownload uint64
}

func (a *Accumulator) Add(report protocol.AgentReport) {
	a.cpuUsage.add(report.CPU.UsagePercent)
	a.load1.add(report.CPU.Load1)
	a.load5.add(report.CPU.Load5)
	a.load15.add(report.CPU.Load15)
	a.memoryUsage.add(usagePercent(report.Memory.UsedBytes, report.Memory.TotalBytes))
	a.swapUsage.add(usagePercent(report.Memory.SwapUsedBytes, report.Memory.SwapTotalBytes))

	if a.disks == nil {
		a.disks = make(map[string]*diskAccumulator)
	}
	for _, disk := range report.Disks {
		entry := a.disks[disk.Mountpoint]
		if entry == nil {
			entry = &diskAccumulator{}
			a.disks[disk.Mountpoint] = entry
		}
		entry.Usage.add(usagePercent(disk.UsedBytes, disk.TotalBytes))
		entry.TotalBytes = disk.TotalBytes
		entry.UsedBytes = disk.UsedBytes
	}

	a.diskReadBPS.add(float64(report.DiskIO.ReadBytesPerSecond))
	a.diskWriteBPS.add(float64(report.DiskIO.WriteBytesPerSecond))
	a.uploadBPS.add(float64(report.Network.UploadBytesPerSecond))
	a.downloadBPS.add(float64(report.Network.DownloadBytesPerSecond))
	a.totalUpload = report.Network.TotalUploadBytes
	a.totalDownload = report.Network.TotalDownloadBytes
}

func (a *Accumulator) Finish(serverID, minuteUnix int64) MinuteRecord {
	mountpoints := make([]string, 0, len(a.disks))
	for mountpoint := range a.disks {
		mountpoints = append(mountpoints, mountpoint)
	}
	sort.Strings(mountpoints)

	disks := make([]DiskMinute, 0, len(mountpoints))
	for _, mountpoint := range mountpoints {
		entry := a.disks[mountpoint]
		disks = append(disks, DiskMinute{
			Mountpoint: mountpoint,
			Usage:      entry.Usage.pair(),
			TotalBytes: entry.TotalBytes,
			UsedBytes:  entry.UsedBytes,
		})
	}

	return MinuteRecord{
		ServerID:   serverID,
		MinuteUnix: minuteUnix,
		Payload: MinutePayload{
			CPUUsage:      a.cpuUsage.pair(),
			Load1:         a.load1.pair(),
			Load5:         a.load5.pair(),
			Load15:        a.load15.pair(),
			MemoryUsage:   a.memoryUsage.pair(),
			SwapUsage:     a.swapUsage.pair(),
			Disks:         disks,
			DiskReadBPS:   a.diskReadBPS.pair(),
			DiskWriteBPS:  a.diskWriteBPS.pair(),
			UploadBPS:     a.uploadBPS.pair(),
			DownloadBPS:   a.downloadBPS.pair(),
			TotalUpload:   a.totalUpload,
			TotalDownload: a.totalDownload,
		},
	}
}

func usagePercent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}
