package agent

import (
	"context"
	"fmt"
	"time"

	"probe.local/monitor/internal/protocol"
)

type ReportCollector interface {
	Collect(context.Context) (protocol.AgentReport, error)
}

type ReportSender interface {
	Send(context.Context, protocol.AgentReport) error
}

type Runner struct {
	Collector ReportCollector
	Client    ReportSender
	Wait      func(context.Context, time.Duration) error
}

var retryDelays = [...]time.Duration{
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	40 * time.Second,
	60 * time.Second,
}

func (r Runner) Run(ctx context.Context) error {
	if r.Collector == nil {
		return fmt.Errorf("collector is required")
	}
	if r.Client == nil {
		return fmt.Errorf("report client is required")
	}
	wait := r.Wait
	if wait == nil {
		wait = waitForContext
	}
	backoff := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		report, err := r.Collector.Collect(ctx)
		if err == nil {
			err = r.Client.Send(ctx, report)
		}

		delay := retryDelays[backoff]
		if err == nil {
			delay = 5 * time.Second
			backoff = 0
		} else if backoff < len(retryDelays)-1 {
			backoff++
		}
		if err := wait(ctx, delay); err != nil {
			return err
		}
	}
}

func waitForContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
