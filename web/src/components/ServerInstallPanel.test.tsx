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

  it("builds one secure install command from the current origin", async () => {
    render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_one_time"
        onTokenCleared={vi.fn()}
      />,
    );

    const origin = window.location.origin;
    expect(screen.getByText("tp_one_time")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "安装 home-lab 的 Agent" }),
    ).toHaveFocus();
    expect(
      await screen.findByText("一次性 Agent Token 已准备好，请立即保存。"),
    ).toHaveAttribute("aria-live", "polite");
    const installCommand = screen.getByTestId("install-command");
    expect(installCommand.textContent).toContain("set -eu");
    expect(installCommand.textContent).toContain('TMP_BIN="$(mktemp)"');
    expect(installCommand.textContent).toContain(
      "trap 'rm -f \"$TMP_BIN\"' EXIT",
    );
    expect(installCommand.textContent).toContain(
      `curl --fail --location --silent --show-error --output "$TMP_BIN" ${origin}/downloads/tinyprobe-agent-linux-amd64`,
    );
    expect(installCommand.textContent).toContain(
      "read -rsp 'Agent Token: ' TINYPROBE_AGENT_TOKEN",
    );
    expect(installCommand.textContent).toContain("printf '\\n'");
    expect(installCommand.textContent).toContain("printf '%s\\n'");
    expect(installCommand.textContent).toContain(
      '"TINYPROBE_AGENT_TOKEN=$TINYPROBE_AGENT_TOKEN" |',
    );
    expect(installCommand.textContent).toContain(
      "sudo install -m 0755 \"$TMP_BIN\" /usr/local/bin/tinyprobe-agent",
    );
    expect(installCommand.textContent).toContain(
      "sudo install -m 0600 /dev/null /etc/tinyprobe-agent.env",
    );
    expect(installCommand.textContent).toContain(
      "sudo tee /etc/tinyprobe-agent.env >/dev/null",
    );
    expect(installCommand.textContent).toContain("unset TINYPROBE_AGENT_TOKEN");
    expect(installCommand.textContent).toContain(
      "EnvironmentFile=/etc/tinyprobe-agent.env",
    );
    expect(installCommand.textContent).toContain(
      "sudo systemctl enable --now tinyprobe-agent",
    );
    expect(installCommand.textContent).not.toContain("tp_one_time");
    expect(screen.queryByTestId("download-command")).not.toBeInTheDocument();
    expect(screen.queryByTestId("environment-file")).not.toBeInTheDocument();
    expect(screen.queryByTestId("install-commands")).not.toBeInTheDocument();
    expect(screen.queryByTestId("systemd-unit")).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain("example.com");
  });

  it("updates a persistent live region when a new token becomes ready", async () => {
    const { rerender } = render(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_first"
        onTokenCleared={vi.fn()}
      />,
    );
    const liveRegion = screen.getByTestId("token-ready-announcement");
    expect(liveRegion).toBeEmptyDOMElement();
    await waitFor(() =>
      expect(liveRegion).toHaveTextContent(
        "一次性 Agent Token 已准备好，请立即保存。",
      ),
    );

    rerender(
      <ServerInstallPanel
        serverName="home-lab"
        token="tp_second"
        onTokenCleared={vi.fn()}
      />,
    );

    expect(screen.getByTestId("token-ready-announcement")).toBe(liveRegion);
    expect(liveRegion).toBeEmptyDOMElement();
    await waitFor(() =>
      expect(liveRegion).toHaveTextContent(
        "一次性 Agent Token 已准备好，请立即保存。",
      ),
    );
  });

  it("switches the single install command to arm64", () => {
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
    expect(screen.getByTestId("install-command")).toHaveTextContent(
      "tinyprobe-agent-linux-arm64",
    );
    expect(screen.getByTestId("install-command")).not.toHaveTextContent(
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

    fireEvent.keyDown(screen.getByRole("tab", { name: "arm64" }), {
      key: "ArrowRight",
    });
    expect(screen.getByRole("tab", { name: "amd64" })).toHaveFocus();

    fireEvent.keyDown(screen.getByRole("tab", { name: "amd64" }), {
      key: "ArrowLeft",
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

    const installCommand = screen.getByTestId("install-command");
    fireEvent.click(screen.getByRole("button", { name: "复制安装命令" }));

    await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
    expect(writeText).toHaveBeenCalledWith(installCommand.textContent);
    expect(screen.getByRole("status")).toHaveTextContent("安装命令已复制");
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

    fireEvent.click(screen.getByRole("button", { name: "复制安装命令" }));

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
