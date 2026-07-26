package agent

import (
	"context"

	"probe.local/monitor/internal/protocol"
)

type Source interface {
	Host(context.Context) (protocol.HostInfo, error)
	CPU(context.Context) (protocol.CPUStats, error)
	Memory(context.Context) (protocol.MemoryStats, error)
	PersistentDisks(context.Context) ([]protocol.DiskStats, error)
	DiskIO(context.Context) (protocol.DiskIOStats, error)
	DefaultRouteNetwork(context.Context) (protocol.NetworkStats, error)
}
