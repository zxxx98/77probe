# Single Agent Install Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Agent 安装面板改为按架构生成一个可复制、交互式读取 Token 的安装命令区块。

**Architecture:** `ServerInstallPanel` 在浏览器中根据当前页面 origin 和选中的架构生成一段多行 shell 脚本。脚本以 `curl` 下载临时文件、交互读取 Token、安装二进制和配置、写入 systemd unit 并启动服务；Token 不嵌入脚本。组件继续使用既有剪贴板反馈、架构标签和一次性 Token 生命周期。

**Tech Stack:** React 19、TypeScript、Vitest、Testing Library、现有管理页 CSS。

---

### Task 1: 以组件测试定义单一、安全的安装命令

**Files:**
- Modify: `web/src/components/ServerInstallPanel.test.tsx`
- Modify: `web/src/components/ServerInstallPanel.tsx`

- [ ] **Step 1: 写入失败的单一命令测试**

替换 `builds one-time install content from the current origin` 测试中的四块命令断言，使其断言单一命令区块：

```tsx
const installCommand = screen.getByTestId("install-command");
expect(installCommand.textContent).toContain(
  `curl --fail --location --silent --show-error --output "$TMP_BIN" ${origin}/downloads/tinyprobe-agent-linux-amd64`,
);
expect(installCommand.textContent).toContain("set -eu");
expect(installCommand.textContent).toContain('TMP_BIN="$(mktemp)"');
expect(installCommand.textContent).toContain("trap 'rm -f \"$TMP_BIN\"' EXIT");
expect(installCommand.textContent).toContain(
  "read -rsp 'Agent Token: ' TINYPROBE_AGENT_TOKEN",
);
expect(installCommand.textContent).toContain(
  "sudo install -m 0755 \"$TMP_BIN\" /usr/local/bin/tinyprobe-agent",
);
expect(installCommand.textContent).toContain(
  "sudo install -m 0600 /dev/null /etc/tinyprobe-agent.env",
);
expect(installCommand.textContent).toContain("sudo systemctl enable --now tinyprobe-agent");
expect(installCommand.textContent).not.toContain("tp_one_time");
expect(screen.queryByTestId("download-command")).not.toBeInTheDocument();
expect(screen.queryByTestId("environment-file")).not.toBeInTheDocument();
expect(screen.queryByTestId("install-commands")).not.toBeInTheDocument();
expect(screen.queryByTestId("systemd-unit")).not.toBeInTheDocument();
```

将架构切换测试改为检查 `install-command` 包含 `tinyprobe-agent-linux-arm64`，而不包含 amd64。将成功复制测试的按钮文本改为 `复制安装命令`，并断言 `writeText` 接收到 `install-command.textContent`；将失败复制测试的按钮文本同样改为 `复制安装命令`。

- [ ] **Step 2: 运行组件测试，确认其因旧四段式 UI 而失败**

Run: `pnpm --dir web test -- --run src/components/ServerInstallPanel.test.tsx`

Expected: FAIL，缺少 `install-command`，且旧复制按钮文本不匹配。

- [ ] **Step 3: 生成一个安装脚本并移除四个旧区块**

在 `ServerInstallPanel.tsx` 中删除 `CopyBlock`、`filename`、`downloadCommand`、`environmentFile`、`installCommands` 和 `systemdUnit`。以选中的 `architecture` 和当前 `origin` 生成：

```ts
const filename = `tinyprobe-agent-linux-${architecture}`;
const installCommand = `set -eu
TMP_BIN="$(mktemp)"
trap 'rm -f "$TMP_BIN"' EXIT
curl --fail --location --silent --show-error --output "$TMP_BIN" ${origin}/downloads/${filename}
read -rsp 'Agent Token: ' TINYPROBE_AGENT_TOKEN
printf '\\n'
sudo install -m 0755 "$TMP_BIN" /usr/local/bin/tinyprobe-agent
sudo install -m 0600 /dev/null /etc/tinyprobe-agent.env
printf '%s\\n' \\
  'TINYPROBE_SERVER_URL=${origin}/api/agent/v1/report' \\
  "TINYPROBE_AGENT_TOKEN=$TINYPROBE_AGENT_TOKEN" |
  sudo tee /etc/tinyprobe-agent.env >/dev/null
unset TINYPROBE_AGENT_TOKEN
sudo tee /etc/systemd/system/tinyprobe-agent.service > /dev/null <<'EOF'
[Unit]
Description=TinyProbe Agent
After=network-online.target
Wants=network-online.target

[Service]
DynamicUser=true
EnvironmentFile=/etc/tinyprobe-agent.env
ExecStart=/usr/local/bin/tinyprobe-agent
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ReadOnlyPaths=/proc /sys

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now tinyprobe-agent`;
```

更新说明为“选择服务器架构后，复制并执行下面的一条安装命令；执行时会提示输入 Agent Token。”在 `agent-install-instructions` 中渲染一个 `install-copy-block`，标题为“安装命令”，复制按钮调用 `copy("安装命令", installCommand)`，代码块使用 `<pre data-testid="install-command" tabIndex={0}>`。

- [ ] **Step 4: 运行组件测试，确认单一命令与既有交互通过**

Run: `pnpm --dir web test -- --run src/components/ServerInstallPanel.test.tsx`

Expected: PASS，包含安全的单一命令、架构切换、复制状态、无障碍提示和 Token 清除行为。

- [ ] **Step 5: 运行完整前端验证**

Run:

```bash
pnpm --dir web test -- --run
pnpm --dir web lint
pnpm --dir web build
git diff --check
```

Expected: 所有前端测试、TypeScript 检查和生产构建成功。构建更新 `internal/webui/dist`。

- [ ] **Step 6: 提交前端与构建产物**

```bash
git add web/src/components/ServerInstallPanel.tsx web/src/components/ServerInstallPanel.test.tsx internal/webui/dist
git commit -m "feat: consolidate agent installation command"
```

## 计划自检

- 单一命令、架构相关下载、交互式 Token、临时文件清理、幂等覆盖安装和 systemd 启动均由 Task 1 实现与测试覆盖。
- 测试明确保证 Token 不会出现在安装脚本，且旧四块 UI 不再渲染。
- 不改变 API、服务端、Agent 协议、Token 生命周期或既有无障碍交互。
