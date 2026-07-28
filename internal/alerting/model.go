package alerting

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid alerting input")
	ErrNotFound     = errors.New("alerting record not found")
	ErrQueueFull    = errors.New("alert delivery queue is full")
)

type Metric string

const (
	MetricOffline       Metric = "offline"
	MetricCPUUsage      Metric = "cpu_usage"
	MetricMemoryUsage   Metric = "memory_usage"
	MetricDiskUsage     Metric = "disk_usage"
	MetricDiskFreeBytes Metric = "disk_free_bytes"
)

type Operator string

const (
	OperatorGreaterThan Operator = "gt"
	OperatorLessThan    Operator = "lt"
)

type Status string

const (
	StatusNormal    Status = "normal"
	StatusPending   Status = "pending"
	StatusFiring    Status = "firing"
	StatusRecovered Status = "recovered"
)

type Rule struct {
	ID              int64     `json:"id"`
	ServerID        int64     `json:"serverId"`
	Metric          Metric    `json:"metric"`
	Operator        Operator  `json:"operator"`
	Threshold       float64   `json:"threshold"`
	DurationSeconds int       `json:"durationSeconds"`
	RepeatSeconds   int       `json:"repeatSeconds"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	State           Status    `json:"state"`
}

type State struct {
	RuleID         int64      `json:"ruleId"`
	Status         Status     `json:"status"`
	PendingSince   *time.Time `json:"pendingSince,omitempty"`
	FiringSince    *time.Time `json:"firingSince,omitempty"`
	LastNotifiedAt *time.Time `json:"lastNotifiedAt,omitempty"`
	LastValue      *float64   `json:"lastValue,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Event struct {
	ID           int64      `json:"id"`
	RuleID       int64      `json:"ruleId"`
	ServerID     int64      `json:"serverId"`
	ServerName   string     `json:"serverName"`
	Metric       Metric     `json:"metric"`
	Status       Status     `json:"status"`
	CurrentValue *float64   `json:"currentValue,omitempty"`
	Threshold    *float64   `json:"threshold,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	EndedAt      *time.Time `json:"endedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	Attempts     []Attempt  `json:"attempts,omitempty"`
}

type WebhookConfig struct {
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers"`
	BodyTemplate string            `json:"bodyTemplate"`
	Enabled      bool              `json:"enabled"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type Attempt struct {
	ID             int64     `json:"id"`
	EventID        *int64    `json:"eventId,omitempty"`
	IsTest         bool      `json:"isTest"`
	Attempt        int       `json:"attempt"`
	ResponseStatus *int      `json:"responseStatus,omitempty"`
	ErrorText      string    `json:"errorText"`
	SentAt         time.Time `json:"sentAt"`
}

func validMetric(metric Metric) bool {
	switch metric {
	case MetricOffline, MetricCPUUsage, MetricMemoryUsage, MetricDiskUsage, MetricDiskFreeBytes:
		return true
	default:
		return false
	}
}

func validOperator(operator Operator) bool {
	return operator == OperatorGreaterThan || operator == OperatorLessThan
}

func validStatus(status Status) bool {
	switch status {
	case StatusNormal, StatusPending, StatusFiring, StatusRecovered:
		return true
	default:
		return false
	}
}
