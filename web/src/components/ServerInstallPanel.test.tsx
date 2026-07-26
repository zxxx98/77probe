import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ServerInstallPanel } from "./ServerInstallPanel";

const writeText = vi.fn<(value: string) => Promise<void>>();

describe("ServerInstallPanel", () => {
  beforeEach(() => {
    writeText.mockReset();
    writeText.mockResolvedValue();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
  });

  it("builds one-time install content from the current origin", () => {
    render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_one_time"
        onTokenCleared={vi.fn()}
      />,
    );

    const origin = window.location.origin;
    expect(screen.getByText("tp_one_time")).toBeInTheDocument();
    expect(screen.getByTestId("download-command")).toHaveTextContent(
      `curl --fail --location --output tinyprobe-agent-linux-amd64 ${origin}/downloads/tinyprobe-agent-linux-amd64`,
    );
    expect(screen.getByTestId("environment-file")).toHaveTextContent(
      `TINYPROBE_SERVER_URL=${origin}/api/agent/v1/report`,
    );
    expect(screen.getByTestId("environment-file")).toHaveTextContent(
      "TINYPROBE_AGENT_TOKEN=tp_one_time",
    );
    expect(screen.getByTestId("install-commands")).toHaveTextContent(
      "sudo install -m 0755 tinyprobe-agent-linux-amd64 /usr/local/bin/tinyprobe-agent",
    );
    expect(screen.getByTestId("install-commands")).toHaveTextContent(
      "sudo install -m 0600 /dev/null /etc/tinyprobe-agent.env",
    );
    expect(screen.getByTestId("install-commands")).toHaveTextContent(
      "sudo systemctl enable --now tinyprobe-agent",
    );

    const unit = screen.getByTestId("systemd-unit");
    for (const directive of [
      "EnvironmentFile=/etc/tinyprobe-agent.env",
      "ExecStart=/usr/local/bin/tinyprobe-agent",
      "Restart=always",
      "RestartSec=5",
      "NoNewPrivileges=true",
      "ProtectSystem=strict",
      "ReadOnlyPaths=/proc /sys",
      "DynamicUser=true",
    ]) {
      expect(unit).toHaveTextContent(directive);
    }
    expect(document.body.textContent).not.toContain("example.com");
  });

  it("switches all architecture-specific commands to arm64", () => {
    render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_one_time"
        onTokenCleared={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("tab", { name: "arm64" }));

    expect(screen.getByRole("tab", { name: "arm64" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByTestId("download-command")).toHaveTextContent(
      "tinyprobe-agent-linux-arm64",
    );
    expect(screen.getByTestId("install-commands")).toHaveTextContent(
      "sudo install -m 0755 tinyprobe-agent-linux-arm64 /usr/local/bin/tinyprobe-agent",
    );
    expect(screen.getByTestId("download-command")).not.toHaveTextContent(
      "tinyprobe-agent-linux-amd64",
    );
  });

  it("supports arrow-key navigation between architecture tabs", () => {
    render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_one_time"
        onTokenCleared={vi.fn()}
      />,
    );
    const amd64 = screen.getByRole("tab", { name: "amd64" });

    amd64.focus();
    fireEvent.keyDown(amd64, { key: "ArrowRight" });

    expect(screen.getByRole("tab", { name: "arm64" })).toHaveFocus();
    expect(screen.getByRole("tab", { name: "arm64" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    fireEvent.keyDown(screen.getByRole("tab", { name: "arm64" }), {
      key: "ArrowLeft",
    });
    expect(screen.getByRole("tab", { name: "amd64" })).toHaveFocus();

    fireEvent.keyDown(screen.getByRole("tab", { name: "amd64" }), {
      key: "End",
    });
    expect(screen.getByRole("tab", { name: "arm64" })).toHaveFocus();
  });

  it("announces copy success accessibly", async () => {
    render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_one_time"
        onTokenCleared={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "复制下载命令" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("status")).toHaveTextContent("下载命令已复制");
  });

  it("announces copy failure accessibly", async () => {
    writeText.mockRejectedValueOnce(new Error("permission denied"));
    render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_one_time"
        onTokenCleared={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "复制环境配置" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "复制失败，请手动选择内容",
    );
  });

  it("lets the owner clear the one-time token", () => {
    const onTokenCleared = vi.fn();
    render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_one_time"
        onTokenCleared={onTokenCleared}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "我已保存 Token" }));

    expect(onTokenCleared).toHaveBeenCalledTimes(1);
  });
});
