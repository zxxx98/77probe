package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"probe.local/monitor/internal/auth"
)

type Dependencies struct {
	Auth *auth.Service
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", health)

	authHandler := auth.NewHandler(deps.Auth)
	r.Get("/api/setup/status", authHandler.SetupStatus)
	r.Post("/api/setup", authHandler.Setup)
	r.Post("/api/login", authHandler.Login)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireSession(deps.Auth))
		r.Post("/api/logout", authHandler.Logout)
		r.Get("/api/me", authHandler.Me)
	})
	return r
}
