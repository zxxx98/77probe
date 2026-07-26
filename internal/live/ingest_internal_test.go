package live

import (
	"math"
	"testing"

	"probe.local/monitor/internal/protocol"
)

func TestValidateReportRejectsNonFiniteCPUAndLoad(t *testing.T) {
	report := protocol.AgentReport{
		CollectedAtUnix: 1,
		Host:            protocol.HostInfo{Hostname: "host"},
		Disks:           []protocol.DiskStats{{Mountpoint: "/"}},
	}
	for _, mutate := range []func(*protocol.AgentReport){
		func(r *protocol.AgentReport) { r.CPU.UsagePercent = math.NaN() },
		func(r *protocol.AgentReport) { r.CPU.Load1 = math.Inf(1) },
		func(r *protocol.AgentReport) { r.CPU.Load5 = math.Inf(-1) },
		func(r *protocol.AgentReport) { r.CPU.Load15 = math.NaN() },
	} {
		candidate := report
		mutate(&candidate)
		if validateReport(candidate) == nil {
			t.Fatalf("accepted CPU stats: %+v", candidate.CPU)
		}
	}
}
