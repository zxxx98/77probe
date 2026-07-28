# TinyProbe

TinyProbe 是面向个人自托管场景的轻量 Linux 服务器探针。一个服务端管理 1–10 台服务器，每台 Agent 只读取系统指标并主动向服务端上报，不提供远程命令执行能力。

## 已实现功能

- 单管理员首次设置与登录。
- 每台服务器独立 Agent Token，服务端仅保存摘要。
- CPU、负载、内存、Swap、磁盘、磁盘 I/O 和默认路由网卡流量采集。
- 5 秒上报、30 秒离线判断和 SSE 实时页面更新。
- 服务器创建、重命名、启停、删除和 Token 轮换。
- Linux `amd64`、`arm64` Agent 下载与一次性安装说明。
- 历史指标按分钟聚合平均值、最大值和末值，提供 1 天、7 天、30 天趋势图，并显式保留缺失分钟的间断。
- 历史记录固定自动保留 30 天；服务端不持久化原始 5 秒上报。
- SQLite WAL 持久化和响应式 Web 管理界面。
- 持久化告警规则、触发/恢复事件和通用 Webhook 通知；Webhook 失败最多重试三次且不阻塞 Agent 上报。

当前没有数据维护或保留期设置界面，30 天保留策略不能通过 UI 调整。

## 告警与 Webhook

“告警”页面可为单台服务器配置离线、CPU 使用率、内存使用率、磁盘使用率和磁盘可用空间规则。资源规则默认持续 5 分钟；离线规则使用既有的 30 秒未上报判定。每次告警周期最多发送一次触发通知和一次恢复通知；重复提醒默认关闭。

Webhook 使用 JSON `POST`，只接受 `http` 或 `https` 地址。可配置请求头和正文模板，正文必须渲染为合法 JSON。模板可使用 `.ServerName`、`.Metric`、`.Status`、`.CurrentValue`、`.Threshold`、`.StartedAt`、`.EndedAt` 和 `.DetailURL`；需要 JSON 转义字符串时使用 `{{json .ServerName}}`。名称含 `authorization`、`token`、`secret` 或 `key` 的请求头会在重新加载时掩码显示。

Webhook 首次发送后最多再重试两次，延迟为 5 秒和 15 秒。投递失败只记录在告警事件中，Agent 上报仍会正常返回 `204`。

## Docker Compose 启动

```bash
docker compose up -d --build
```

服务启动后访问 `http://localhost:8080`，首次打开时创建唯一管理员。数据库保存在 Compose 命名卷 `tinyprobe-data` 中。

进入“服务器”页面创建服务器后，页面会一次性显示 Agent Token，并按 `amd64` 或 `arm64` 生成安装命令。确认保存后，页面不会再次提供原始 Token；如遗失请轮换 Token。

常用命令：

```bash
docker compose ps
docker compose logs -f tinyprobe
docker compose restart tinyprobe
```

## 服务端配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `TINYPROBE_ADDR` | `:8080` | HTTP 监听地址 |
| `TINYPROBE_DB_PATH` | `tinyprobe.db` | SQLite 数据库路径 |
| `TINYPROBE_SECURE_COOKIES` | `false` | 是否为会话 Cookie 添加 `Secure` 属性 |

容器内服务端从工作目录的 `downloads/` 读取两个 Agent 文件：

- `tinyprobe-agent-linux-amd64`
- `tinyprobe-agent-linux-arm64`

## 本地开发

需要 Go 1.24+、Node.js 和 pnpm。

```bash
pnpm --dir web install --frozen-lockfile
pnpm --dir web test -- --run
pnpm --dir web build
go test ./...
go run ./cmd/server
```

前端生产构建会写入 `internal/webui/dist`，随后由 Go 服务嵌入并提供。

## 负载生成器

`cmd/loadgen` 仅调用公开 Agent 上报协议。先在管理页面创建所需服务器，把每个一次性 Token 各放一行写入本地文件，然后运行：

```bash
go run ./cmd/loadgen \
  -base-url http://127.0.0.1:8080 \
  -token-file ./tokens.txt \
  -agents 10 \
  -duration 1m
```

生成器默认每 5 秒为每个 Token 上报一份确定性的合法指标；任一请求返回非 2xx 时会立即以非零状态退出。Token 文件不应提交到版本库。

仅在本地加速验证时，可以显式启用最低 100 毫秒的上报间隔：

```bash
go run ./cmd/loadgen \
  -base-url http://127.0.0.1:8080 \
  -token-file ./tokens.txt \
  -agents 10 \
  -duration 1m \
  -interval 100ms \
  -allow-fast=true
```

加速模式只用于验证，不会改变生产 Agent 固定的 5 秒上报节奏。未启用 `-allow-fast` 时，低于 5 秒的间隔会被拒绝；启用后仍不能低于 100 毫秒。

验证告警时可固定指标，或暂停上报以触发离线规则：

```bash
go run ./cmd/loadgen \
  -base-url http://127.0.0.1:8080 \
  -token-file ./tokens.txt \
  -agents 10 \
  -duration 3m \
  -cpu-percent 95 \
  -disk-used-percent 92 \
  -stop-after 1m \
  -resume-after 2m
```

## 验证

```bash
go test ./...
go vet ./...
pnpm --dir web test -- --run
pnpm --dir web lint
pnpm --dir web build
```

当前版本不包含远程命令、进程监控或数据维护/保留期设置界面。
