package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"probe.local/monitor/internal/auth"
	"probe.local/monitor/internal/live"
	"probe.local/monitor/internal/servers"
	"probe.local/monitor/internal/webui"
)

type Dependencies struct {
	Auth    *auth.Service
	Servers *servers.Service
	Live    *live.Handler
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	spa := webui.Handler()
	r.Get("/api/health", health)

	authHandler := auth.NewHandler(deps.Auth)
	serverHandler := servers.NewHandler(deps.Servers)
	r.Get("/api/setup/status", authHandler.SetupStatus)
	r.Post("/api/setup", authHandler.Setup)
	r.Post("/api/login", authHandler.Login)
	r.Post("/api/agent/v1/report", deps.Live.Ingest)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireSession(deps.Auth))
		r.Post("/api/logout", authHandler.Logout)
		r.Get("/api/me", authHandler.Me)
		r.Get("/api/servers", serverHandler.List)
		r.Get("/api/servers/status", deps.Live.ListStatus)
		r.Get("/api/servers/{id}/status", deps.Live.DetailStatus)
		r.Get("/api/live", deps.Live.SSE)
		r.Post("/api/servers", serverHandler.Create)
		r.Patch("/api/servers/{id}", serverHandler.Update)
		r.Delete("/api/servers/{id}", serverHandler.Delete)
		r.Post("/api/servers/{id}/token", serverHandler.RotateToken)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		spa.ServeHTTP(w, r)
	})
	return r
}
