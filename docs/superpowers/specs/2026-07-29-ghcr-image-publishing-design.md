# GHCR 镜像自动发布设计

## 目标

当代码推送到 `main` 时，GitHub Actions 自动构建 TinyProbe 容器镜像并发布到 GitHub Container Registry（GHCR）。

## 范围

- 镜像名称固定为 `ghcr.io/zxxx98/77probe`。
- 仅 `main` 分支的 `push` 触发发布。
- 发布 `latest` 和对应提交的短 SHA 标签。
- 构建并发布 `linux/amd64` 与 `linux/arm64` 多架构 manifest。
- 使用现有 `deploy/Dockerfile` 作为唯一构建定义。

不在本次范围内：PR 镜像、语义版本标签、镜像签名、发布到 Docker Hub、部署动作或运行时密钥配置。

## 工作流设计

新增一个 GitHub Actions 工作流。工作流使用最小权限：`contents: read` 用于检出源码，`packages: write` 用于向同仓库所属的 GHCR 包写入镜像。

流程依次为：检出代码、启用 QEMU 以交叉构建 ARM 镜像、配置 Buildx、以仓库自带的 `GITHUB_TOKEN` 登录 `ghcr.io`、用 metadata action 计算 `latest` 和短 SHA 标签、用 Buildx 构建并推送多架构镜像。

构建上下文为仓库根目录，Dockerfile 路径为 `deploy/Dockerfile`。现有 Dockerfile 已接收 BuildKit 的目标操作系统与架构参数，并会在最终镜像中携带两个 Linux Agent 下载二进制，因此无需更改应用或容器构建逻辑。

## 标签与可追溯性

每次 `main` 推送生成：

- `ghcr.io/zxxx98/77probe:latest`
- `ghcr.io/zxxx98/77probe:sha-<短提交 SHA>`

两个标签指向同一个多架构 manifest。短 SHA 标签可用于部署回滚和定位镜像来源；`latest` 供 Compose 使用者直接更新。

## 失败行为与验证

任一构建、测试或 GHCR 推送步骤失败时，工作流以失败状态结束，且不会将未完成的镜像 manifest 标为 `latest`。工作流文件将接受 YAML 语法校验；本地继续运行既有 Go、前端测试、lint 和前端构建，确保发布配置未影响产品构建。

在 GitHub 上，向 `main` 推送一次提交后，应能在 Actions 日志中看到两个平台的构建与推送，并能通过 `docker buildx imagetools inspect ghcr.io/zxxx98/77probe:latest` 看到 `linux/amd64` 和 `linux/arm64`。
