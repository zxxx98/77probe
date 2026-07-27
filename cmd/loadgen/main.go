package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"probe.local/monitor/internal/protocol"
)

const reportInterval = 5 * time.Second

type config struct {
	baseURL   string
	tokenFile string
	agents    int
	duration  time.Duration
}

func main() {
	configuration := config{}
	flag.StringVar(&configuration.baseURL, "base-url", "http://127.0.0.1:8080", "TinyProbe server base URL")
	flag.StringVar(&configuration.tokenFile, "token-file", "tokens.txt", "file containing one Agent token per line")
	flag.IntVar(&configuration.agents, "agents", 10, "number of Agents to simulate")
	flag.DurationVar(&configuration.duration, "duration", time.Minute, "load generation duration")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, configuration); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context, configuration config) error {
	if configuration.agents <= 0 {
		return fmt.Errorf("agents must be greater than zero")
	}
	if configuration.duration <= 0 {
		return fmt.Errorf("duration must be greater than zero")
	}
	if _, err := reportEndpoint(configuration.baseURL); err != nil {
		return err
	}
	tokens, err := loadTokens(configuration.tokenFile, configuration.agents)
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithTimeout(ctx, configuration.duration)
	defer cancel()
	client := &http.Client{Timeout: 10 * time.Second}
	if err := sendBatch(runCtx, client, configuration.baseURL, tokens, time.Now().UTC()); err != nil {
		return err
	}

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-runCtx.Done():
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				return nil
			}
			return runCtx.Err()
		case collectedAt := <-ticker.C:
			if err := sendBatch(runCtx, client, configuration.baseURL, tokens, collectedAt.UTC()); err != nil {
				return err
			}
		}
	}
}

func loadTokens(path string, agents int) ([]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}
	tokens := make([]string, 0, agents)
	for _, line := range strings.Split(string(body), "\n") {
		token := strings.TrimSpace(line)
		if token == "" {
			continue
		}
		tokens = append(tokens, token)
		if len(tokens) == agents {
			return tokens, nil
		}
	}
	return nil, fmt.Errorf("token file contains %d usable tokens, need %d", len(tokens), agents)
}

func sendBatch(ctx context.Context, client *http.Client, baseURL string, tokens []string, collectedAt time.Time) error {
	endpoint, err := reportEndpoint(baseURL)
	if err != nil {
		return err
	}
	for index, token := range tokens {
		body, err := json.Marshal(reportForAgent(index, collectedAt))
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("agent %d report: %w", index+1, err)
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		closeErr := response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("agent %d response: %w", index+1, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("agent %d response close: %w", index+1, closeErr)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("agent %d report returned %s: %s", index+1, response.Status, strings.TrimSpace(string(responseBody)))
		}
	}
	return nil
}

func reportEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("base URL must be an absolute http or https URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/agent/v1/report"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func reportForAgent(index int, collectedAt time.Time) protocol.AgentReport {
	sequence := uint64(index + 1)
	totalMemory := uint64(8 * 1024 * 1024 * 1024)
	totalDisk := uint64(128 * 1024 * 1024 * 1024)
	return protocol.AgentReport{
		CollectedAtUnix: collectedAt.Unix(),
		AgentVersion:    "loadgen",
		Host: protocol.HostInfo{
			Hostname:        fmt.Sprintf("loadgen-%02d", index+1),
			OS:              "linux",
			Platform:        "tinyprobe-loadgen",
			PlatformVersion: "1",
			KernelVersion:   "simulated",
			Architecture:    "amd64",
			CPUModel:        "TinyProbe virtual CPU",
			CPUCores:        2 + index%7,
			PrimaryIP:       fmt.Sprintf("192.0.2.%d", index+10),
			BootTimeUnix:    collectedAt.Add(-time.Duration(index+1) * time.Hour).Unix(),
			UptimeSeconds:   uint64((index + 1) * 3600),
		},
		CPU: protocol.CPUStats{
			UsagePercent: float64(10 + index*7%80),
			Load1:        float64(index+1) / 10,
			Load5:        float64(index+1) / 12,
			Load15:       float64(index+1) / 15,
		},
		Memory: protocol.MemoryStats{
			TotalBytes:     totalMemory,
			UsedBytes:      totalMemory * (30 + sequence*3%60) / 100,
			SwapTotalBytes: 2 * 1024 * 1024 * 1024,
			SwapUsedBytes:  sequence * 32 * 1024 * 1024,
		},
		Disks: []protocol.DiskStats{{
			Mountpoint: "/",
			TotalBytes: totalDisk,
			UsedBytes:  totalDisk * (25 + sequence*4%65) / 100,
		}},
		DiskIO: protocol.DiskIOStats{
			ReadBytesPerSecond:  sequence * 1024 * 1024,
			WriteBytesPerSecond: sequence * 512 * 1024,
		},
		Network: protocol.NetworkStats{
			Interface:              "eth0",
			UploadBytesPerSecond:   sequence * 128 * 1024,
			DownloadBytesPerSecond: sequence * 256 * 1024,
			TotalUploadBytes:       sequence * 1024 * 1024 * 1024,
			TotalDownloadBytes:     sequence * 2 * 1024 * 1024 * 1024,
		},
	}
}
