package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	maxHistoryQuerySeconds = int64(30 * 24 * 60 * 60)
	maxHistoryQueryRows    = 43_201
	historyQueryPageRows   = 256
)

var ErrInvalidRange = errors.New("invalid history range")

type InvalidRangeError struct {
	FromUnix int64
	ToUnix   int64
}

func (e *InvalidRangeError) Error() string {
	return fmt.Sprintf("%v: from %d to %d", ErrInvalidRange, e.FromUnix, e.ToUnix)
}

func (e *InvalidRangeError) Unwrap() error {
	return ErrInvalidRange
}

type Store struct {
	conn          *sql.DB
	now           func() time.Time
	decodePayload func([]byte, *MinutePayload) error
}

func NewStore(conn *sql.DB) *Store {
	if conn == nil {
		panic("history store requires database connection")
	}
	return &Store{
		conn: conn,
		now:  time.Now,
		decodePayload: func(data []byte, payload *MinutePayload) error {
			return json.Unmarshal(data, payload)
		},
	}
}

func (s *Store) UpsertMinute(ctx context.Context, record MinuteRecord) error {
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return fmt.Errorf("marshal minute payload: %w", err)
	}
	_, err = s.conn.ExecContext(ctx, `
		INSERT INTO metric_minutes(server_id, minute_unix, payload_json, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(server_id, minute_unix) DO UPDATE SET payload_json=excluded.payload_json
	`, record.ServerID, record.MinuteUnix, string(payload), s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upsert history minute: %w", err)
	}
	return nil
}

func (s *Store) Query(ctx context.Context, serverID, fromUnix, toUnix int64) ([]MinuteRecord, error) {
	if fromUnix > toUnix || (fromUnix <= math.MaxInt64-maxHistoryQuerySeconds && toUnix > fromUnix+maxHistoryQuerySeconds) {
		return nil, &InvalidRangeError{FromUnix: fromUnix, ToUnix: toUnix}
	}

	records := make([]MinuteRecord, 0)
	var afterUnix int64
	hasAfter := false
	for len(records) < maxHistoryQueryRows {
		pageLimit := min(historyQueryPageRows, maxHistoryQueryRows-len(records))
		page, err := s.queryPage(ctx, serverID, fromUnix, toUnix, afterUnix, hasAfter, pageLimit)
		if err != nil {
			return nil, err
		}

		for _, encoded := range page {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("iterate history minutes: %w", err)
			}
			record := MinuteRecord{ServerID: serverID, MinuteUnix: encoded.minuteUnix}
			if err := s.decodePayload(encoded.payload, &record.Payload); err != nil {
				return nil, fmt.Errorf("unmarshal history minute %d: %w", record.MinuteUnix, err)
			}
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("iterate history minutes: %w", err)
			}
			records = append(records, record)
		}

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("iterate history minutes: %w", err)
		}
		if len(page) < pageLimit || len(records) == maxHistoryQueryRows {
			return records, nil
		}
		afterUnix = page[len(page)-1].minuteUnix
		hasAfter = true
	}
	return records, nil
}

type encodedMinute struct {
	minuteUnix int64
	payload    []byte
}

func (s *Store) queryPage(ctx context.Context, serverID, fromUnix, toUnix, afterUnix int64, hasAfter bool, limit int) ([]encodedMinute, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if hasAfter {
		rows, err = s.conn.QueryContext(ctx, `
			SELECT minute_unix, payload_json
			FROM metric_minutes
			WHERE server_id=? AND minute_unix>? AND minute_unix<=?
			ORDER BY minute_unix ASC
			LIMIT ?
		`, serverID, afterUnix, toUnix, limit)
	} else {
		rows, err = s.conn.QueryContext(ctx, `
			SELECT minute_unix, payload_json
			FROM metric_minutes
			WHERE server_id=? AND minute_unix>=? AND minute_unix<=?
			ORDER BY minute_unix ASC
			LIMIT ?
		`, serverID, fromUnix, toUnix, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("query history minutes: %w", err)
	}
	defer rows.Close()

	page := make([]encodedMinute, 0, limit)
	for rows.Next() {
		var encoded encodedMinute
		var payload []byte
		if err := rows.Scan(&encoded.minuteUnix, &payload); err != nil {
			return nil, fmt.Errorf("scan history minute: %w", err)
		}
		encoded.payload = append([]byte(nil), payload...)
		page = append(page, encoded)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history minutes: %w", err)
	}
	return page, nil
}

func (s *Store) DeleteBefore(ctx context.Context, cutoffUnix int64) (int64, error) {
	result, err := s.conn.ExecContext(ctx, `DELETE FROM metric_minutes WHERE minute_unix < ?`, cutoffUnix)
	if err != nil {
		return 0, fmt.Errorf("delete expired history minutes: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted history minutes: %w", err)
	}
	return deleted, nil
}
