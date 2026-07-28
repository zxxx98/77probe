package alerting

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRenderTemplateEscapesJSONValues(t *testing.T) {
	body, err := RenderTemplate(`{"server":{{json .ServerName}},"status":"{{.Status}}"}`, TemplateData{ServerName: `home"lab`, Status: StatusFiring, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value["server"] != `home"lab` {
		t.Fatalf("server=%q", value["server"])
	}
}

func TestRenderTemplateRejectsInvalidJSON(t *testing.T) {
	if _, err := RenderTemplate(`{"server": {{.ServerName}}`, TemplateData{}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
