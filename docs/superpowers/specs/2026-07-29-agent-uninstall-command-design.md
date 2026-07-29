# Agent 卸载命令设计

## 目标

让管理员能在“服务器”管理页为任意已登记的服务器复制一条 Linux 卸载命令，并在目标服务器上执行，以移除 TinyProbe Agent。

## 范围与边界

- 每个服务器行新增“卸载 Agent”操作；点击后在该行内显示命令和复制按钮。
- 命令只在浏览器复制，TinyProbe 服务端不会连接、登录或执行目标服务器上的任何命令。
- 卸载命令与服务器 Token、服务器名称和架构无关，因此不会暴露敏感数据。
- 现有“删除”操作继续只删除 TinyProbe 中的服务器记录、历史、告警及 Token；它不会运行卸载命令。
- 不提供 SSH、远程执行、自动卸载、进程清理或其他超出 TinyProbe 安装范围的删除行为。

## 卸载命令

页面提供以下幂等 systemd 命令：

```bash
sudo systemctl disable --now tinyprobe-agent.service 2>/dev/null || true
sudo rm -f /etc/systemd/system/tinyprobe-agent.service
sudo rm -f /etc/tinyprobe-agent.env
sudo rm -f /usr/local/bin/tinyprobe-agent
sudo systemctl daemon-reload
sudo systemctl reset-failed tinyprobe-agent.service 2>/dev/null || true
```

它只清理安装流程创建的 unit、环境文件和二进制。若 Agent 已经不存在，命令仍成功完成。

## 交互与可访问性

“卸载 Agent”使用普通操作按钮，位于重命名、启停和 Token 操作旁。展开区域说明：先在目标服务器执行此命令；随后如不再监控该机器，再使用 TinyProbe 的“删除”操作移除本地记录。命令以现有安装面板相同的代码块和“复制”按钮呈现，复制成功或失败通过现有状态反馈通知。

移动端中该区域随服务器行堆叠，不引入横向溢出。展开与收起有明确的按钮文本和 `aria-expanded` 状态。

## 验证

新增前端测试覆盖：展开/收起、精确命令内容、复制到剪贴板、复制失败反馈，以及“删除服务器”仍保持独立语义。完整前端测试、lint 和生产构建与 Go 回归测试在实现结束后运行。
