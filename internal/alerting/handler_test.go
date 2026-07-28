package alerting

import "testing"

func TestRuleCreateDefaultsResourceDurationAndAllowsExplicitZero(t *testing.T) {
	threshold := 85.0
	operator := OperatorGreaterThan
	rule, err := ruleFromCreate(ruleCreate{ServerID: 1, Metric: MetricCPUUsage, Operator: &operator, Threshold: &threshold})
	if err != nil || rule.DurationSeconds != 300 || !rule.Enabled {
		t.Fatalf("rule=%+v err=%v", rule, err)
	}
	zero := 0
	rule, err = ruleFromCreate(ruleCreate{ServerID: 1, Metric: MetricCPUUsage, Operator: &operator, Threshold: &threshold, DurationSeconds: &zero})
	if err != nil || rule.DurationSeconds != 0 {
		t.Fatalf("rule=%+v err=%v", rule, err)
	}
}

func TestWebhookValidationRequiresJSONAndAbsoluteURL(t *testing.T) {
	config := WebhookConfig{URL: "https://example.test/hook", BodyTemplate: `{"server":{{json .ServerName}}}`}
	if err := validateWebhook(config); err != nil {
		t.Fatal(err)
	}
	config.URL = "/hook"
	if err := validateWebhook(config); err == nil {
		t.Fatal("expected relative URL rejection")
	}
}
