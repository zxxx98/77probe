package live

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"probe.local/monitor/internal/protocol"
	"probe.local/monitor/internal/servers"
)

const maxReportBytes = 256 * 1024

type HandlerOption func(*Handler)

type Handler struct {
	servers            *servers.Service
	store              *Store
	hub                *Hub
	now                func() time.Time
	newHeartbeatTicker func(time.Duration) Ticker
	coordinator        *Coordinator
}

func NewHandler(coordinator *Coordinator, options ...HandlerOption) *Handler {
	if coordinator == nil || coordinator.serverService == nil || coordinator.store == nil || coordinator.hub == nil {
		panic("live handler requires a coordinator")
	}
	handler := &Handler{
		servers: coordinator.serverService,
		store:   coordinator.store,
		hub:     coordinator.hub,
		now:     time.Now,
		newHeartbeatTicker: func(interval time.Duration) Ticker {
			return realTicker{Ticker: time.NewTicker(interval)}
		},
		coordinator: coordinator,
	}
	for _, option := range options {
		option(handler)
	}
	return handler
}

func WithHandlerClock(now func() time.Time) HandlerOption {
	return func(handler *Handler) { handler.now = now }
}

func WithHeartbeatTicker(factory func(time.Duration) Ticker) HandlerOption {
	return func(handler *Handler) { handler.newHeartbeatTicker = factory }
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeLiveError(w, http.StatusUnauthorized, servers.ErrInvalidToken)
		return
	}
	_, err := h.servers.AuthenticateToken(r.Context(), token)
	if errors.Is(err, servers.ErrInvalidToken) {
		writeLiveError(w, http.StatusUnauthorized, err)
		return
	}
	if errors.Is(err, servers.ErrDisabled) {
		writeLiveError(w, http.StatusForbidden, err)
		return
	}
	if err != nil {
		writeLiveError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}

	var report protocol.AgentReport
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	if err := validateReport(report); err != nil {
		writeLiveError(w, http.StatusBadRequest, err)
		return
	}
	receivedAt := h.now().UTC()
	_, err = h.coordinator.Accept(r.Context(), token, report, receivedAt, sourceIP(r.RemoteAddr))
	if errors.Is(err, servers.ErrInvalidToken) {
		writeLiveError(w, http.StatusUnauthorized, err)
		return
	}
	if errors.Is(err, servers.ErrDisabled) {
		writeLiveError(w, http.StatusForbidden, err)
		return
	}
	if err != nil {
		writeLiveError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListStatus(w http.ResponseWriter, r *http.Request) {
	registered, err := h.servers.List(r.Context())
	if err != nil {
		writeLiveError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	statuses := make([]Snapshot, 0, len(registered))
	for _, server := range registered {
		statuses = append(statuses, h.snapshotFor(server))
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Online != statuses[j].Online {
			return !statuses[i].Online
		}
		return statuses[i].ServerName < statuses[j].ServerName
	})
	writeLiveJSON(w, http.StatusOK, statuses)
}

func (h *Handler) DetailStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeLiveError(w, http.StatusBadRequest, servers.ErrInvalidInput)
		return
	}
	server, err := h.servers.Get(r.Context(), id)
	if errors.Is(err, servers.ErrNotFound) {
		writeLiveError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeLiveError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	writeLiveJSON(w, http.StatusOK, h.snapshotFor(server))
}

func (h *Handler) SSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeLiveError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	events, cancel := h.hub.Subscribe()
	defer cancel()
	heartbeat := h.newHeartbeatTicker(15 * time.Second)
	defer heartbeat.Stop()
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C():
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) snapshotFor(server servers.Server) Snapshot {
	snapshot, ok := h.store.Get(server.ID)
	if !ok {
		return Snapshot{ServerID: server.ID, ServerName: server.Name}
	}
	snapshot.ServerID = server.ID
	snapshot.ServerName = server.Name
	return snapshot
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	returnToken := ""
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		returnToken = parts[1]
	}
	return returnToken, returnToken != ""
}

func sourceIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return strings.Trim(remoteAddr, "[]")
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeLiveError(w, http.StatusRequestEntityTooLarge, errors.New("report body too large"))
		return
	}
	writeLiveError(w, http.StatusBadRequest, errInvalidReport)
}

func writeLiveJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeLiveError(w http.ResponseWriter, status int, err error) {
	writeLiveJSON(w, status, map[string]string{"error": err.Error()})
}
