package history

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"probe.local/monitor/internal/servers"
)

type queryStore interface {
	Query(context.Context, int64, int64, int64) ([]MinuteRecord, error)
}

type serverLookup interface {
	Get(context.Context, int64) (servers.Server, error)
}

type Handler struct {
	store   queryStore
	servers serverLookup
	now     func() time.Time
}

type HandlerOption func(*Handler)

func WithClock(now func() time.Time) HandlerOption {
	return func(handler *Handler) {
		handler.now = now
	}
}

func NewHandler(store queryStore, servers serverLookup, options ...HandlerOption) *Handler {
	handler := &Handler{store: store, servers: servers, now: time.Now}
	for _, option := range options {
		option(handler)
	}
	return handler
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	duration, ok := historyDuration(r.URL.Query().Get("range"))
	if !ok {
		writeHistoryError(w, http.StatusBadRequest, ErrInvalidRange)
		return
	}
	serverID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || serverID < 1 {
		writeHistoryError(w, http.StatusBadRequest, servers.ErrInvalidInput)
		return
	}

	if _, err := h.servers.Get(r.Context(), serverID); err != nil {
		if errors.Is(err, servers.ErrNotFound) {
			writeHistoryError(w, http.StatusNotFound, servers.ErrNotFound)
		} else {
			writeHistoryError(w, http.StatusInternalServerError, errors.New("internal server error"))
		}
		return
	}

	to := h.now().UTC().Truncate(time.Minute)
	from := to.Add(-duration)
	points, err := h.store.Query(r.Context(), serverID, from.Unix(), to.Unix())
	if err != nil {
		if errors.Is(err, ErrInvalidRange) {
			writeHistoryError(w, http.StatusBadRequest, ErrInvalidRange)
		} else {
			writeHistoryError(w, http.StatusInternalServerError, errors.New("internal server error"))
		}
		return
	}
	if points == nil {
		points = make([]MinuteRecord, 0)
	}

	writeHistoryJSON(w, http.StatusOK, struct {
		FromUnix int64          `json:"fromUnix"`
		ToUnix   int64          `json:"toUnix"`
		Points   []MinuteRecord `json:"points"`
	}{FromUnix: from.Unix(), ToUnix: to.Unix(), Points: points})
}

func historyDuration(value string) (time.Duration, bool) {
	switch value {
	case "1d":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func writeHistoryJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeHistoryError(w http.ResponseWriter, status int, err error) {
	writeHistoryJSON(w, status, map[string]string{"error": err.Error()})
}
