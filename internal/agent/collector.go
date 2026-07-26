package agent

import (
	"context"
	"fmt"
	"time"

	"probe.local/monitor/internal/protocol"
)

type Collector struct {
	Source       Source
	AgentVersion string
	Now          func() time.Time
}

func (c Collector) Collect(ctx context.Context) (protocol.AgentReport, error) {
	if c.Source == nil {
		return protocol.AgentReport{}, fmt.Errorf("agent source is required")
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	host, err := c.Source.Host(ctx)
	if err != nil {
		return protocol.AgentReport{}, fmt.Errorf("collect host: %w", err)
	}
	cpu, err := c.Source.CPU(ctx)
	if err != nil {
		return protocol.AgentReport{}, fmt.Errorf("collect cpu: %w", err)
	}
	memory, err := c.Source.Memory(ctx)
	if err != nil {
		return protocol.AgentReport{}, fmt.Errorf("collect memory: %w", err)
	}
	disks, err := c.Source.PersistentDisks(ctx)
	if err != nil {
		return protocol.AgentReport{}, fmt.Errorf("collect disks: %w", err)
	}
	diskIO, err := c.Source.DiskIO(ctx)
	if err != nil {
		return protocol.AgentReport{}, fmt.Errorf("collect disk io: %w", err)
	}
	network, err := c.Source.DefaultRouteNetwork(ctx)
	if err != nil {
		return protocol.AgentReport{}, fmt.Errorf("collect network: %w", err)
	}
	return protocol.AgentReport{
		CollectedAtUnix: now().Unix(),
		AgentVersion:    c.AgentVersion,
		Host:            host,
		CPU:             cpu,
		Memory:          memory,
		Disks:           disks,
		DiskIO:          diskIO,
		Network:         network,
	}, nil
}
