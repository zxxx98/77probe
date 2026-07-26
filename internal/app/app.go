package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"probe.local/monitor/internal/auth"
	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/servers"
)

func Run(ctx context.Context, addr string) error {
	dbPath := os.Getenv("TINYPROBE_DB_PATH")
	if dbPath == "" {
		dbPath = "tinyprobe.db"
	}
	conn, err := monitorDB.Open(dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := monitorDB.ApplyMigrations(ctx, conn); err != nil {
		return err
	}

	authService := auth.NewService(conn)
	serverService := servers.NewService(conn)
	srv := &http.Server{Addr: addr, Handler: httpapi.NewRouter(httpapi.Dependencies{Auth: authService, Servers: serverService})}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
