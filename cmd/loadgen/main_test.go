package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"probe.local/monitor/internal/protocol"
)

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
