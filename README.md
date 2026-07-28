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

当前没有数据维护或保留期设置界面，30 天保留策略不能通过 UI 调整。

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

## 验证

```bash
go test ./...
go vet ./...
pnpm --dir web test -- --run
pnpm --dir web lint
pnpm --dir web build
```

当前版本不包含远程命令、进程监控或数据维护/保留期设置界面。
