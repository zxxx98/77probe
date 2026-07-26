package servers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

const maxServers = 10

var (
	ErrInvalidInput = errors.New("invalid server input")
	ErrNotFound     = errors.New("server not found")
	ErrInvalidToken = errors.New("invalid agent token")
	ErrDisabled     = errors.New("server is disabled")
	ErrServerLimit  = errors.New("server limit reached")
)

type Server struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Enabled      bool      `json:"enabled"`
	AgentVersion string    `json:"agentVersion"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Service struct {
	db  *sql.DB
	now func() time.Time
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn, now: time.Now}
}

func (s *Service) Create(ctx context.Context, name string) (Server, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Server{}, "", ErrInvalidInput
	}
	rawToken, digest, err := agentTokenPair()
	if err != nil {
		return Server{}, "", err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Server{}, "", err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers`).Scan(&count); err != nil {
		return Server{}, "", err
	}
	if count >= maxServers {
		return Server{}, "", ErrServerLimit
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO servers(name, token_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`, name, digest, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return Server{}, "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Server{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return Server{}, "", err
	}
	return Server{ID: id, Name: name, Enabled: true, CreatedAt: now, UpdatedAt: now}, rawToken, nil
}

func (s *Service) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, enabled, agent_version, created_at, updated_at FROM servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := make([]Server, 0)
	for rows.Next() {
		server, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Service) Get(ctx context.Context, id int64) (Server, error) {
	server, err := scanServer(s.db.QueryRowContext(ctx, `SELECT id, name, enabled, agent_version, created_at, updated_at FROM servers WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return server, err
}

func (s *Service) Update(ctx context.Context, id int64, name *string, enabled *bool) (Server, error) {
	if name == nil && enabled == nil {
		return Server{}, ErrInvalidInput
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return Server{}, ErrInvalidInput
		}
		name = &trimmed
	}
	now := s.now().UTC()
	var row scanner
	switch {
	case name != nil && enabled != nil:
		row = s.db.QueryRowContext(ctx, `UPDATE servers SET name=?, enabled=?, updated_at=? WHERE id=? RETURNING id, name, enabled, agent_version, created_at, updated_at`, *name, *enabled, now.Format(time.RFC3339Nano), id)
	case name != nil:
		row = s.db.QueryRowContext(ctx, `UPDATE servers SET name=?, updated_at=? WHERE id=? RETURNING id, name, enabled, agent_version, created_at, updated_at`, *name, now.Format(time.RFC3339Nano), id)
	default:
		row = s.db.QueryRowContext(ctx, `UPDATE servers SET enabled=?, updated_at=? WHERE id=? RETURNING id, name, enabled, agent_version, created_at, updated_at`, *enabled, now.Format(time.RFC3339Nano), id)
	}
	server, err := scanServer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	return server, err
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id=?`, id)
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

func (s *Service) RotateToken(ctx context.Context, id int64) (string, error) {
	_, token, err := s.RotateTokenWithServer(ctx, id)
	return token, err
}

func (s *Service) RotateTokenWithServer(ctx context.Context, id int64) (Server, string, error) {
	rawToken, digest, err := agentTokenPair()
	if err != nil {
		return Server{}, "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Server{}, "", err
	}
	defer tx.Rollback()
	server, err := scanServer(tx.QueryRowContext(ctx, `UPDATE servers SET token_hash=?, updated_at=? WHERE id=? RETURNING id, name, enabled, agent_version, created_at, updated_at`, digest, s.now().UTC().Format(time.RFC3339Nano), id))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, "", ErrNotFound
	}
	if err != nil {
		return Server{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return Server{}, "", err
	}
	return server, rawToken, nil
}

func (s *Service) AuthenticateToken(ctx context.Context, rawToken string) (Server, error) {
	digest := sha256.Sum256([]byte(rawToken))
	server, err := scanServer(s.db.QueryRowContext(ctx, `SELECT id, name, enabled, agent_version, created_at, updated_at FROM servers WHERE token_hash=?`, digest[:]))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrInvalidToken
	}
	if err != nil {
		return Server{}, err
	}
	if !server.Enabled {
		return Server{}, ErrDisabled
	}
	return server, nil
}

func (s *Service) UpdateAgentVersion(ctx context.Context, id int64, version string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE servers SET agent_version=? WHERE id=?`, version, id)
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

type scanner interface {
	Scan(dest ...any) error
}

func scanServer(row scanner) (Server, error) {
	var server Server
	var enabled int
	var createdAt, updatedAt string
	err := row.Scan(&server.ID, &server.Name, &enabled, &server.AgentVersion, &createdAt, &updatedAt)
	if err != nil {
		return Server{}, err
	}
	server.Enabled = enabled != 0
	server.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Server{}, err
	}
	server.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Server{}, err
	}
	return server, nil
}

func agentTokenPair() (string, []byte, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, err
	}
	rawToken := "tp_" + base64.RawURLEncoding.EncodeToString(bytes)
	digest := sha256.Sum256([]byte(rawToken))
	return rawToken, digest[:], nil
}
