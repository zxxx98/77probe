package app

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"os"
	"time"

	"probe.local/monitor/internal/auth"
	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/servers"
)

type runtime struct {
	handler http.Handler
	servers *servers.Service
	store   *live.Store
	sweeper *live.Sweeper
}

func newRuntime(conn *sql.DB) (*runtime, error) {
	authService := auth.NewService(conn)
	serverService := servers.NewService(conn)
	store := live.NewStore()
	hub := live.NewHub()
	coordinator := live.NewCoordinator(serverService, store, hub)
	if err := serverService.AttachRegistryObserver(coordinator); err != nil {
		return nil, err
	}
	liveHandler := live.NewHandler(coordinator)
	return &runtime{
		handler: httpapi.NewRouter(httpapi.Dependencies{Auth: authService, Servers: serverService, Live: liveHandler}),
		servers: serverService,
		store:   store,
		sweeper: live.NewSweeper(coordinator),
	}, nil
}

func (r *runtime) startSweeper(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		r.sweeper.Run(ctx)
		close(done)
	}()
	return done
}

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

	runtime, err := newRuntime(conn)
	if err != nil {
		return err
	}
	liveCtx, cancelLive := context.WithCancel(ctx)
	liveDone := runtime.startSweeper(liveCtx)
	defer func() {
		cancelLive()
		<-liveDone
	}()
	srv := &http.Server{
		Addr:    addr,
		Handler: runtime.handler,
		BaseContext: func(net.Listener) context.Context {
			return liveCtx
		},
	}
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
