package alerting

import (
	"math"

	"probe.local/monitor/internal/live"
)

func MetricValue(snapshot live.Snapshot, metric Metric) float64 {
	switch metric {
	case MetricOffline:
		if snapshot.Online {
			return 0
		}
		return 1
	case MetricCPUUsage:
		return snapshot.Report.CPU.UsagePercent
	case MetricMemoryUsage:
		if snapshot.Report.Memory.TotalBytes == 0 {
			return 0
		}
		return float64(snapshot.Report.Memory.UsedBytes) / float64(snapshot.Report.Memory.TotalBytes) * 100
	case MetricDiskUsage:
		value := 0.0
		for _, disk := range snapshot.Report.Disks {
			if disk.TotalBytes == 0 {
				continue
			}
			value = max(value, float64(disk.UsedBytes)/float64(disk.TotalBytes)*100)
		}
		return value
	case MetricDiskFreeBytes:
		value := math.Inf(1)
		for _, disk := range snapshot.Report.Disks {
			free := float64(disk.TotalBytes - disk.UsedBytes)
			value = min(value, free)
		}
		if math.IsInf(value, 1) {
			return 0
		}
		return value
	default:
		return 0
	}
}

func breached(value float64, rule Rule) bool {
	switch rule.Operator {
	case OperatorGreaterThan:
		return value > rule.Threshold
	case OperatorLessThan:
		return value < rule.Threshold
	default:
		return false
	}
}
