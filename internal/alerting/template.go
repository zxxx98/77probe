package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
	"time"
)

type TemplateData struct {
	EventID      int64
	ServerID     int64
	ServerName   string
	Metric       Metric
	Status       Status
	CurrentValue float64
	Threshold    float64
	StartedAt    time.Time
	EndedAt      *time.Time
	DetailURL    string
}

const DefaultBodyTemplate = `{
  "server": {{json .ServerName}},
  "metric": "{{.Metric}}",
  "status": "{{.Status}}",
  "currentValue": {{.CurrentValue}},
  "threshold": {{.Threshold}},
  "startedAt": "{{.StartedAt.Format "2006-01-02T15:04:05Z07:00"}}",
  "detailUrl": {{json .DetailURL}}
}`

func RenderTemplate(templateText string, data TemplateData) ([]byte, error) {
	templateValue, err := template.New("webhook").Option("missingkey=error").Funcs(template.FuncMap{
		"json": func(value any) (string, error) {
			encoded, err := json.Marshal(value)
			return string(encoded), err
		},
	}).Parse(templateText)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := templateValue.Execute(&output, data); err != nil {
		return nil, err
	}
	body := output.Bytes()
	if !json.Valid(body) {
		return nil, fmt.Errorf("webhook template must render valid JSON")
	}
	return body, nil
}
