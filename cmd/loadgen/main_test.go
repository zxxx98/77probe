package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"probe.local/monitor/internal/protocol"
)

func TestValidateConfigIntervalBoundaries(t *testing.T) {
	base := config{agents: 1, duration: time.Second}
	tests := []struct {
		name        string
		interval    time.Duration
		allowFast   bool
		wantError   string
		wantAllowed bool
	}{
		{name: "zero", interval: 0, wantError: "greater than zero"},
		{name: "negative", interval: -time.Millisecond, wantError: "greater than zero"},
		{name: "below default without guard", interval: 5*time.Second - time.Nanosecond, wantError: "-allow-fast"},
		{name: "default without guard", interval: 5 * time.Second, wantAllowed: true},
		{name: "slower without guard", interval: 6 * time.Second, wantAllowed: true},
		{name: "below floor with guard", interval: 100*time.Millisecond - time.Nanosecond, allowFast: true, wantError: "100ms"},
		{name: "floor with guard", interval: 100 * time.Millisecond, allowFast: true, wantAllowed: true},
		{name: "below default with guard", interval: 250 * time.Millisecond, allowFast: true, wantAllowed: true},
		{name: "default with guard", interval: 5 * time.Second, allowFast: true, wantAllowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := base
			configuration.interval = test.interval
			configuration.allowFast = test.allowFast
			err := validateConfig(configuration)
			if test.wantAllowed {
				if err != nil {
					t.Fatalf("validateConfig() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateConfig() error = %v, want text %q", err, test.wantError)
			}
		})
	}
}

func TestRunUsesConfiguredAcceleratedInterval(t *testing.T) {
	requests := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tokenFile := filepath.Join(t.TempDir(), "tokens.txt")
	if err := os.WriteFile(tokenFile, []byte("tp_one\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, config{
			baseURL:   server.URL,
			tokenFile: tokenFile,
			agents:    1,
			duration:  2 * time.Second,
			interval:  100 * time.Millisecond,
			allowFast: true,
		})
	}()

	for batch := 1; batch <= 3; batch++ {
		select {
		case <-requests:
		case <-time.After(time.Second):
			cancel()
			t.Fatalf("received %d batches, want at least 3", batch-1)
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context cancellation", err)
	}
}

func TestLoadTokensSelectsRequestedNonEmptyTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.txt")
	if err := os.WriteFile(path, []byte("tp_one\n\n tp_two \ntp_three\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tokens, err := loadTokens(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"tp_one", "tp_two"}; !reflect.DeepEqual(tokens, want) {
		t.Fatalf("tokens=%q, want %q", tokens, want)
	}
	if _, err := loadTokens(path, 4); err == nil {
		t.Fatal("loadTokens accepted fewer tokens than requested agents")
	}
}

func TestReportForAgentIsDeterministicAndBounded(t *testing.T) {
	now := time.Unix(1_753_588_800, 0).UTC()
	first := reportForAgent(0, now)
	if duplicate := reportForAgent(0, now); !reflect.DeepEqual(first, duplicate) {
		t.Fatalf("report is not deterministic:\nfirst=%+v\nsecond=%+v", first, duplicate)
	}
	second := reportForAgent(1, now)
	if first.Host.Hostname == second.Host.Hostname {
		t.Fatalf("hostnames are not distinct: %q", first.Host.Hostname)
	}
	if first.CollectedAtUnix != now.Unix() || first.AgentVersion != "loadgen" {
		t.Fatalf("unexpected report metadata: %+v", first)
	}
	if first.CPU.UsagePercent < 0 || first.CPU.UsagePercent > 100 {
		t.Fatalf("CPU usage out of bounds: %f", first.CPU.UsagePercent)
	}
	if first.Memory.UsedBytes > first.Memory.TotalBytes {
		t.Fatalf("memory out of bounds: %+v", first.Memory)
	}
	if len(first.Disks) != 1 || first.Disks[0].UsedBytes > first.Disks[0].TotalBytes {
		t.Fatalf("disk out of bounds: %+v", first.Disks)
	}
}

func TestSendBatchUsesAgentProtocolAndRejectsNon2xx(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/v1/report" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		requests++
		wantToken := "Bearer tp_one"
		if requests == 2 {
			wantToken = "Bearer tp_two"
		}
		if got := r.Header.Get("Authorization"); got != wantToken {
			t.Fatalf("Authorization=%q, want %q", got, wantToken)
		}
		var report protocol.AgentReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Fatal(err)
		}
		if report.Host.Hostname == "" || report.AgentVersion != "loadgen" {
			t.Fatalf("report=%+v", report)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := sendBatch(
		context.Background(),
		server.Client(),
		server.URL,
		[]string{"tp_one", "tp_two"},
		time.Unix(1_753_588_800, 0).UTC(),
	); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests=%d, want 2", requests)
	}

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer failing.Close()
	if err := sendBatch(
		context.Background(),
		failing.Client(),
		failing.URL,
		[]string{"tp_one"},
		time.Unix(1_753_588_800, 0).UTC(),
	); err == nil {
		t.Fatal("sendBatch accepted a non-2xx response")
	}
}
