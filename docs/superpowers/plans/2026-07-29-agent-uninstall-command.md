# Agent 卸载命令实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在服务器管理页为每台已登记服务器提供可复制、幂等的 TinyProbe Agent 卸载命令。

**Architecture:** 扩展既有 `ServersPage` 的单行操作状态，在服务器行内展开卸载说明和命令代码块。命令由纯前端常量生成，只清理安装流程创建的 systemd unit、环境文件和二进制；浏览器只写入剪贴板，不调用新 API，也不远程执行。

**Tech Stack:** React 19、TypeScript、Vitest、Testing Library、现有 CSS 管理页样式。

---

### Task 1: 添加可测试的卸载命令与服务器行交互

**Files:**
- Modify: `web/src/pages/ServersPage.tsx`
- Modify: `web/src/pages/ServersPage.test.tsx`
- Modify: `web/src/styles/dashboard.css`

- [ ] **Step 1: 写入失败的 UI 测试**

在 `ServersPage.test.tsx` 增加测试：点击 `卸载 home-lab 的 Agent` 后，页面显示以下完整命令；点击 `复制卸载命令` 时，`navigator.clipboard.writeText` 接收同一字符串；按钮的 `aria-expanded` 从 `false` 变为 `true`。

```ts
const uninstallCommand = `sudo systemctl disable --now tinyprobe-agent.service 2>/dev/null || true
sudo rm -f /etc/systemd/system/tinyprobe-agent.service
sudo rm -f /etc/tinyprobe-agent.env
sudo rm -f /usr/local/bin/tinyprobe-agent
sudo systemctl daemon-reload
sudo systemctl reset-failed tinyprobe-agent.service 2>/dev/null || true`;

fireEvent.click(screen.getByRole("button", { name: "卸载 home-lab 的 Agent" }));
expect(screen.getByTestId("uninstall-command-1")).toHaveTextContent(uninstallCommand);
expect(screen.getByRole("button", { name: "卸载 home-lab 的 Agent" })).toHaveAttribute("aria-expanded", "true");
fireEvent.click(screen.getByRole("button", { name: "复制卸载命令" }));
expect(writeText).toHaveBeenCalledWith(uninstallCommand);
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --dir web test -- --run src/pages/ServersPage.test.tsx`

Expected: FAIL，因为卸载操作按钮、展开区域和复制命令尚不存在。

- [ ] **Step 3: 实现最小交互**

在 `ServersPage` 中新增 `uninstallServerID: number | null` 状态和以下常量：

```ts
const uninstallCommand = `sudo systemctl disable --now tinyprobe-agent.service 2>/dev/null || true
sudo rm -f /etc/systemd/system/tinyprobe-agent.service
sudo rm -f /etc/tinyprobe-agent.env
sudo rm -f /usr/local/bin/tinyprobe-agent
sudo systemctl daemon-reload
sudo systemctl reset-failed tinyprobe-agent.service 2>/dev/null || true`;
```

每个未编辑的服务器行增加“卸载 Agent”按钮；它只切换 `uninstallServerID`，且使用 `aria-expanded` 与 `aria-controls`。当对应行展开时，显示说明、`<pre data-testid={\`uninstall-command-${server.id}\`}>` 和复制按钮。复制逻辑复用 `navigator.clipboard.writeText`，成功显示“卸载命令已复制”，失败显示现有的复制失败反馈。不能调用服务器 API，也不能改变删除确认内容。

- [ ] **Step 4: 添加响应式样式**

为 `.agent-uninstall-panel` 添加与 `.install-panel` 一致的浅色容器、可滚动代码块和移动端 `min-width: 0` 约束；在 `@media (max-width: 46rem)` 中使其随管理行自然堆叠，避免页面横向溢出。

- [ ] **Step 5: 运行聚焦测试确认通过**

Run: `pnpm --dir web test -- --run src/pages/ServersPage.test.tsx`

Expected: PASS，且现有服务器创建、Token、删除和重命名测试仍全部通过。

- [ ] **Step 6: 运行完整前端验证并提交**

Run:

```bash
pnpm --dir web test -- --run
pnpm --dir web lint
pnpm --dir web build
```

Expected: 所有测试通过，TypeScript 无错误，生产构建成功。

```bash
git add web/src/pages/ServersPage.tsx web/src/pages/ServersPage.test.tsx web/src/styles/dashboard.css internal/webui/dist
git commit -m "feat: add agent uninstall command"
```

## 计划自检

- 命令只卸载 TinyProbe 自身安装的文件，且在服务不存在时仍可执行。
- 每台服务器均可展开和复制，命令不包含 Token 或远程执行逻辑。
- 服务器记录删除仍独立，现有删除文案与 API 均不改变。
- 测试覆盖命令内容、展开状态、复制调用、以及完整前端回归与构建。
