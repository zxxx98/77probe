package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"probe.local/monitor/internal/agent"
)

type config struct {
	serverURL string
	token     string
	version   string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context, getenv func(string) string) error {
	configuration, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	client, err := agent.NewReportClient(configuration.serverURL, configuration.token)
	if err != nil {
		return err
	}
	collector := agent.Collector{
		Source:       agent.NewGopsutilSource(),
		AgentVersion: configuration.version,
		Now:          time.Now,
	}
	return (agent.Runner{Collector: collector, Client: client}).Run(ctx)
}

func loadConfig(getenv func(string) string) (config, error) {
	serverURL := strings.TrimSpace(getenv("TINYPROBE_SERVER_URL"))
	if err := agent.ValidateReportEndpoint(serverURL); err != nil {
		return config{}, fmt.Errorf("TINYPROBE_SERVER_URL: %w", err)
	}
	token := getenv("TINYPROBE_AGENT_TOKEN")
	if strings.TrimSpace(token) == "" {
		return config{}, fmt.Errorf("TINYPROBE_AGENT_TOKEN is required")
	}
	version := strings.TrimSpace(getenv("TINYPROBE_AGENT_VERSION"))
	if version == "" {
		version = "dev"
	}
	return config{serverURL: serverURL, token: token, version: version}, nil
}
