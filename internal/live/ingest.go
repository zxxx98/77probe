package live

import (
	"errors"
	"math"
	"strings"

	"probe.local/monitor/internal/protocol"
)

var errInvalidReport = errors.New("invalid agent report")

func validateReport(report protocol.AgentReport) error {
	if strings.TrimSpace(report.Host.Hostname) == "" || len(report.Disks) == 0 || report.CollectedAtUnix <= 0 {
		return errInvalidReport
	}
	if !finite(report.CPU.UsagePercent) || report.CPU.UsagePercent < 0 || report.CPU.UsagePercent > 100 ||
		!finite(report.CPU.Load1) || !finite(report.CPU.Load5) || !finite(report.CPU.Load15) {
		return errInvalidReport
	}
	if report.Memory.UsedBytes > report.Memory.TotalBytes || report.Memory.SwapUsedBytes > report.Memory.SwapTotalBytes {
		return errInvalidReport
	}
	for _, disk := range report.Disks {
		if strings.TrimSpace(disk.Mountpoint) == "" || disk.UsedBytes > disk.TotalBytes {
			return errInvalidReport
		}
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
