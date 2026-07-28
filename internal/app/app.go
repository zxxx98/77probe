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
	streams           streamCloser
}

type backgroundRunner interface {
	Run(context.Context)
}

type streamCloser interface {
	Close()
}

type Config struct {
	DatabasePath string
	AgentFiles   fs.FS
}

type Application struct {
	conn    *sql.DB
	runtime *runtime

	lifecycleMu       sync.Mutex
	backgroundStarted bool
	backgroundCancel  context.CancelFunc
	backgroundDone    <-chan struct{}
	closeStarted      bool
	closeDone         chan struct{}
	closeErr          error
	streamsCloseOnce  sync.Once
}

func newRuntime(conn *sql.DB, agentFiles fs.FS) (*runtime, error) {
	authService := auth.NewService(conn)
	serverService := servers.NewService(conn)
	store := live.NewStore()
	hub := live.NewHub()
	historyStore := history.NewStore(conn)
	historyAggregator := history.NewAggregator(historyStore)
	coordinator := live.NewCoordinator(serverService, store, hub, live.WithHistory(historyAggregator, historyAggregator))
	if err := serverService.AttachRegistryObserver(coordinator); err != nil {
		return nil, err
	}
	liveHandler := live.NewHandler(coordinator)
	historyHandler := history.NewHandler(historyStore, serverService)
	return &runtime{
		handler: httpapi.NewRouter(httpapi.Dependencies{
			Auth:       authService,
			Servers:    serverService,
			Live:       liveHandler,
			History:    historyHandler,
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
		streams: hub,
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
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.closeStarted {
		if a.backgroundDone == nil {
			a.backgroundDone = alreadyDone()
		}
		return a.backgroundDone
	}
	if a.backgroundStarted {
		return a.backgroundDone
	}
	backgroundCtx, cancel := context.WithCancel(ctx)
	a.backgroundStarted = true
	a.backgroundCancel = cancel
	a.backgroundDone = a.runtime.startBackground(backgroundCtx)
	return a.backgroundDone
}

func (a *Application) Close() error {
	if a == nil {
		return nil
	}

	a.lifecycleMu.Lock()
	if a.closeStarted {
		closeDone := a.closeDone
		a.lifecycleMu.Unlock()
		<-closeDone
		a.lifecycleMu.Lock()
		err := a.closeErr
		a.lifecycleMu.Unlock()
		return err
	}
	a.closeStarted = true
	a.closeDone = make(chan struct{})
	cancel := a.backgroundCancel
	done := a.backgroundDone
	if done == nil {
		a.backgroundDone = alreadyDone()
		done = a.backgroundDone
	}
	closeDone := a.closeDone
	a.lifecycleMu.Unlock()

	a.closeStreams()
	if cancel != nil {
		cancel()
	}
	<-done
	var err error
	if a.conn != nil {
		err = a.conn.Close()
	}

	a.lifecycleMu.Lock()
	a.closeErr = err
	close(closeDone)
	a.lifecycleMu.Unlock()
	return err
}

func (a *Application) closeStreams() {
	if a == nil {
		return
	}
	a.streamsCloseOnce.Do(func() {
		if a.runtime != nil && a.runtime.streams != nil {
			a.runtime.streams.Close()
		}
	})
}

func alreadyDone() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
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
	serverBase, cancelServerBase := context.WithCancel(context.WithoutCancel(ctx))
	application.RunBackground(context.WithoutCancel(ctx))
	srv := &http.Server{
		Addr:    addr,
		Handler: application.Handler(),
		BaseContext: func(net.Listener) context.Context {
			return serverBase
		},
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return shutdownServer(shutdownCtx, srv, application, cancelServerBase)
	case err := <-errCh:
		cancelServerBase()
		closeErr := application.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return closeErr
		}
		return errors.Join(err, closeErr)
	}
}

func shutdownServer(ctx context.Context, server *http.Server, application *Application, cancelServerBase context.CancelFunc) error {
	application.closeStreams()
	shutdownErr := server.Shutdown(ctx)
	var forceCloseErr error
	if shutdownErr != nil {
		forceCloseErr = server.Close()
		if errors.Is(forceCloseErr, http.ErrServerClosed) {
			forceCloseErr = nil
		}
	}
	cancelServerBase()
	applicationErr := application.Close()
	return errors.Join(shutdownErr, forceCloseErr, applicationErr)
}
