package servers_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/servers"
)

func newServerService(t *testing.T) (*servers.Service, *sql.DB) {
	t.Helper()
	conn, err := monitorDB.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	return servers.NewService(conn), conn
}

func TestCreateReturnsTokenOnceAndStoresDigest(t *testing.T) {
	svc, conn := newServerService(t)
	server, token, err := svc.Create(context.Background(), "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "tp_") || len(token) != len("tp_")+43 {
		t.Fatalf("token = %q", token)
	}
	if server.Name != "home-lab" || !server.Enabled || server.AgentVersion != "" {
		t.Fatalf("server = %+v", server)
	}
	authenticated, err := svc.AuthenticateToken(context.Background(), token)
	if err != nil || authenticated.ID != server.ID {
		t.Fatalf("authenticated=%+v err=%v", authenticated, err)
	}
	var stored []byte
	if err := conn.QueryRow(`SELECT token_hash FROM servers WHERE id=?`, server.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(token))
	if !bytes.Equal(stored, digest[:]) || bytes.Equal(stored, []byte(token)) {
		t.Fatal("server token was not stored solely as a SHA-256 digest")
	}
	listed, err := svc.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].ID != server.ID {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
}

func TestRotateTokenInvalidatesPreviousToken(t *testing.T) {
	svc, _ := newServerService(t)
	ctx := context.Background()
	server, oldToken, err := svc.Create(ctx, "home-lab")
	if err != nil {
		t.Fatal(err)
	}

	newToken, err := svc.RotateToken(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateToken(ctx, oldToken); !errors.Is(err, servers.ErrInvalidToken) {
		t.Fatalf("old token error = %v", err)
	}
	got, err := svc.AuthenticateToken(ctx, newToken)
	if err != nil || got.ID != server.ID {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestRotateTokenWithServerRollsBackWhenResponseModelCannotBeRead(t *testing.T) {
	svc, conn := newServerService(t)
	ctx := context.Background()
	server, oldToken, err := svc.Create(ctx, "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE servers SET created_at='not-a-time' WHERE id=?`, server.ID); err != nil {
		t.Fatal(err)
	}

	if _, _, err := svc.RotateTokenWithServer(ctx, server.ID); err == nil {
		t.Fatal("rotation unexpectedly succeeded")
	}

	var stored []byte
	if err := conn.QueryRowContext(ctx, `SELECT token_hash FROM servers WHERE id=?`, server.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(oldToken))
	if !bytes.Equal(stored, digest[:]) {
		t.Fatal("rotation committed a replacement token after response-model retrieval failed")
	}
}

func TestSetEnabledFalseRejectsToken(t *testing.T) {
	svc, _ := newServerService(t)
	ctx := context.Background()
	server, token, err := svc.Create(ctx, "home-lab")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, server.ID, nil, boolPointer(false)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateToken(ctx, token); !errors.Is(err, servers.ErrDisabled) {
		t.Fatalf("token error = %v", err)
	}
}

func TestUpdateChangesOnlyRequestedFieldAndRejectsDeleteDuringUpdate(t *testing.T) {
	svc, conn := newServerService(t)
	ctx := context.Background()
	server, _, err := svc.Create(ctx, "home-lab")
	if err != nil {
		t.Fatal(err)
	}

	name := "office-lab"
	renamed, err := svc.Update(ctx, server.ID, &name, nil)
	if err != nil || renamed.Name != name || !renamed.Enabled {
		t.Fatalf("rename=%+v err=%v", renamed, err)
	}
	disabled := false
	updated, err := svc.Update(ctx, server.ID, nil, &disabled)
	if err != nil || updated.Name != name || updated.Enabled {
		t.Fatalf("disable=%+v err=%v", updated, err)
	}

	trigger := fmt.Sprintf(`CREATE TRIGGER delete_server_before_update BEFORE UPDATE ON servers WHEN OLD.id=%d BEGIN DELETE FROM servers WHERE id=OLD.id; END`, server.ID)
	if _, err := conn.ExecContext(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, server.ID, &name, nil); !errors.Is(err, servers.ErrNotFound) {
		t.Fatalf("update after deletion error=%v, want ErrNotFound", err)
	}
}

func TestCreateEnforcesTenServerLimit(t *testing.T) {
	svc, _ := newServerService(t)
	ctx := context.Background()
	for i := range 10 {
		if _, _, err := svc.Create(ctx, "server-"+string(rune('a'+i))); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, _, err := svc.Create(ctx, "eleven"); !errors.Is(err, servers.ErrServerLimit) {
		t.Fatalf("eleventh create error = %v", err)
	}
}

func boolPointer(value bool) *bool { return &value }
