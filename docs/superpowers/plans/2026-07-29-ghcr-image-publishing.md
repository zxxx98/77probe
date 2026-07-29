# GHCR Image Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在每次推送 `main` 时构建 `linux/amd64` 与 `linux/arm64` TinyProbe 镜像，并发布到 GHCR 的 `latest` 和短 SHA 标签。

**Architecture:** 一个仅由 `main` push 触发的 GitHub Actions 工作流复用 `deploy/Dockerfile`。工作流以 `GITHUB_TOKEN` 登录 GHCR，先用 metadata action 生成标签，再通过 Buildx 推送一个多架构 manifest；GitHub Actions 缓存存储 Docker 层。

**Tech Stack:** GitHub Actions、Docker Buildx、QEMU、GHCR、Go 测试。

---

### Task 1: 用部署契约测试固定发布工作流

**Files:**
- Modify: `internal/app/deployment_contract_test.go`
- Create: `.github/workflows/publish-image.yml`

- [ ] **Step 1: 写入失败的 GitHub Actions 契约测试**

在 `internal/app/deployment_contract_test.go` 的末尾、`readDeploymentFile` 前新增：

```go
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
```

- [ ] **Step 2: 运行新测试，确认其因工作流尚不存在而失败**

Run: `go test ./internal/app -run TestGitHubActionsPublishesMultiArchitectureGHCRImage -count=1`

Expected: FAIL，错误指出 `.github/workflows/publish-image.yml` 不存在。

- [ ] **Step 3: 新增最小发布工作流**

创建 `.github/workflows/publish-image.yml`：

```yaml
name: Publish image

on:
  push:
    branches: [main]

permissions:
  contents: read
  packages: write

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - name: Check out source
        uses: actions/checkout@v5

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Generate image metadata
        id: metadata
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/zxxx98/77probe
          tags: |
            type=raw,value=latest
            type=sha,prefix=sha-,format=short

      - name: Build and publish image
        uses: docker/build-push-action@v6
        with:
          context: .
          file: deploy/Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.metadata.outputs.tags }}
          labels: ${{ steps.metadata.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 4: 运行契约测试，确认工作流满足发布要求**

Run: `go test ./internal/app -run TestGitHubActionsPublishesMultiArchitectureGHCRImage -count=1`

Expected: PASS。

- [ ] **Step 5: 运行相关部署与完整本地验证**

Run:

```bash
go test ./internal/app -count=1
go test ./...
go vet ./...
pnpm --dir web test -- --run
pnpm --dir web lint
pnpm --dir web build
git diff --check
```

Expected: 每条命令以退出码 0 完成。本地不能模拟 `GITHUB_TOKEN` 的 GHCR 写入；首次推送后需在 GitHub Actions 中确认工作流成功，并用 `docker buildx imagetools inspect ghcr.io/zxxx98/77probe:latest` 查看两个平台。

- [ ] **Step 6: 提交发布工作流**

```bash
git add .github/workflows/publish-image.yml internal/app/deployment_contract_test.go
git commit -m "ci: publish multi-architecture image to GHCR"
```

## 计划自检

- 设计中的 `main` 唯一触发器、最小权限、固定 GHCR 名称、两个标签和两个平台均由 Task 1 的工作流与契约测试覆盖。
- 该计划不引入 PR 发布、语义版本、Docker Hub、镜像签名或部署动作。
- Dockerfile、应用代码与 Compose 配置保持不变，避免改变当前本地部署行为。
