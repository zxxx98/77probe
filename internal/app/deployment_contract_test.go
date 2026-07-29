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
		"23333:8080",
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

func TestDockerfileBuildsServerForBuildKitTargetArchitecture(t *testing.T) {
	root := filepath.Join("..", "..")
	dockerfile := readDeploymentFile(t, filepath.Join(root, "deploy", "Dockerfile"))

	for _, required := range []string{
		"ARG TARGETOS\n",
		"ARG TARGETARCH\n",
		"GOOS=${TARGETOS} GOARCH=${TARGETARCH}",
		"GOOS=linux GOARCH=amd64",
		"-o /out/downloads/tinyprobe-agent-linux-amd64 ./cmd/agent",
		"GOOS=linux GOARCH=arm64",
		"-o /out/downloads/tinyprobe-agent-linux-arm64 ./cmd/agent",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Errorf("Dockerfile missing target architecture contract %q", required)
		}
	}

	for _, forbidden := range []string{
		"ARG TARGETOS=",
		"ARG TARGETARCH=",
	} {
		if strings.Contains(dockerfile, forbidden) {
			t.Errorf("Dockerfile must not default BuildKit target argument %q", forbidden)
		}
	}
}

func TestGitHubActionsPublishesMultiArchitectureGHCRImage(t *testing.T) {
	root := filepath.Join("..", "..")
	workflow := readDeploymentFile(t, filepath.Join(root, ".github", "workflows", "publish-image.yml"))

	for _, required := range []string{
		"branches: [main]",
		"contents: read",
		"packages: write",
		"docker/setup-qemu-action@v3",
		"docker/setup-buildx-action@v3",
		"docker/login-action@v3",
		"registry: ghcr.io",
		"images: ghcr.io/zxxx98/77probe",
		"type=raw,value=latest",
		"type=sha,prefix=sha-,format=short",
		"docker/build-push-action@v6",
		"file: deploy/Dockerfile",
		"platforms: linux/amd64,linux/arm64",
		"push: true",
		"cache-from: type=gha",
		"cache-to: type=gha,mode=max",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("publish workflow missing %q", required)
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
