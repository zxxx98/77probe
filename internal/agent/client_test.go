package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"probe.local/monitor/internal/protocol"
)

func TestReportClientPostsReportToCompleteEndpoint(t *testing.T) {
	var method string
	var path string
	var authorization string
	var contentType string
	var received protocol.AgentReport
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		authorization = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewReportClient(server.URL+"/custom/report", "tp_secret")
	if err != nil {
		t.Fatalf("NewReportClient() error = %v", err)
	}
	report := protocol.AgentReport{CollectedAtUnix: 123, AgentVersion: "test", Host: protocol.HostInfo{Hostname: "tiny"}}
	if err := client.Send(context.Background(), report); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if method != http.MethodPost || path != "/custom/report" {
		t.Fatalf("request = %s %s, want POST /custom/report", method, path)
	}
	if authorization != "Bearer tp_secret" {
		t.Fatalf("Authorization = %q", authorization)
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if received.CollectedAtUnix != 123 || received.AgentVersion != "test" || received.Host.Hostname != "tiny" {
		t.Fatalf("received report = %+v", received)
	}
	if client.httpClient.Timeout != 5*time.Second {
		t.Fatalf("HTTP timeout = %s, want 5s", client.httpClient.Timeout)
	}
}

func TestReportClientReturnsErrorForNonSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewReportClient(server.URL+"/api/agent/v1/report", "bad-token")
	if err != nil {
		t.Fatalf("NewReportClient() error = %v", err)
	}
	if err := client.Send(context.Background(), protocol.AgentReport{}); err == nil {
		t.Fatal("Send() error = nil, want non-2xx error")
	}
}

func TestNewReportClientRejectsInvalidEndpointAndEmptyToken(t *testing.T) {
	for _, endpoint := range []string{"", "monitor.example/report", "/api/agent/v1/report", "ftp://monitor.example/report", "://bad"} {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := NewReportClient(endpoint, "tp_secret"); err == nil {
				t.Fatalf("NewReportClient(%q, token) error = nil", endpoint)
			}
		})
	}
	if _, err := NewReportClient("https://monitor.example/api/agent/v1/report", ""); err == nil {
		t.Fatal("NewReportClient(valid endpoint, empty token) error = nil")
	}
}
