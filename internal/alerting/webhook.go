package alerting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxAttemptErrorLength = 2048

type AttemptResult struct {
	ResponseStatus *int
	ErrorText      string
}

func (result AttemptResult) Success() bool {
	return result.ResponseStatus != nil && *result.ResponseStatus >= http.StatusOK && *result.ResponseStatus < http.StatusMultipleChoices && result.ErrorText == ""
}

type WebhookClient struct {
	client *http.Client
}

func NewWebhookClient() *WebhookClient {
	return &WebhookClient{client: &http.Client{Timeout: 10 * time.Second}}
}

func (c *WebhookClient) Send(ctx context.Context, config WebhookConfig, body []byte) AttemptResult {
	endpoint, err := url.Parse(config.URL)
	if err != nil || endpoint == nil || !endpoint.IsAbs() || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return AttemptResult{ErrorText: "webhook URL must be an absolute http or https URL"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return AttemptResult{ErrorText: limitAttemptError(err.Error())}
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range config.Headers {
		request.Header.Set(name, value)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return AttemptResult{ErrorText: limitAttemptError(err.Error())}
	}
	defer response.Body.Close()
	status := response.StatusCode
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return AttemptResult{ResponseStatus: &status}
	}
	bodyText, _ := io.ReadAll(io.LimitReader(response.Body, maxAttemptErrorLength))
	message := strings.TrimSpace(string(bodyText))
	if message == "" {
		message = fmt.Sprintf("webhook returned HTTP %d", status)
	}
	return AttemptResult{ResponseStatus: &status, ErrorText: limitAttemptError(message)}
}

func limitAttemptError(value string) string {
	if len(value) <= maxAttemptErrorLength {
		return value
	}
	return value[:maxAttemptErrorLength]
}
