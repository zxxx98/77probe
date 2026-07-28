package history

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

const (
	flushInterval          = 5 * time.Second
	retentionInterval      = 6 * time.Hour
	historyRetention       = 30 * 24 * time.Hour
	defaultShutdownTimeout = 5 * time.Second
)

type MinuteFlusher interface {
	FlushBefore(context.Context, time.Time) error
}

type RetentionStore interface {
	DeleteBefore(context.Context, int64) (int64, error)
}

type jobTicker interface {
	C() <-chan time.Time
	Stop()
}

type realJobTicker struct {
	*time.Ticker
}

func (t realJobTicker) C() <-chan time.Time {
	return t.Ticker.C
}

type Jobs struct {
	flusher         MinuteFlusher
	retention       RetentionStore
	now             func() time.Time
	newTicker       func(time.Duration) jobTicker
	reportError     func(error)
	shutdownTimeout time.Duration
}

type jobsConfig struct {
	now             func() time.Time
	newTicker       func(time.Duration) jobTicker
	reportError     func(error)
	shutdownTimeout time.Duration
}

func NewJobs(flusher MinuteFlusher, retention RetentionStore) *Jobs {
	if flusher == nil || retention == nil {
		panic("history jobs require flusher and retention store")
	}
	return newJobs(flusher, retention, jobsConfig{})
}

func newJobs(flusher MinuteFlusher, retention RetentionStore, config jobsConfig) *Jobs {
	if config.now == nil {
		config.now = time.Now
	}
	if config.newTicker == nil {
		config.newTicker = func(interval time.Duration) jobTicker {
			return realJobTicker{Ticker: time.NewTicker(interval)}
		}
	}
	if config.reportError == nil {
		config.reportError = func(err error) {
			log.Printf("history background job: %v", err)
		}
	}
	if config.shutdownTimeout <= 0 {
		config.shutdownTimeout = defaultShutdownTimeout
	}
	return &Jobs{
		flusher:         flusher,
		retention:       retention,
		now:             config.now,
		newTicker:       config.newTicker,
		reportError:     config.reportError,
		shutdownTimeout: config.shutdownTimeout,
	}
}

func (j *Jobs) Run(ctx context.Context) {
	j.runRetention(ctx)

	flushTicker := j.newTicker(flushInterval)
	retentionTicker := j.newTicker(retentionInterval)
	defer flushTicker.Stop()
	defer retentionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			j.flushOnShutdown()
			return
		case <-flushTicker.C():
			j.flush(ctx, "flush completed history minutes", true)
		case <-retentionTicker.C():
			j.runRetention(ctx)
		}
	}
}

func (j *Jobs) flush(ctx context.Context, operation string, suppressContextError bool) {
	boundary := j.now().UTC().Truncate(time.Minute)
	if err := j.flusher.FlushBefore(ctx, boundary); err != nil {
		if suppressContextError && ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return
		}
		j.reportError(fmt.Errorf("%s: %w", operation, err))
	}
}

func (j *Jobs) flushOnShutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), j.shutdownTimeout)
	defer cancel()
	j.flush(ctx, "flush completed history minutes on shutdown", false)
}

func (j *Jobs) runRetention(ctx context.Context) {
	cutoffUnix := j.now().Add(-historyRetention).UTC().Truncate(time.Minute).Unix()
	if _, err := j.retention.DeleteBefore(ctx, cutoffUnix); err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return
		}
		j.reportError(fmt.Errorf("delete expired history minutes: %w", err))
	}
}
