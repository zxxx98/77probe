package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"probe.local/monitor/internal/auth"
	monitorDB "probe.local/monitor/internal/db"
	"probe.local/monitor/internal/history"
	"probe.local/monitor/internal/httpapi"
	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/servers"
)

type runtime struct {
	handler           http.Handler
	servers           *servers.Service
	store             *live.Store
	historyStore      *history.Store
	historyAggregator *history.Aggregator
	background        []backgroundRunner
}

type backgroundRunner interface {
	Run(context.Context)
}

type Config struct {
	DatabasePath string
	AgentFiles   fs.FS
}

type Application struct {
	conn      *sql.DB
	runtime   *runtime
	closeOnce sync.Once
	closeErr  error
}

func newRuntime(conn *sql.DB, agentFiles fs.FS) (*runtime, error) {
	authService := auth.NewService(conn)
	serverService := servers.NewService(conn)
	store := live.NewStore()
	hub := live.NewHub()
	coordinator := live.NewCoordinator(serverService, store, hub)
	if err := serverService.AttachRegistryObserver(coordinator); err != nil {
		return nil, err
	}
	historyStore := history.NewStore(conn)
	historyAggregator := history.NewAggregator(historyStore)
	liveHandler := live.NewHandler(coordinator, live.WithHistoryAccepter(historyAggregator))
	return &runtime{
		handler: httpapi.NewRouter(httpapi.Dependencies{
			Auth:       authService,
			Servers:    serverService,
			Live:       liveHandler,
			AgentFiles: agentFiles,
		}),
		servers:           serverService,
		store:             store,
		historyStore:      historyStore,
		historyAggregator: historyAggregator,
		background: []backgroundRunner{
			live.NewSweeper(coordinator),
			history.NewJobs(historyAggregator, historyStore),
		},
	}, nil
}

func New(config Config) (*Application, error) {
	if strings.TrimSpace(config.DatabasePath) == "" {
		return nil, fmt.Errorf("database path is required")
	}
	conn, err := monitorDB.Open(config.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := monitorDB.ApplyMigrations(context.Background(), conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	runtime, err := newRuntime(conn, config.AgentFiles)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Application{conn: conn, runtime: runtime}, nil
}

func (a *Application) Handler() http.Handler {
	return a.runtime.handler
}

func (a *Application) RunBackground(ctx context.Context) <-chan struct{} {
	return a.runtime.startBackground(ctx)
}

func (a *Application) Close() error {
	if a == nil || a.conn == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		a.closeErr = a.conn.Close()
	})
	return a.closeErr
}

func (r *runtime) startBackground(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(len(r.background))
	for _, runner := range r.background {
		runner := runner
		go func() {
			defer wait.Done()
			runner.Run(ctx)
		}()
	}
	go func() {
		wait.Wait()
		close(done)
	}()
	return done
}

func Run(ctx context.Context, addr string, config Config) error {
	application, err := New(config)
	if err != nil {
		return err
	}
	defer application.Close()
	liveCtx, cancelLive := context.WithCancel(ctx)
	liveDone := application.RunBackground(liveCtx)
	defer func() {
		cancelLive()
		<-liveDone
	}()
	srv := &http.Server{
		Addr:    addr,
		Handler: application.Handler(),
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
