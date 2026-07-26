package auth

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

type Admin struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

var (
	ErrSetupComplete   = errors.New("administrator already exists")
	ErrInvalidInput    = errors.New("invalid administrator input")
	ErrInvalidLogin    = errors.New("invalid username or password")
	ErrUnauthenticated = errors.New("unauthenticated")
)

type Service struct {
	db             *sql.DB
	now            func() time.Time
	verifyPassword func(password, encoded string) (bool, error)
}

func NewService(conn *sql.DB) *Service {
	return &Service{db: conn, now: time.Now, verifyPassword: verifyPassword}
}

func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *Service) CreateAdmin(ctx context.Context, username, password string) error {
	if len(username) < 1 || len(username) > 64 || len(password) < 12 || len(password) > 128 {
		return ErrInvalidInput
	}

	required, err := s.SetupRequired(ctx)
	if err != nil {
		return err
	}
	if !required {
		return ErrSetupComplete
	}

	encoded, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO admins(id, username, password_hash, created_at) VALUES (1, ?, ?, ?)`, username, encoded, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrSetupComplete
	}
	return err
}

func (s *Service) Login(ctx context.Context, username, password string) (string, time.Time, error) {
	var id int64
	var storedUsername string
	var encoded string
	usernameMismatch := false
	if err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash FROM admins LIMIT 1`).Scan(&id, &storedUsername, &encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			encoded = dummyPasswordHash
			usernameMismatch = true
		} else {
			return "", time.Time{}, err
		}
	} else {
		usernameMismatch = storedUsername != username
	}

	ok, err := s.verifyPassword(password, encoded)
	if err != nil || !ok || usernameMismatch {
		return "", time.Time{}, ErrInvalidLogin
	}

	raw, digest, err := tokenPair()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expires := now.Add(7 * 24 * time.Hour)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash, admin_id, expires_at, created_at) VALUES (?, ?, ?, ?)`, digest, id, expires.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return raw, expires, err
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (Admin, error) {
	digest := sha256.Sum256([]byte(rawToken))
	var admin Admin
	err := s.db.QueryRowContext(ctx, `SELECT a.id, a.username FROM sessions s JOIN admins a ON a.id=s.admin_id WHERE s.token_hash=? AND s.expires_at>?`, digest[:], s.now().UTC().Format(time.RFC3339Nano)).Scan(&admin.ID, &admin.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return Admin{}, ErrUnauthenticated
	}
	if err != nil {
		return Admin{}, err
	}
	return admin, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	digest := sha256.Sum256([]byte(rawToken))
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, digest[:])
	return err
}

func tokenPair() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	text := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(text))
	return text, digest[:], nil
}
