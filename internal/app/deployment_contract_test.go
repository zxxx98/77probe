package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentFilesDescribeSingleNonRootService(t *testing.T) {
	root := filepath.Join("..", "..")
	dockerfile := readDeploymentFile(t, filepath.Join(root, "deploy", "Dockerfile"))
	for _, required := range []string{
		"FROM node:",
		"pnpm install --frozen-lockfile",
		"pnpm build",
		"FROM golang:",
		"go test ./...",
		"GOARCH=amd64",
		"GOARCH=arm64",
		"FROM alpine:",
		"USER tinyprobe",
		"EXPOSE 8080",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Errorf("Dockerfile missing %q", required)
		}
	}

	compose := readDeploymentFile(t, filepath.Join(root, "docker-compose.yml"))
	for _, required := range []string{
		"tinyprobe:",
		"dockerfile: deploy/Dockerfile",
		"8080:8080",
		"tinyprobe-data:/data",
		"restart: unless-stopped",
		"/api/health",
		"tinyprobe-data:",
	} {
		if !strings.Contains(compose, required) {
			t.Errorf("docker-compose.yml missing %q", required)
		}
	}
}

func readDeploymentFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
