package alerting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type recordingAttemptStore struct{ attempts []Attempt }

func (s *recordingAttemptStore) RecordAttempt(_ context.Context, attempt Attempt) (Attempt, error) {
	attempt.ID = int64(len(s.attempts) + 1)
	s.attempts = append(s.attempts, attempt)
	return attempt, nil
}

func TestDispatcherRetriesThreeTimes(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer endpoint.Close()
	store := &recordingAttemptStore{}
	dispatcher := NewDispatcher(store, NewWebhookClient())
	dispatcher.sleep = func(context.Context, time.Duration) error { return nil }
	eventID := int64(5)
	outcome := dispatcher.DispatchNow(context.Background(), DeliveryJob{
		Event: Event{ID: eventID}, Config: WebhookConfig{URL: endpoint.URL, BodyTemplate: `{"status":"{{.Status}}"}`},
		Data: TemplateData{Status: StatusFiring},
	})
	if outcome.Success || len(outcome.Attempts) != 3 || len(store.attempts) != 3 {
		t.Fatalf("outcome=%+v attempts=%+v", outcome, store.attempts)
	}
	for index, attempt := range store.attempts {
		if attempt.Attempt != index+1 || attempt.ResponseStatus == nil || *attempt.ResponseStatus != http.StatusInternalServerError {
			t.Fatalf("attempt=%+v", attempt)
		}
	}
}
