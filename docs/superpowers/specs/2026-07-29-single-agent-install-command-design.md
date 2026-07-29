# 单一 Agent 安装命令设计

## 目标

将服务器创建或 Token 轮换后显示的 Agent 安装说明精简为一个可复制、可执行的安装命令区块。用户选择 `amd64` 或 `arm64` 后，只需复制并执行该区块。

## 交互与安全

安装面板继续显示一次性 Agent Token 和架构选择。说明文案改为“选择架构后，复制并执行下面的一条安装命令”。原先独立的下载命令、环境配置、安装命令和 systemd 配置区块全部移除。

新的命令由多行 shell 脚本组成，但作为一个复制区块执行。它使用 `curl` 下载匹配架构的 Agent 到临时目录，交互式读取 Token，使用 `sudo install` 写入 `/usr/local/bin/tinyprobe-agent` 与 `/etc/tinyprobe-agent.env`，写入 systemd unit，重新加载 systemd 并启用服务。成功后删除临时下载文件。

Token 只由 `read -rsp` 在目标终端读取，且只通过标准输入写入 root-only 的环境文件；它不会嵌入网页复制内容、shell 参数、URL、终端历史或进程参数。Token 继续以现有 UI 的一次性明文形式显示，供用户在命令提示时输入。

## 幂等与失败行为

命令可在同一服务器重复执行：二进制、环境文件和 systemd unit 会被覆盖，`systemctl enable --now` 会确保服务正在运行。`curl --fail --location --silent --show-error` 失败时，shell 使用 `set -eu` 立即退出，不会继续安装不完整文件。临时文件通过 `mktemp` 创建，并由 `trap` 在成功或失败时删除。

## 测试与范围

组件测试将断言：默认与切换架构后的单一区块包含正确下载地址和架构名；命令包含交互式 Token 输入、临时文件清理、环境文件、systemd unit 及服务启动；复制写入的内容与可见命令完全一致；原先四个区块不再出现。现有键盘架构切换、复制成功/失败通知和“我已保存 Token”行为保持。

不新增 API、服务端安装脚本、远程执行、Token 持久化或 Agent 协议变更。
