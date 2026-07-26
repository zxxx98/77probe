package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}

	return conn, nil
}
