package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"probe.local/monitor/internal/httpapi"
)

func Run(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: httpapi.NewRouter(httpapi.Dependencies{})}
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
