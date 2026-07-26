package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"probe.local/monitor/internal/auth"
	monitorDB "probe.local/monitor/internal/db"
)

func newTestService(t *testing.T) (*auth.Service, *sql.DB) {
	t.Helper()

	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	return auth.NewService(conn), conn
}

func TestCreateAdminOnlyOnceAndLogin(t *testing.T) {
	svc, conn := newTestService(t)
	ctx := context.Background()

	required, err := svc.SetupRequired(ctx)
	if err != nil || !required {
		t.Fatalf("required=%v err=%v", required, err)
	}
	if err := svc.CreateAdmin(ctx, "xiaodi", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateAdmin(ctx, "second", "another secure password"); !errors.Is(err, auth.ErrSetupComplete) {
		t.Fatalf("second setup error = %v", err)
	}

	token, expiresAt, err := svc.Login(ctx, "xiaodi", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || !expiresAt.After(time.Now().Add(6*24*time.Hour)) {
		t.Fatalf("token present=%v expiresAt=%v", token != "", expiresAt)
	}

	admin, err := svc.Authenticate(ctx, token)
	if err != nil || admin.ID != 1 || admin.Username != "xiaodi" {
		t.Fatalf("admin=%+v err=%v", admin, err)
	}

	var stored []byte
	if err := conn.QueryRowContext(ctx, `SELECT token_hash FROM sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	if !bytes.Equal(stored, digest[:]) {
		t.Fatal("stored session token hash does not match the raw token digest")
	}
	if bytes.Equal(stored, []byte(token)) {
		t.Fatal("raw session token was stored")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateAdmin(ctx, "xiaodi", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := svc.Login(ctx, "xiaodi", "wrong password"); !errors.Is(err, auth.ErrInvalidLogin) {
		t.Fatalf("login error = %v", err)
	}
}

func TestAuthenticateRejectsExpiredSession(t *testing.T) {
	svc, conn := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateAdmin(ctx, "xiaodi", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.Login(ctx, "xiaodi", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	if _, err := conn.ExecContext(ctx, `UPDATE sessions SET expires_at=? WHERE token_hash=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), digest[:]); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("authenticate error = %v", err)
	}
}

func TestLogoutDeletesSession(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if err := svc.CreateAdmin(ctx, "xiaodi", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.Login(ctx, "xiaodi", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("authenticate after logout error = %v", err)
	}
}

func TestCreateAdminValidatesInput(t *testing.T) {
	svc, _ := newTestService(t)

	tests := []struct {
		name     string
		username string
		password string
	}{
		{name: "empty username", password: "correct horse battery staple"},
		{name: "long username", username: string(bytes.Repeat([]byte("x"), 65)), password: "correct horse battery staple"},
		{name: "short password", username: "xiaodi", password: "too short"},
		{name: "long password", username: "xiaodi", password: string(bytes.Repeat([]byte("x"), 129))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.CreateAdmin(context.Background(), tt.username, tt.password); !errors.Is(err, auth.ErrInvalidInput) {
				t.Fatalf("CreateAdmin error = %v", err)
			}
		})
	}
}
