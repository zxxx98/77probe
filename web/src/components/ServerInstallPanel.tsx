import { useEffect, useRef, useState, type KeyboardEvent } from "react";

type Architecture = "amd64" | "arm64";

interface ServerInstallPanelProps {
  serverName: string;
  token: string;
  onTokenCleared: () => void;
}

interface CopyBlockProps {
  label: string;
  value: string;
  testId: string;
  onCopy: (label: string, value: string) => void;
}

function CopyBlock({ label, value, testId, onCopy }: CopyBlockProps) {
  return (
    <div className="install-copy-block">
      <div className="install-copy-heading">
        <h3>{label}</h3>
        <button
          className="button button-quiet"
          type="button"
          onClick={() => onCopy(label, value)}
        >
          复制{label}
        </button>
      </div>
      <pre data-testid={testId} tabIndex={0}>
        <code>{value}</code>
      </pre>
    </div>
  );
}

export function ServerInstallPanel({
  serverName,
  token,
  onTokenCleared,
}: ServerInstallPanelProps) {
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [architecture, setArchitecture] = useState<Architecture>("amd64");
  const [copyFeedback, setCopyFeedback] = useState<{
    message: string;
    failed: boolean;
  } | null>(null);
  const [tokenAnnouncement, setTokenAnnouncement] = useState("");
  const origin = window.location.origin;
  const filename = `tinyprobe-agent-linux-${architecture}`;
  const downloadCommand = `curl --fail --location --output ${filename} ${origin}/downloads/${filename}`;
  const environmentFile = `TINYPROBE_SERVER_URL=${origin}/api/agent/v1/report\nTINYPROBE_AGENT_TOKEN=${token}`;
  const installCommands = `sudo install -m 0755 ${filename} /usr/local/bin/tinyprobe-agent
read -rsp 'Agent Token: ' TINYPROBE_AGENT_TOKEN
printf '\\n'
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
  const systemdUnit = `[Unit]
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
WantedBy=multi-user.target`;

  useEffect(() => {
    headingRef.current?.focus();
  }, [serverName, token]);

  useEffect(() => {
    setTokenAnnouncement("");
    const timeout = window.setTimeout(() => {
      setTokenAnnouncement("一次性 Agent Token 已准备好，请立即保存。");
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [token]);

  const copy = async (label: string, value: string) => {
    try {
      if (!navigator.clipboard) {
        throw new Error("clipboard unavailable");
      }
      await navigator.clipboard.writeText(value);
      setCopyFeedback({ message: `${label}已复制`, failed: false });
    } catch {
      setCopyFeedback({ message: "复制失败，请手动选择内容。", failed: true });
    }
  };

  const moveArchitectureFocus = (
    event: KeyboardEvent<HTMLButtonElement>,
  ) => {
    let next: Architecture | null = null;
    if (event.key === "ArrowRight") {
      next = architecture === "amd64" ? "arm64" : "amd64";
    } else if (event.key === "ArrowLeft") {
      next = architecture === "amd64" ? "arm64" : "amd64";
    } else if (event.key === "End") {
      next = "arm64";
    } else if (event.key === "Home") {
      next = "amd64";
    }
    if (!next) {
      return;
    }
    event.preventDefault();
    setArchitecture(next);
    document.getElementById(`architecture-${next}`)?.focus();
  };

  return (
    <section className="install-panel" aria-labelledby="install-panel-title">
      <div className="install-panel-heading">
        <div>
          <p className="calm-status">
            <span className="status-dot" aria-hidden="true" />
            Token 只在这里完整显示一次
          </p>
          <h2 id="install-panel-title" ref={headingRef} tabIndex={-1}>
            安装 {serverName} 的 Agent
          </h2>
          <p>先保存 Token，再选择服务器架构并依次执行下面的命令。</p>
        </div>
        <button
          className="button button-primary install-token-saved"
          type="button"
          onClick={onTokenCleared}
        >
          我已保存 Token
        </button>
      </div>

      <p
        className="sr-only"
        aria-live="polite"
        data-testid="token-ready-announcement"
      >
        {tokenAnnouncement}
      </p>

      <div className="one-time-token" aria-label="一次性 Agent Token">
        <span>Agent Token</span>
        <code>{token}</code>
      </div>

      <div className="architecture-tabs" role="tablist" aria-label="Agent 架构">
        {(["amd64", "arm64"] as const).map((value) => (
          <button
            id={`architecture-${value}`}
            key={value}
            role="tab"
            type="button"
            aria-controls="agent-install-instructions"
            aria-selected={architecture === value}
            tabIndex={architecture === value ? 0 : -1}
            onClick={() => setArchitecture(value)}
            onKeyDown={moveArchitectureFocus}
          >
            {value}
          </button>
        ))}
      </div>

      <div
        id="agent-install-instructions"
        className="install-steps"
        role="tabpanel"
        aria-labelledby={`architecture-${architecture}`}
      >
        <CopyBlock
          label="下载命令"
          value={downloadCommand}
          testId="download-command"
          onCopy={copy}
        />
        <CopyBlock
          label="环境配置"
          value={environmentFile}
          testId="environment-file"
          onCopy={copy}
        />
        <CopyBlock
          label="安装命令"
          value={installCommands}
          testId="install-commands"
          onCopy={copy}
        />
        <CopyBlock
          label="systemd 配置"
          value={systemdUnit}
          testId="systemd-unit"
          onCopy={copy}
        />
      </div>

      {copyFeedback ? (
        <p
          className={copyFeedback.failed ? "copy-feedback copy-feedback--error" : "copy-feedback"}
          role={copyFeedback.failed ? "alert" : "status"}
        >
          {copyFeedback.message}
        </p>
      ) : null}
    </section>
  );
}
