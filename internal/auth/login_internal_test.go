package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	monitorDB "probe.local/monitor/internal/db"
)

func TestLoginUnknownUserVerifiesDummyPasswordHash(t *testing.T) {
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	svc := NewService(conn)
	var verifiedHash string
	verifyCalls := 0
	svc.verifyPassword = func(_ string, encoded string) (bool, error) {
		verifyCalls++
		verifiedHash = encoded
		return false, nil
	}

	if _, _, err := svc.Login(context.Background(), "unknown", "candidate password"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("login error = %v", err)
	}
	if verifyCalls != 1 {
		t.Fatalf("password verification calls = %d, want 1", verifyCalls)
	}
	if _, err := verifyPassword("candidate password", verifiedHash); err != nil {
		t.Fatalf("dummy password hash is invalid: %v", err)
	}
}

func TestLoginUsesStoredAdminHashForKnownAndUnknownUsername(t *testing.T) {
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	svc := NewService(conn)
	if err := svc.CreateAdmin(context.Background(), "xiaodi", "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	var storedHash string
	if err := conn.QueryRow(`SELECT password_hash FROM admins WHERE id=1`).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}

	var verifiedHashes []string
	svc.verifyPassword = func(_ string, encoded string) (bool, error) {
		verifiedHashes = append(verifiedHashes, encoded)
		return false, nil
	}

	for _, username := range []string{"xiaodi", "unknown"} {
		if _, _, err := svc.Login(context.Background(), username, "wrong password"); !errors.Is(err, ErrInvalidLogin) {
			t.Fatalf("login for %q error = %v", username, err)
		}
	}
	if len(verifiedHashes) != 2 {
		t.Fatalf("password verification calls = %d, want 2", len(verifiedHashes))
	}
	if verifiedHashes[0] != storedHash || verifiedHashes[1] != storedHash {
		t.Fatal("known and unknown usernames were not verified against the identical stored administrator hash")
	}
}
