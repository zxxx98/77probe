package history_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	monitorDB "probe.local/monitor/internal/db"
)

func TestMetricMinuteUniqueness(t *testing.T) {
	conn := migratedDatabase(t)

	_, err := conn.Exec(`
		INSERT INTO servers(id, name, token_hash, created_at, updated_at)
		VALUES (1, 'test server', X'01', '2026-07-28T00:00:00Z', '2026-07-28T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = conn.Exec(`
		INSERT INTO metric_minutes(server_id, minute_unix, payload_json, created_at)
		VALUES (1, 100, '{}', '2026-07-28T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = conn.Exec(`
		INSERT INTO metric_minutes(server_id, minute_unix, payload_json, created_at)
		VALUES (1, 100, '{}', '2026-07-28T00:00:00Z')
	`)
	if err == nil {
		t.Fatal("expected unique server/minute constraint")
	}
}

func migratedDatabase(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	return conn
}
