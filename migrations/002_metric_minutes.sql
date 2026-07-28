CREATE TABLE metric_minutes (
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    minute_unix INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (server_id, minute_unix)
);

CREATE INDEX metric_minutes_minute_idx ON metric_minutes(minute_unix);
