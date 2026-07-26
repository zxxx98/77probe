package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Dependencies struct{}

func NewRouter(_ Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/health", health)
	return r
}
