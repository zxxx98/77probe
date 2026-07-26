package db_test

import (
	"context"
	"path/filepath"
	"testing"

	monitorDB "probe.local/monitor/internal/db"
)

func TestApplyMigrationsCreatesCoreTables(t *testing.T) {
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	var journalMode string
	if err := conn.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal mode = %q, want wal", journalMode)
	}

	for _, table := range []string{"admins", "sessions", "servers", "schema_migrations"} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil || name != table {
			t.Fatalf("table %s was not created: %v", table, err)
		}
	}
}
