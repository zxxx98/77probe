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
	conn *sql.DB
	now  func() time.Time
}

func NewStore(conn *sql.DB) *Store {
	return &Store{conn: conn, now: time.Now}
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

	rows, err := s.conn.QueryContext(ctx, `
		SELECT minute_unix, payload_json
		FROM metric_minutes
		WHERE server_id=? AND minute_unix>=? AND minute_unix<=?
		ORDER BY minute_unix ASC
		LIMIT ?
	`, serverID, fromUnix, toUnix, maxHistoryQueryRows)
	if err != nil {
		return nil, fmt.Errorf("query history minutes: %w", err)
	}
	defer rows.Close()

	records := make([]MinuteRecord, 0)
	for rows.Next() {
		var record MinuteRecord
		var payload []byte
		record.ServerID = serverID
		if err := rows.Scan(&record.MinuteUnix, &payload); err != nil {
			return nil, fmt.Errorf("scan history minute: %w", err)
		}
		if err := json.Unmarshal(payload, &record.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal history minute %d: %w", record.MinuteUnix, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history minutes: %w", err)
	}
	return records, nil
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
