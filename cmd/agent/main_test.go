package main

import "testing"

func TestLoadConfigRequiresAbsoluteHTTPURLAndToken(t *testing.T) {
	valid := map[string]string{
		"TINYPROBE_SERVER_URL":  "https://monitor.example/api/agent/v1/report",
		"TINYPROBE_AGENT_TOKEN": "tp_secret",
	}
	config, err := loadConfig(func(key string) string { return valid[key] })
	if err != nil {
		t.Fatalf("loadConfig(valid) error = %v", err)
	}
	if config.serverURL != valid["TINYPROBE_SERVER_URL"] || config.token != "tp_secret" || config.version != "dev" {
		t.Fatalf("loadConfig(valid) = %+v", config)
	}

	for _, serverURL := range []string{"", "monitor.example/report", "/api/agent/v1/report", "ftp://monitor.example/report"} {
		t.Run(serverURL, func(t *testing.T) {
			env := map[string]string{"TINYPROBE_SERVER_URL": serverURL, "TINYPROBE_AGENT_TOKEN": "tp_secret"}
			if _, err := loadConfig(func(key string) string { return env[key] }); err == nil {
				t.Fatalf("loadConfig(server URL %q) error = nil", serverURL)
			}
		})
	}

	missingToken := map[string]string{"TINYPROBE_SERVER_URL": valid["TINYPROBE_SERVER_URL"]}
	if _, err := loadConfig(func(key string) string { return missingToken[key] }); err == nil {
		t.Fatal("loadConfig(empty token) error = nil")
	}
}

func TestLoadConfigUsesConfiguredAgentVersion(t *testing.T) {
	env := map[string]string{
		"TINYPROBE_SERVER_URL":    "http://127.0.0.1:8080/custom/report",
		"TINYPROBE_AGENT_TOKEN":   "tp_secret",
		"TINYPROBE_AGENT_VERSION": "1.2.3",
	}
	config, err := loadConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if config.version != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", config.version)
	}
}
