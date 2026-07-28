package history

import (
	"context"
	"sort"
	"sync"
	"time"

	"probe.local/monitor/internal/protocol"
)

type Writer interface {
	UpsertMinute(context.Context, MinuteRecord) error
}

type bucketKey struct {
	ServerID   int64
	MinuteUnix int64
}

type bucket struct {
	accumulator Accumulator
	revision    uint64
}

type Aggregator struct {
	flushGate chan struct{}
	mu        sync.Mutex
	writer    Writer
	buckets   map[bucketKey]*bucket
}

func NewAggregator(writer Writer) *Aggregator {
	return &Aggregator{
		flushGate: make(chan struct{}, 1),
		writer:    writer,
		buckets:   make(map[bucketKey]*bucket),
	}
}

func (a *Aggregator) Accept(serverID int64, report protocol.AgentReport, receivedAt time.Time) {
	key := bucketKey{
		ServerID:   serverID,
		MinuteUnix: receivedAt.UTC().Truncate(time.Minute).Unix(),
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	entry := a.buckets[key]
	if entry == nil {
		entry = &bucket{}
		a.buckets[key] = entry
	}
	entry.accumulator.addAt(report, receivedAt)
	entry.revision++
}

func (a *Aggregator) RemoveServer(serverID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for key := range a.buckets {
		if key.ServerID == serverID {
			delete(a.buckets, key)
		}
	}
}

func (a *Aggregator) FlushBefore(ctx context.Context, minute time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case a.flushGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-a.flushGate }()
	if err := ctx.Err(); err != nil {
		return err
	}

	type candidate struct {
		key      bucketKey
		bucket   *bucket
		revision uint64
		record   MinuteRecord
	}

	a.mu.Lock()
	keys := make([]bucketKey, 0, len(a.buckets))
	for key := range a.buckets {
		if time.Unix(key.MinuteUnix, 0).Before(minute) {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ServerID != keys[j].ServerID {
			return keys[i].ServerID < keys[j].ServerID
		}
		return keys[i].MinuteUnix < keys[j].MinuteUnix
	})

	candidates := make([]candidate, 0, len(keys))
	for _, key := range keys {
		entry := a.buckets[key]
		candidates = append(candidates, candidate{
			key:      key,
			bucket:   entry,
			revision: entry.revision,
			record:   entry.accumulator.Finish(key.ServerID, key.MinuteUnix),
		})
	}
	a.mu.Unlock()

	var firstErr error
	for _, candidate := range candidates {
		err := a.writer.UpsertMinute(ctx, candidate.record)

		a.mu.Lock()
		entry := a.buckets[candidate.key]
		if entry != candidate.bucket {
			err = nil
		} else if err == nil && entry.revision == candidate.revision {
			delete(a.buckets, candidate.key)
		}
		a.mu.Unlock()

		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
