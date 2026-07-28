package alerting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Repository struct {
	db  *sql.DB
	now func() time.Time
}

func NewRepository(db *sql.DB) *Repository {
	if db == nil {
		panic("alerting repository requires database")
	}
	return &Repository{db: db, now: time.Now}
}

func (r *Repository) CreateRule(ctx context.Context, rule Rule) (Rule, error) {
	if err := validateRule(rule); err != nil {
		return Rule{}, err
	}
	now := r.now().UTC()
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO alert_rules(server_id, metric, operator, threshold, duration_seconds, repeat_seconds, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, server_id, metric, operator, threshold, duration_seconds, repeat_seconds, enabled, created_at, updated_at`,
		rule.ServerID, rule.Metric, rule.Operator, rule.Threshold, rule.DurationSeconds, rule.RepeatSeconds, boolInt(rule.Enabled), formatTime(now), formatTime(now))
	created, err := scanRule(row)
	if err != nil {
		return Rule{}, err
	}
	created.State = StatusNormal
	return created, nil
}

func (r *Repository) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.server_id, r.metric, r.operator, r.threshold, r.duration_seconds, r.repeat_seconds, r.enabled, r.created_at, r.updated_at,
		       COALESCE(s.status, 'normal')
		FROM alert_rules r
		LEFT JOIN alert_states s ON s.rule_id = r.id
		ORDER BY r.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]Rule, 0)
	for rows.Next() {
		rule, err := scanRuleWithState(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *Repository) GetRule(ctx context.Context, id int64) (Rule, error) {
	if id < 1 {
		return Rule{}, ErrInvalidInput
	}
	rule, err := scanRule(r.db.QueryRowContext(ctx, `
		SELECT id, server_id, metric, operator, threshold, duration_seconds, repeat_seconds, enabled, created_at, updated_at
		FROM alert_rules WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	if err != nil {
		return Rule{}, err
	}
	state, err := r.GetState(ctx, id)
	if err == nil {
		rule.State = state.Status
	} else if errors.Is(err, ErrNotFound) {
		rule.State = StatusNormal
	} else {
		return Rule{}, err
	}
	return rule, nil
}

func (r *Repository) ListEnabledRulesForServer(ctx context.Context, serverID int64) ([]Rule, error) {
	if serverID < 1 {
		return nil, ErrInvalidInput
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, server_id, metric, operator, threshold, duration_seconds, repeat_seconds, enabled, created_at, updated_at
		FROM alert_rules WHERE server_id=? AND enabled=1 ORDER BY id ASC`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]Rule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rule.State = StatusNormal
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *Repository) UpdateRule(ctx context.Context, rule Rule) (Rule, error) {
	if rule.ID < 1 || validateRule(rule) != nil {
		return Rule{}, ErrInvalidInput
	}
	now := r.now().UTC()
	row := r.db.QueryRowContext(ctx, `
		UPDATE alert_rules SET server_id=?, metric=?, operator=?, threshold=?, duration_seconds=?, repeat_seconds=?, enabled=?, updated_at=?
		WHERE id=?
		RETURNING id, server_id, metric, operator, threshold, duration_seconds, repeat_seconds, enabled, created_at, updated_at`,
		rule.ServerID, rule.Metric, rule.Operator, rule.Threshold, rule.DurationSeconds, rule.RepeatSeconds, boolInt(rule.Enabled), formatTime(now), rule.ID)
	updated, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	if err != nil {
		return Rule{}, err
	}
	state, err := r.GetState(ctx, updated.ID)
	if err == nil {
		updated.State = state.Status
	} else if errors.Is(err, ErrNotFound) {
		updated.State = StatusNormal
	} else {
		return Rule{}, err
	}
	return updated, nil
}

func (r *Repository) DeleteRule(ctx context.Context, id int64) error {
	if id < 1 {
		return ErrInvalidInput
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetState(ctx context.Context, ruleID int64) (State, error) {
	if ruleID < 1 {
		return State{}, ErrInvalidInput
	}
	state, err := scanState(r.db.QueryRowContext(ctx, `
		SELECT rule_id, status, pending_since, firing_since, last_notified_at, last_value, updated_at
		FROM alert_states WHERE rule_id=?`, ruleID))
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, ErrNotFound
	}
	return state, err
}

func (r *Repository) SaveStateAndEvent(ctx context.Context, state State, event *Event) (Event, error) {
	if state.RuleID < 1 || !validStatus(state.Status) {
		return Event{}, ErrInvalidInput
	}
	if event != nil && (event.RuleID < 1 || event.ServerID < 1 || !validStatus(event.Status)) {
		return Event{}, ErrInvalidInput
	}
	now := r.now().UTC()
	state.UpdatedAt = now
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO alert_states(rule_id, status, pending_since, firing_since, last_notified_at, last_value, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(rule_id) DO UPDATE SET status=excluded.status, pending_since=excluded.pending_since,
		firing_since=excluded.firing_since, last_notified_at=excluded.last_notified_at, last_value=excluded.last_value,
		updated_at=excluded.updated_at`,
		state.RuleID, state.Status, optionalTime(state.PendingSince), optionalTime(state.FiringSince), optionalTime(state.LastNotifiedAt), optionalFloat(state.LastValue), formatTime(state.UpdatedAt))
	if err != nil {
		return Event{}, err
	}
	if event == nil {
		if err := tx.Commit(); err != nil {
			return Event{}, err
		}
		return Event{}, nil
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO alert_events(rule_id, server_id, status, current_value, threshold, started_at, ended_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id`, event.RuleID, event.ServerID, event.Status, optionalFloat(event.CurrentValue), optionalFloat(event.Threshold), formatTime(event.StartedAt), optionalTime(event.EndedAt), formatTime(event.CreatedAt))
	if err := row.Scan(&event.ID); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return *event, nil
}

func (r *Repository) ListEvents(ctx context.Context, beforeID int64, limit int) ([]Event, error) {
	if beforeID < 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, e.rule_id, e.server_id, s.name, r.metric, e.status, e.current_value, e.threshold, e.started_at, e.ended_at, e.created_at
		FROM alert_events e
		JOIN servers s ON s.id=e.server_id
		JOIN alert_rules r ON r.id=e.rule_id
		WHERE (?=0 OR e.id<?)
		ORDER BY e.id DESC LIMIT ?`, beforeID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range events {
		attempts, err := r.ListAttempts(ctx, events[index].ID)
		if err != nil {
			return nil, err
		}
		events[index].Attempts = attempts
	}
	return events, nil
}

func (r *Repository) GetWebhook(ctx context.Context) (WebhookConfig, error) {
	config, err := scanWebhook(r.db.QueryRowContext(ctx, `
		SELECT url, headers_json, body_template, enabled, created_at, updated_at FROM webhook_configs WHERE id=1`))
	if errors.Is(err, sql.ErrNoRows) {
		return WebhookConfig{}, ErrNotFound
	}
	return config, err
}

func (r *Repository) UpsertWebhook(ctx context.Context, config WebhookConfig) (WebhookConfig, error) {
	if config.URL == "" || config.BodyTemplate == "" {
		return WebhookConfig{}, ErrInvalidInput
	}
	headers, err := json.Marshal(config.Headers)
	if err != nil {
		return WebhookConfig{}, ErrInvalidInput
	}
	now := r.now().UTC()
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO webhook_configs(id, url, headers_json, body_template, enabled, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET url=excluded.url, headers_json=excluded.headers_json, body_template=excluded.body_template,
		enabled=excluded.enabled, updated_at=excluded.updated_at
		RETURNING url, headers_json, body_template, enabled, created_at, updated_at`,
		config.URL, string(headers), config.BodyTemplate, boolInt(config.Enabled), formatTime(now), formatTime(now))
	return scanWebhook(row)
}

func (r *Repository) RecordAttempt(ctx context.Context, attempt Attempt) (Attempt, error) {
	if attempt.Attempt < 1 {
		return Attempt{}, ErrInvalidInput
	}
	if attempt.EventID != nil && *attempt.EventID < 1 {
		return Attempt{}, ErrInvalidInput
	}
	if attempt.SentAt.IsZero() {
		attempt.SentAt = r.now().UTC()
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO webhook_attempts(event_id, is_test, attempt, response_status, error_text, sent_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id, event_id, is_test, attempt, response_status, error_text, sent_at`,
		optionalInt64(attempt.EventID), boolInt(attempt.IsTest), attempt.Attempt, optionalInt(attempt.ResponseStatus), attempt.ErrorText, formatTime(attempt.SentAt))
	return scanAttempt(row)
}

func (r *Repository) ListAttempts(ctx context.Context, eventID int64) ([]Attempt, error) {
	if eventID < 1 {
		return nil, ErrInvalidInput
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, event_id, is_test, attempt, response_status, error_text, sent_at
		FROM webhook_attempts WHERE event_id=? ORDER BY id ASC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := make([]Attempt, 0)
	for rows.Next() {
		attempt, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func validateRule(rule Rule) error {
	if rule.ServerID < 1 || !validMetric(rule.Metric) || !validOperator(rule.Operator) || rule.DurationSeconds < 0 || rule.RepeatSeconds < 0 {
		return ErrInvalidInput
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanRule(row rowScanner) (Rule, error) {
	var rule Rule
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&rule.ID, &rule.ServerID, &rule.Metric, &rule.Operator, &rule.Threshold, &rule.DurationSeconds, &rule.RepeatSeconds, &enabled, &createdAt, &updatedAt)
	if err != nil {
		return Rule{}, err
	}
	if !validMetric(rule.Metric) || !validOperator(rule.Operator) {
		return Rule{}, fmt.Errorf("stored alert rule has invalid enum")
	}
	rule.Enabled = enabled != 0
	rule.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Rule{}, err
	}
	rule.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func scanRuleWithState(row rowScanner) (Rule, error) {
	var rule Rule
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&rule.ID, &rule.ServerID, &rule.Metric, &rule.Operator, &rule.Threshold, &rule.DurationSeconds, &rule.RepeatSeconds, &enabled, &createdAt, &updatedAt, &rule.State)
	if err != nil {
		return Rule{}, err
	}
	if !validMetric(rule.Metric) || !validOperator(rule.Operator) || !validStatus(rule.State) {
		return Rule{}, fmt.Errorf("stored alert rule has invalid enum")
	}
	rule.Enabled = enabled != 0
	rule.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Rule{}, err
	}
	rule.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func scanState(row rowScanner) (State, error) {
	var state State
	var pending, firing, notified sql.NullString
	var value sql.NullFloat64
	var updated string
	err := row.Scan(&state.RuleID, &state.Status, &pending, &firing, &notified, &value, &updated)
	if err != nil {
		return State{}, err
	}
	if !validStatus(state.Status) {
		return State{}, fmt.Errorf("stored alert state has invalid status")
	}
	var parseErr error
	state.PendingSince, parseErr = parseOptionalTime(pending)
	if parseErr != nil {
		return State{}, parseErr
	}
	state.FiringSince, parseErr = parseOptionalTime(firing)
	if parseErr != nil {
		return State{}, parseErr
	}
	state.LastNotifiedAt, parseErr = parseOptionalTime(notified)
	if parseErr != nil {
		return State{}, parseErr
	}
	if value.Valid {
		state.LastValue = &value.Float64
	}
	state.UpdatedAt, parseErr = parseTime(updated)
	if parseErr != nil {
		return State{}, parseErr
	}
	return state, nil
}

func scanEvent(row rowScanner) (Event, error) {
	var event Event
	var current, threshold sql.NullFloat64
	var ended sql.NullString
	var started, created string
	err := row.Scan(&event.ID, &event.RuleID, &event.ServerID, &event.ServerName, &event.Metric, &event.Status, &current, &threshold, &started, &ended, &created)
	if err != nil {
		return Event{}, err
	}
	if !validMetric(event.Metric) || !validStatus(event.Status) {
		return Event{}, fmt.Errorf("stored alert event has invalid enum")
	}
	if current.Valid {
		event.CurrentValue = &current.Float64
	}
	if threshold.Valid {
		event.Threshold = &threshold.Float64
	}
	var parseErr error
	event.StartedAt, parseErr = parseTime(started)
	if parseErr != nil {
		return Event{}, parseErr
	}
	event.EndedAt, parseErr = parseOptionalTime(ended)
	if parseErr != nil {
		return Event{}, parseErr
	}
	event.CreatedAt, parseErr = parseTime(created)
	if parseErr != nil {
		return Event{}, parseErr
	}
	return event, nil
}

func scanWebhook(row rowScanner) (WebhookConfig, error) {
	var config WebhookConfig
	var headers, created, updated string
	var enabled int
	err := row.Scan(&config.URL, &headers, &config.BodyTemplate, &enabled, &created, &updated)
	if err != nil {
		return WebhookConfig{}, err
	}
	if err := json.Unmarshal([]byte(headers), &config.Headers); err != nil {
		return WebhookConfig{}, err
	}
	if config.Headers == nil {
		config.Headers = map[string]string{}
	}
	config.Enabled = enabled != 0
	config.CreatedAt, err = parseTime(created)
	if err != nil {
		return WebhookConfig{}, err
	}
	config.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return WebhookConfig{}, err
	}
	return config, nil
}

func scanAttempt(row rowScanner) (Attempt, error) {
	var attempt Attempt
	var eventID sql.NullInt64
	var isTest int
	var response sql.NullInt64
	var sent string
	err := row.Scan(&attempt.ID, &eventID, &isTest, &attempt.Attempt, &response, &attempt.ErrorText, &sent)
	if err != nil {
		return Attempt{}, err
	}
	if eventID.Valid {
		attempt.EventID = &eventID.Int64
	}
	if response.Valid {
		value := int(response.Int64)
		attempt.ResponseStatus = &value
	}
	attempt.IsTest = isTest != 0
	attempt.SentAt, err = parseTime(sent)
	return attempt, err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func optionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}
func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
func optionalInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
func optionalInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
func formatTime(value time.Time) string         { return value.UTC().Format(time.RFC3339Nano) }
func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }
func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
