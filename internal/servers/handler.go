package servers

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	servers, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	server, token, err := h.service.Create(r.Context(), request.Name)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tokenResponse{Server: server, Token: token})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := serverID(w, r)
	if !ok {
		return
	}
	var request struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	server, err := h.service.Update(r.Context(), id, request.Name, request.Enabled)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, server)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := serverID(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RotateToken(w http.ResponseWriter, r *http.Request) {
	id, ok := serverID(w, r)
	if !ok {
		return
	}
	token, err := h.service.RotateToken(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	server, err := h.service.Get(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tokenResponse{Server: server, Token: token})
}

type tokenResponse struct {
	Server Server `json:"server"`
	Token  string `json:"token"`
}

func serverID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return 0, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("content type must be application/json"))
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return false
	}
	return true
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, ErrServerLimit):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
