CREATE TABLE webhook_configs (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    url TEXT NOT NULL,
    headers_json TEXT NOT NULL DEFAULT '{}',
    body_template TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    metric TEXT NOT NULL,
    operator TEXT NOT NULL,
    threshold REAL NOT NULL,
    duration_seconds INTEGER NOT NULL,
    repeat_seconds INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX alert_rules_server_enabled_idx ON alert_rules(server_id, enabled);

CREATE TABLE alert_states (
    rule_id INTEGER PRIMARY KEY REFERENCES alert_rules(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    pending_since TEXT,
    firing_since TEXT,
    last_notified_at TEXT,
    last_value REAL,
    updated_at TEXT NOT NULL
);

CREATE TABLE alert_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id INTEGER NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    current_value REAL,
    threshold REAL,
    started_at TEXT NOT NULL,
    ended_at TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX alert_events_server_id_idx ON alert_events(server_id, id DESC);

CREATE TABLE webhook_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id INTEGER REFERENCES alert_events(id) ON DELETE CASCADE,
    is_test INTEGER NOT NULL DEFAULT 0,
    attempt INTEGER NOT NULL,
    response_status INTEGER,
    error_text TEXT NOT NULL DEFAULT '',
    sent_at TEXT NOT NULL
);

CREATE INDEX webhook_attempts_event_id_idx ON webhook_attempts(event_id, id ASC);
