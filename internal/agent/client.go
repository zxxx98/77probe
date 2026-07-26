package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"probe.local/monitor/internal/protocol"
)

type ReportClient struct {
	endpoint   *url.URL
	token      string
	httpClient *http.Client
}

func NewReportClient(endpoint, token string) (*ReportClient, error) {
	parsed, err := validateReportEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("agent token is required")
	}
	return &ReportClient{
		endpoint: parsed,
		token:    token,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func ValidateReportEndpoint(endpoint string) error {
	_, err := validateReportEndpoint(endpoint)
	return err
}

func validateReportEndpoint(endpoint string) (*url.URL, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	if !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("server URL must be an absolute http or https URL")
	}
	return parsed, nil
}

func (c *ReportClient) Send(ctx context.Context, report protocol.AgentReport) error {
	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode agent report: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create report request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send agent report: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("send agent report: status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
