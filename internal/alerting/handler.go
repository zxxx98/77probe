package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"probe.local/monitor/internal/servers"
)

const maskedHeaderValue = "••••••"

type ruleRepository interface {
	CreateRule(context.Context, Rule) (Rule, error)
	ListRules(context.Context) ([]Rule, error)
	GetRule(context.Context, int64) (Rule, error)
	UpdateRule(context.Context, Rule) (Rule, error)
	DeleteRule(context.Context, int64) error
	ListEvents(context.Context, int64, int) ([]Event, error)
	GetWebhook(context.Context) (WebhookConfig, error)
	UpsertWebhook(context.Context, WebhookConfig) (WebhookConfig, error)
}

type webhookDispatcher interface {
	DispatchNow(context.Context, DeliveryJob) DeliveryOutcome
}

type serverReader interface {
	Get(context.Context, int64) (servers.Server, error)
}

type Handler struct {
	repository ruleRepository
	servers    serverReader
	dispatcher webhookDispatcher
}

func NewHandler(repository ruleRepository, servers serverReader, dispatcher webhookDispatcher) *Handler {
	if repository == nil || servers == nil || dispatcher == nil {
		panic("alerting handler requires repository, servers, and dispatcher")
	}
	return &Handler{repository: repository, servers: servers, dispatcher: dispatcher}
}

func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.repository.ListRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	var input ruleCreate
	if !decodeJSON(w, r, &input) {
		return
	}
	rule, err := ruleFromCreate(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := h.servers.Get(r.Context(), rule.ServerID); err != nil {
		handleServerLookupError(w, err)
		return
	}
	created, err := h.repository.CreateRule(r.Context(), rule)
	if err != nil {
		handleRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "id")
	if !ok {
		return
	}
	current, err := h.repository.GetRule(r.Context(), id)
	if err != nil {
		handleRepositoryError(w, err)
		return
	}
	var update ruleUpdate
	if !decodeJSON(w, r, &update) {
		return
	}
	next := applyRuleUpdate(current, update)
	next, err = normalizeRule(next)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := h.servers.Get(r.Context(), next.ServerID); err != nil {
		handleServerLookupError(w, err)
		return
	}
	updated, err := h.repository.UpdateRule(r.Context(), next)
	if err != nil {
		handleRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "id")
	if !ok {
		return
	}
	if err := h.repository.DeleteRule(r.Context(), id); err != nil {
		handleRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	beforeID, limit, err := eventPage(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return
	}
	events, err := h.repository.ListEvents(r.Context(), beforeID, limit)
	if err != nil {
		handleRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	config, err := h.repository.GetWebhook(r.Context())
	if errors.Is(err, ErrNotFound) {
		config = defaultWebhookConfig()
	} else if err != nil {
		handleRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, maskWebhookHeaders(config))
}

func (h *Handler) PutWebhook(w http.ResponseWriter, r *http.Request) {
	var input WebhookConfig
	if !decodeJSON(w, r, &input) {
		return
	}
	current, err := h.repository.GetWebhook(r.Context())
	if errors.Is(err, ErrNotFound) {
		current = defaultWebhookConfig()
	} else if err != nil {
		handleRepositoryError(w, err)
		return
	}
	input.Headers = restoreMaskedHeaders(input.Headers, current.Headers)
	if err := validateWebhook(input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := h.repository.UpsertWebhook(r.Context(), input)
	if err != nil {
		handleRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, maskWebhookHeaders(updated))
}

func (h *Handler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	config, err := h.repository.GetWebhook(r.Context())
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return
	}
	if err != nil {
		handleRepositoryError(w, err)
		return
	}
	if err := validateWebhook(config); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	now := time.Now().UTC()
	outcome := h.dispatcher.DispatchNow(r.Context(), DeliveryJob{
		Config: config, IsTest: true,
		Data: TemplateData{ServerID: 0, ServerName: "Webhook 测试", Metric: MetricCPUUsage, Status: StatusFiring, CurrentValue: 85, Threshold: 80, StartedAt: now, DetailURL: "/"},
	})
	writeJSON(w, http.StatusOK, outcome)
}

type ruleUpdate struct {
	ServerID        *int64    `json:"serverId"`
	Metric          *Metric   `json:"metric"`
	Operator        *Operator `json:"operator"`
	Threshold       *float64  `json:"threshold"`
	DurationSeconds *int      `json:"durationSeconds"`
	RepeatSeconds   *int      `json:"repeatSeconds"`
	Enabled         *bool     `json:"enabled"`
}

type ruleCreate struct {
	ServerID        int64     `json:"serverId"`
	Metric          Metric    `json:"metric"`
	Operator        *Operator `json:"operator"`
	Threshold       *float64  `json:"threshold"`
	DurationSeconds *int      `json:"durationSeconds"`
	RepeatSeconds   *int      `json:"repeatSeconds"`
	Enabled         *bool     `json:"enabled"`
}

func ruleFromCreate(input ruleCreate) (Rule, error) {
	rule := Rule{
		ServerID: input.ServerID, Metric: input.Metric,
		DurationSeconds: 300, RepeatSeconds: 0, Enabled: true,
	}
	if input.Operator != nil {
		rule.Operator = *input.Operator
	}
	if input.Threshold != nil {
		rule.Threshold = *input.Threshold
	}
	if input.DurationSeconds != nil {
		rule.DurationSeconds = *input.DurationSeconds
	}
	if input.RepeatSeconds != nil {
		rule.RepeatSeconds = *input.RepeatSeconds
	}
	if input.Enabled != nil {
		rule.Enabled = *input.Enabled
	}
	return normalizeRule(rule)
}

func applyRuleUpdate(current Rule, update ruleUpdate) Rule {
	if update.ServerID != nil {
		current.ServerID = *update.ServerID
	}
	if update.Metric != nil {
		current.Metric = *update.Metric
	}
	if update.Operator != nil {
		current.Operator = *update.Operator
	}
	if update.Threshold != nil {
		current.Threshold = *update.Threshold
	}
	if update.DurationSeconds != nil {
		current.DurationSeconds = *update.DurationSeconds
	}
	if update.RepeatSeconds != nil {
		current.RepeatSeconds = *update.RepeatSeconds
	}
	if update.Enabled != nil {
		current.Enabled = *update.Enabled
	}
	return current
}

func normalizeRule(rule Rule) (Rule, error) {
	if rule.ServerID < 1 || !validMetric(rule.Metric) {
		return Rule{}, ErrInvalidInput
	}
	if rule.Metric == MetricOffline {
		if rule.Operator != "" && rule.Operator != OperatorGreaterThan || rule.Threshold != 0 {
			return Rule{}, ErrInvalidInput
		}
		rule.Operator, rule.Threshold, rule.DurationSeconds = OperatorGreaterThan, 0, 0
	} else if rule.Metric == MetricDiskFreeBytes {
		if rule.Operator != OperatorLessThan || rule.Threshold <= 0 {
			return Rule{}, ErrInvalidInput
		}
	} else if rule.Operator != OperatorGreaterThan || rule.Threshold < 0 || rule.Threshold > 100 {
		return Rule{}, ErrInvalidInput
	}
	if rule.DurationSeconds < 0 || rule.DurationSeconds > 86400 || rule.RepeatSeconds < 0 || rule.RepeatSeconds > 604800 || (rule.RepeatSeconds != 0 && rule.RepeatSeconds < 300) {
		return Rule{}, ErrInvalidInput
	}
	return rule, nil
}

func defaultWebhookConfig() WebhookConfig {
	return WebhookConfig{Headers: map[string]string{}, BodyTemplate: DefaultBodyTemplate, Enabled: false}
}

func validateWebhook(config WebhookConfig) error {
	endpoint, err := url.Parse(config.URL)
	if err != nil || !endpoint.IsAbs() || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return ErrInvalidInput
	}
	_, err = RenderTemplate(config.BodyTemplate, TemplateData{ServerName: "Webhook 测试", Metric: MetricCPUUsage, Status: StatusFiring, StartedAt: time.Now().UTC()})
	if err != nil {
		return ErrInvalidInput
	}
	return nil
}

func maskWebhookHeaders(config WebhookConfig) WebhookConfig {
	config.Headers = copyHeaders(config.Headers)
	for name := range config.Headers {
		if secretHeader(name) {
			config.Headers[name] = maskedHeaderValue
		}
	}
	return config
}

func restoreMaskedHeaders(input, existing map[string]string) map[string]string {
	result := copyHeaders(input)
	for name, value := range result {
		if value == maskedHeaderValue && secretHeader(name) {
			if existingValue, ok := existing[name]; ok {
				result[name] = existingValue
			}
		}
	}
	return result
}

func copyHeaders(headers map[string]string) map[string]string {
	copy := make(map[string]string, len(headers))
	for name, value := range headers {
		copy[name] = value
	}
	return copy
}

func secretHeader(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "authorization") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key")
}

func eventPage(request *http.Request) (int64, int, error) {
	beforeID, limit := int64(0), 50
	if raw := request.URL.Query().Get("beforeId"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 1 {
			return 0, 0, ErrInvalidInput
		}
		beforeID = value
	}
	if raw := request.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return 0, 0, ErrInvalidInput
		}
		limit = value
	}
	return beforeID, limit, nil
}

func routeID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	value, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || value < 1 {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return 0, false
	}
	return value, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, errors.New("content type must be application/json"))
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, ErrInvalidInput)
		return false
	}
	return true
}

func handleServerLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, servers.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
}

func handleRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, errors.New("internal server error"))
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
