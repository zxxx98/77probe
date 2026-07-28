# Phase 3 告警与 Webhook 实施计划

> **执行方式：** 当前 `main` 分支单线程内联执行；不调度子代理。用户要求所有测试命令在整期完成后统一运行，因此各任务只编写测试，不在中途执行测试命令。

**目标：** 为实时监控加入可管理的告警规则、事件历史和可靠的通用 Webhook 通知，并把管理界面集中在新的告警页。

**架构：** `internal/alerting` 将持久化、纯状态机、异步评估和异步投递分离。实时协调器通过多播发布 `live.Event`；告警评估器订阅该事件流并把工作投入有界队列。HTTP API 和 React 告警页仅调用 Repository 与 Dispatcher 的公开能力，不参与 Agent 上报同步路径。

**技术栈：** Go 1.24、SQLite WAL、`text/template`、React 19、TypeScript、Vitest、Testing Library。

---

## 文件边界

- `migrations/003_alerting.sql`：告警、Webhook 与投递记录表。
- `internal/alerting/model.go`、`repository.go`：严格枚举、数据类型与 SQLite Repository。
- `internal/alerting/state_machine.go`：无 I/O 的状态转换。
- `internal/alerting/evaluator.go`、`metrics.go`：有界异步实时事件评估。
- `internal/alerting/template.go`、`webhook.go`、`dispatcher.go`：模板、HTTP 投递与重试。
- `internal/alerting/handler.go`：受认证保护的管理 API。
- `internal/live/hub.go`、`coordinator.go`：支持除 SSE 外的内部订阅者。
- `internal/app/app.go`、`internal/httpapi/router.go`：装配后台组件与路由。
- `web/src/alerts/*`、`web/src/components/*`、`web/src/pages/AlertsPage.tsx`：告警页数据层与三个页面区块。
- `web/src/app/App.tsx`、`web/src/components/AppNav.tsx`、`web/src/styles/dashboard.css`：路由、导航和现有风格的扩展。

### Task 1：持久化告警模型与 Repository

**文件：** 新建 `migrations/003_alerting.sql`、`internal/alerting/model.go`、`internal/alerting/repository.go`、`internal/alerting/repository_test.go`。

- [ ] 创建 `webhook_configs`（`id=1` 单例）、`alert_rules`、`alert_states`、`alert_events`、`webhook_attempts`；所有关联以 `ON DELETE CASCADE` 清理。
- [ ] 定义 `Metric`、`Operator`、`Status` 和 `Rule`、`State`、`Event`、`WebhookConfig`、`WebhookAttempt`。固定枚举值为 `offline`、`cpu_usage`、`memory_usage`、`disk_usage`、`disk_free_bytes`；状态为 `normal`、`pending`、`firing`、`recovered`。
- [ ] 实现 `Repository`：`CreateRule`、`ListRules`、`UpdateRule`、`DeleteRule`、`GetState`、`SaveStateAndEvent`、`ListEvents`、`GetWebhook`、`UpsertWebhook`、`RecordAttempt` 与 `ListAttempts`。未知记录返回 `ErrNotFound`；入库前验证枚举。
- [ ] 编写 Repository 合同测试，覆盖规则 CRUD、每规则单一状态、最新优先事件、Webhook 单例、尝试记录与级联删除；暂不执行。
- [ ] 提交：`feat: add alerting persistence`。

### Task 2：实现纯告警状态机

**文件：** 新建 `internal/alerting/state_machine.go`、`internal/alerting/state_machine_test.go`。

- [ ] 定义无副作用输入输出：

```go
type EvaluationInput struct {
	State State; Breached bool; CurrentValue float64
	Duration, RepeatInterval time.Duration; Now time.Time
}
type EvaluationResult struct { State State; Notify bool }
func Evaluate(input EvaluationInput) EvaluationResult
```

- [ ] 实现规则：`normal` 越界进入 `pending`（零时长立即 `firing` 并通知）；`pending` 到期变 `firing`；健康的 `pending` 回到 `normal`；`firing` 健康变 `recovered` 并通知；健康 `recovered` 回到 `normal`；再次越界开始新周期；仅配置重复间隔时发送重复通知。
- [ ] 编写表驱动状态转换、零时长、边界时刻、重复通知、pending 重置与恢复后新周期测试；暂不执行。
- [ ] 提交：`feat: add alert state machine`。

### Task 3：异步 Webhook 模板、投递与重试

**文件：** 新建 `internal/alerting/template.go`、`webhook.go`、`dispatcher.go` 及对应三个测试文件。

- [ ] 用 `text/template` 的 `missingkey=error` 和 `json` 函数实现：

```go
func RenderTemplate(text string, data TemplateData) ([]byte, error)
```

渲染后用 `json.Valid` 拒绝不合法正文。`TemplateData` 含事件、服务器、指标、状态、当前值、阈值、起止时间及详情链接。
- [ ] 实现 `WebhookClient.Send`：仅接受绝对 `http`/`https` URL，以 10 秒超时发 JSON `POST`，保留自定义头，仅 2xx 成功，并把错误摘要截断至 2048 字节。
- [ ] 实现 `Dispatcher`：两个 worker、64 容量队列，尝试延迟为 `0s`、`5s`、`15s`。每一次结果先经 Repository 持久化，再决定重试；队列满只返回可记录错误，不影响上报。
- [ ] 编写 JSON 转义、未知模板变量、无效 JSON、请求头、2xx/非 2xx、超时、三次尝试和队列满测试；测试注入 sleeper，暂不执行。
- [ ] 提交：`feat: add webhook delivery`。

### Task 4：接入实时事件并在后台评估

**文件：** 修改 `internal/live/hub.go`、`coordinator.go`、`internal/app/app.go`；新建 `internal/alerting/metrics.go`、`evaluator.go`、`evaluator_test.go`。

- [ ] 给 `live.Hub` 增加独立内部订阅通道，使 SSE 与告警各自消费相同事件，不能让任一订阅者阻塞 `Coordinator.Accept` 或 `Coordinator.Sweep`。
- [ ] 实现 `MetricValue(snapshot, metric)`：内存总量为零返回零；磁盘使用率取最大百分比；磁盘可用空间取最小字节值；离线为 `1` 或 `0`。
- [ ] 实现 `Evaluator.Submit(live.Event)` 和后台 `Run(context.Context)`。使用容量 128 的输入队列；满时记录丢弃并依据当前 Store 快照补偿重评；单 worker 顺序加载已启用规则、调用 `Evaluate`、在同一持久化操作中保存状态和事件；只有 `Notify` 才投递 `DeliveryJob`。
- [ ] 将 Evaluator 和 Dispatcher 加入应用后台生命周期，确保关闭时等待 worker 停止；评估器订阅 Coordinator 的实时事件。
- [ ] 编写 CPU 持续时间、零总内存、磁盘极值、离线/恢复、禁用规则、队列饱和及慢评估下 Agent 返回 `204` 的测试；暂不执行。
- [ ] 提交：`feat: evaluate live alert rules`。

### Task 5：实现认证告警与 Webhook API

**文件：** 新建 `internal/alerting/handler.go`、`handler_test.go`；修改 `internal/httpapi/router.go`、`router_test.go`。

- [ ] 在认证路由组添加：

```text
GET/POST /api/alert-rules
PATCH/DELETE /api/alert-rules/{id}
GET /api/alert-events
GET/PUT /api/webhook
POST /api/webhook/test
```

- [ ] 严格校验：离线为 `gt 0` 且时长零；百分比指标只能 `gt`、阈值 0–100；可用空间只能 `lt`、阈值大于零；资源持续时间默认 300、范围 0–86400；重复时间为零或 300–604800。
- [ ] `GET/PUT /api/webhook` 掩码敏感头值；接收掩码时保留已保存的秘密。`POST /api/webhook/test` 构造 “Webhook 测试” 样本并返回最终投递结果与尝试数。
- [ ] 编写未认证、验证矩阵、分页、掩码保持、非法模板、测试成功与三次失败详情测试；暂不执行。
- [ ] 提交：`feat: add alert management api`。

### Task 6：构建与现有风格一致的告警页

**文件：** 新建 `web/src/alerts/types.ts`、`api.ts`、`web/src/components/AlertRuleForm.tsx`、`AlertEventList.tsx`、`WebhookForm.tsx`、`web/src/pages/AlertsPage.tsx` 及对应测试；修改 `web/src/app/App.tsx`、`web/src/components/AppNav.tsx`、`web/src/styles/dashboard.css`。

- [ ] 在前端 API 层为五类规则、事件、投递尝试和 Webhook 设置建模，调用 Task 5 的 API。
- [ ] 将 `/alerts` 加进 `DashboardRouter` 和主导航。页面沿用 `management-content`、标题、按钮、错误反馈和内联确认的既有视觉样式。
- [ ] 实现规则区：内联表单（服务器、指标、阈值、持续时长、重复间隔、启用状态），离线规则禁用时长；列表提供启用切换、编辑和删除确认。
- [ ] 实现事件区：新事件在前，语义状态、服务器、指标、数值、阈值与时间；行内展开三次尝试记录。
- [ ] 实现 Webhook 区：URL、开关、可增删头行、JSON 模板、变量参考和测试发送结果。敏感头重载后保持掩码；保存失败时保留输入值。
- [ ] 添加移动端样式：390px 时表单标签可见，规则与事件以堆叠行呈现，控件保持 44px 最小点击高度。
- [ ] 编写路由、离线规则、掩码请求头、测试反馈、事件展开及移动样式合同测试；暂不执行。
- [ ] 提交：`feat: add alert management page`。

### Task 7：整期统一验证、文档与验收

**文件：** 修改 `README.md`、必要时 `cmd/loadgen/main.go`；新建 `internal/alerting/integration_test.go`。

- [ ] 编写端到端测试：创建服务器、2 秒 CPU 规则和测试 Webhook，验证一次 firing、无重复 firing、一次 recovered 以及各自投递记录。
- [ ] 更新 README：支持指标、Webhook 模板变量、三次重试、秘密头掩码和“Webhook 失败不影响 Agent 上报”的边界。
- [ ] 仅在此任务运行全部验证：

```bash
go test ./...
go vet ./...
pnpm --dir web test -- --run
pnpm --dir web lint
pnpm --dir web build
docker compose up -d --build
curl --fail http://localhost:8080/api/health
```

- [ ] 用返回 500 的测试 Webhook 进行隔离验收：十个模拟 Agent 的上报均为 `204`，每个事件有三次尝试，恢复事件产生独立三次尝试。
- [ ] 提交：`test: verify webhook alert delivery`。

## 计划自检

- 设计中的持久化、状态机、异步隔离、Webhook、API、单页告警 UI、响应式表现和最终验收均有对应任务。
- 不包含配置页、邮件、即时通信、签名、多用户路由或远程操作。
- 每项测试均在实现时编写，但根据用户明确要求，只在 Task 7 运行测试命令。
