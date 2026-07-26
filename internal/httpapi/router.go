package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"probe.local/monitor/internal/auth"
	"probe.local/monitor/internal/servers"
)

type Dependencies struct {
	Auth    *auth.Service
	Servers *servers.Service
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", health)

	authHandler := auth.NewHandler(deps.Auth)
	serverHandler := servers.NewHandler(deps.Servers)
	r.Get("/api/setup/status", authHandler.SetupStatus)
	r.Post("/api/setup", authHandler.Setup)
	r.Post("/api/login", authHandler.Login)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireSession(deps.Auth))
		r.Post("/api/logout", authHandler.Logout)
		r.Get("/api/me", authHandler.Me)
		r.Get("/api/servers", serverHandler.List)
		r.Post("/api/servers", serverHandler.Create)
		r.Patch("/api/servers/{id}", serverHandler.Update)
		r.Delete("/api/servers/{id}", serverHandler.Delete)
		r.Post("/api/servers/{id}/token", serverHandler.RotateToken)
	})
	return r
}
