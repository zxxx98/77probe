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
	flushing    bool
}

type Aggregator struct {
	mu      sync.Mutex
	writer  Writer
	buckets map[bucketKey]*bucket
}

func NewAggregator(writer Writer) *Aggregator {
	return &Aggregator{
		writer:  writer,
		buckets: make(map[bucketKey]*bucket),
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
	entry.accumulator.Add(report)
	entry.revision++
}

func (a *Aggregator) FlushBefore(ctx context.Context, minute time.Time) error {
	type candidate struct {
		key      bucketKey
		revision uint64
		record   MinuteRecord
	}

	a.mu.Lock()
	keys := make([]bucketKey, 0, len(a.buckets))
	for key, entry := range a.buckets {
		if !entry.flushing && time.Unix(key.MinuteUnix, 0).Before(minute) {
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
		entry.flushing = true
		candidates = append(candidates, candidate{
			key:      key,
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
		if err == nil && entry.revision == candidate.revision {
			delete(a.buckets, candidate.key)
		} else {
			entry.flushing = false
		}
		a.mu.Unlock()

		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
