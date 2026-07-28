package history_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"probe.local/monitor/internal/history"
	"probe.local/monitor/internal/servers"
)

func TestHistoryHandlerQueriesAcceptedRangeBoundaries(t *testing.T) {
	fixedNow := time.Date(2026, time.July, 28, 18, 42, 37, 900, time.FixedZone("UTC+0530", 5*60*60+30*60))

	for _, test := range []struct {
		name     string
		rangeKey string
		duration time.Duration
	}{
		{name: "one day", rangeKey: "1d", duration: 24 * time.Hour},
		{name: "seven days", rangeKey: "7d", duration: 7 * 24 * time.Hour},
		{name: "thirty days", rangeKey: "30d", duration: 30 * 24 * time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverCalls := 0
			serverLookup := serverGetFunc(func(_ context.Context, id int64) (servers.Server, error) {
				serverCalls++
				if id != 42 {
					t.Fatalf("server ID = %d, want 42", id)
				}
				return servers.Server{ID: id}, nil
			})
			store := &recordingQueryStore{points: []history.MinuteRecord{}}
			handler := history.NewHandler(store, serverLookup, history.WithClock(func() time.Time { return fixedNow }))

			recorder := serveHistory(handler, "/api/servers/42/history?range="+test.rangeKey)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
			}
			if serverCalls != 1 {
				t.Fatalf("server lookup calls = %d, want 1", serverCalls)
			}
			if store.calls != 1 {
				t.Fatalf("store calls = %d, want 1", store.calls)
			}
			wantTo := fixedNow.UTC().Truncate(time.Minute).Unix()
			wantFrom := fixedNow.Add(-test.duration).UTC().Truncate(time.Minute).Unix()
			if store.serverID != 42 || store.fromUnix != wantFrom || store.toUnix != wantTo {
				t.Fatalf("Query(%d, %d, %d), want Query(42, %d, %d)", store.serverID, store.fromUnix, store.toUnix, wantFrom, wantTo)
			}
			wantBody := historyResponse{FromUnix: wantFrom, ToUnix: wantTo, Points: []history.MinuteRecord{}}
			if got := decodeHistoryResponse(t, recorder); !reflect.DeepEqual(got, wantBody) {
				t.Fatalf("response = %#v, want %#v", got, wantBody)
			}
		})
	}
}

func TestHistoryHandlerRejectsInvalidOrMissingRange(t *testing.T) {
	for _, target := range []string{
		"/api/servers/1/history",
		"/api/servers/1/history?range=1D",
		"/api/servers/1/history?range=%201d",
		"/api/servers/1/history?range=1d%20",
		"/api/servers/1/history?range=24h",
	} {
		t.Run(target, func(t *testing.T) {
			serverCalls := 0
			store := &recordingQueryStore{}
			handler := history.NewHandler(store, serverGetFunc(func(context.Context, int64) (servers.Server, error) {
				serverCalls++
				return servers.Server{}, nil
			}))

			recorder := serveHistory(handler, target)

			assertHistoryError(t, recorder, http.StatusBadRequest, "{\"error\":\"invalid history range\"}\n")
			if serverCalls != 0 || store.calls != 0 {
				t.Fatalf("invalid range made server/store calls = %d/%d", serverCalls, store.calls)
			}
		})
	}
}

func TestHistoryHandlerRejectsInvalidServerID(t *testing.T) {
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			serverCalls := 0
			store := &recordingQueryStore{}
			handler := history.NewHandler(store, serverGetFunc(func(context.Context, int64) (servers.Server, error) {
				serverCalls++
				return servers.Server{}, nil
			}))

			recorder := serveHistory(handler, "/api/servers/"+id+"/history?range=1d")

			assertHistoryError(t, recorder, http.StatusBadRequest, "{\"error\":\"invalid server input\"}\n")
			if serverCalls != 0 || store.calls != 0 {
				t.Fatalf("invalid ID made server/store calls = %d/%d", serverCalls, store.calls)
			}
		})
	}
}

func TestHistoryHandlerChecksServerBeforeQuery(t *testing.T) {
	store := &recordingQueryStore{}
	handler := history.NewHandler(store, serverGetFunc(func(_ context.Context, id int64) (servers.Server, error) {
		if id != 9 {
			t.Fatalf("server ID = %d, want 9", id)
		}
		return servers.Server{}, servers.ErrNotFound
	}))

	recorder := serveHistory(handler, "/api/servers/9/history?range=7d")

	assertHistoryError(t, recorder, http.StatusNotFound, "{\"error\":\"server not found\"}\n")
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

func TestHistoryHandlerHidesServerLookupErrors(t *testing.T) {
	store := &recordingQueryStore{}
	handler := history.NewHandler(store, serverGetFunc(func(context.Context, int64) (servers.Server, error) {
		return servers.Server{}, errors.New("database credentials leaked")
	}))

	recorder := serveHistory(handler, "/api/servers/1/history?range=1d")

	assertHistoryError(t, recorder, http.StatusInternalServerError, "{\"error\":\"internal server error\"}\n")
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0", store.calls)
	}
}

func TestHistoryHandlerMapsStoreInvalidRangeToBadRequest(t *testing.T) {
	store := &recordingQueryStore{err: &history.InvalidRangeError{FromUnix: 2, ToUnix: 1}}
	handler := history.NewHandler(store, existingServerLookup())

	recorder := serveHistory(handler, "/api/servers/1/history?range=30d")

	assertHistoryError(t, recorder, http.StatusBadRequest, "{\"error\":\"invalid history range\"}\n")
}

func TestHistoryHandlerHidesStoreErrors(t *testing.T) {
	store := &recordingQueryStore{err: errors.New("malformed stored payload")}
	handler := history.NewHandler(store, existingServerLookup())

	recorder := serveHistory(handler, "/api/servers/1/history?range=1d")

	assertHistoryError(t, recorder, http.StatusInternalServerError, "{\"error\":\"internal server error\"}\n")
}

func TestHistoryHandlerPreservesPointOrder(t *testing.T) {
	points := []history.MinuteRecord{
		{ServerID: 4, MinuteUnix: 300, Payload: history.MinutePayload{CPUUsage: history.Pair{Average: 30}, Disks: []history.DiskMinute{}}},
		{ServerID: 4, MinuteUnix: 100, Payload: history.MinutePayload{CPUUsage: history.Pair{Average: 10}, Disks: []history.DiskMinute{}}},
	}
	handler := history.NewHandler(&recordingQueryStore{points: points}, existingServerLookup())

	recorder := serveHistory(handler, "/api/servers/4/history?range=1d")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if got := decodeHistoryResponse(t, recorder).Points; !reflect.DeepEqual(got, points) {
		t.Fatalf("points = %#v, want %#v", got, points)
	}
}

func TestHistoryHandlerEncodesNilStoreResultAsEmptyArray(t *testing.T) {
	handler := history.NewHandler(&recordingQueryStore{points: nil}, existingServerLookup())

	recorder := serveHistory(handler, "/api/servers/1/history?range=1d")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	response := decodeHistoryResponse(t, recorder)
	if response.Points == nil || len(response.Points) != 0 {
		t.Fatalf("points = %#v, want allocated empty slice", response.Points)
	}
}

type historyResponse struct {
	FromUnix int64                  `json:"fromUnix"`
	ToUnix   int64                  `json:"toUnix"`
	Points   []history.MinuteRecord `json:"points"`
}

type recordingQueryStore struct {
	points   []history.MinuteRecord
	err      error
	calls    int
	serverID int64
	fromUnix int64
	toUnix   int64
}

func (s *recordingQueryStore) Query(_ context.Context, serverID, fromUnix, toUnix int64) ([]history.MinuteRecord, error) {
	s.calls++
	s.serverID = serverID
	s.fromUnix = fromUnix
	s.toUnix = toUnix
	return s.points, s.err
}

type serverGetFunc func(context.Context, int64) (servers.Server, error)

func (f serverGetFunc) Get(ctx context.Context, id int64) (servers.Server, error) {
	return f(ctx, id)
}

func existingServerLookup() serverGetFunc {
	return func(_ context.Context, id int64) (servers.Server, error) {
		return servers.Server{ID: id}, nil
	}
}

func serveHistory(handler *history.Handler, target string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	router.Get("/api/servers/{id}/history", handler.Get)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func decodeHistoryResponse(t *testing.T, recorder *httptest.ResponseRecorder) historyResponse {
	t.Helper()
	var response historyResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}

func assertHistoryError(t *testing.T, recorder *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if recorder.Code != status || recorder.Body.String() != body {
		t.Fatalf("status/body = %d/%q, want %d/%q", recorder.Code, recorder.Body.String(), status, body)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
}
