import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DashboardRouter } from "./App";

const fetchMock = vi.mocked(fetch);

class QuietEventSource {
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  addEventListener() {}
  close() {}
}

function detailSnapshot() {
  return {
    serverId: 7,
    serverName: "home-lab",
    online: true,
    lastReceivedAt: "2026-07-26T04:00:00Z",
    sourceIp: "192.0.2.10",
    report: {
      collectedAtUnix: 1_753_588_800,
      agentVersion: "0.1.0",
      host: {
        hostname: "home-lab",
        os: "linux",
        platform: "ubuntu",
        platformVersion: "24.04",
        kernelVersion: "6.8.0",
        architecture: "amd64",
        cpuModel: "Tiny CPU",
        cpuCores: 4,
        primaryIp: "192.0.2.10",
        bootTimeUnix: 1_752_724_800,
        uptimeSeconds: 864_000,
      },
      cpu: { usagePercent: 42, load1: 0.2, load5: 0.3, load15: 0.4 },
      memory: {
        totalBytes: 8_589_934_592,
        usedBytes: 4_294_967_296,
        swapTotalBytes: 2_147_483_648,
        swapUsedBytes: 536_870_912,
      },
      disks: [
        {
          mountpoint: "/",
          totalBytes: 107_374_182_400,
          usedBytes: 53_687_091_200,
        },
      ],
      diskIo: { readBytesPerSecond: 1_024, writeBytesPerSecond: 2_048 },
      network: {
        interface: "eth0",
        uploadBytesPerSecond: 4_096,
        downloadBytesPerSecond: 8_192,
        totalUploadBytes: 10_737_418_240,
        totalDownloadBytes: 21_474_836_480,
      },
    },
  };
}

describe("DashboardRouter", () => {
  beforeEach(() => {
    window.history.replaceState(null, "", "/servers/7");
    vi.stubGlobal("EventSource", QuietEventSource as unknown as typeof EventSource);
  });

  it("renders detail data from a server path and returns through browser history", async () => {
    fetchMock.mockResolvedValueOnce(Response.json(detailSnapshot()));
    render(<DashboardRouter />);

    expect(
      await screen.findByRole("heading", { name: "home-lab" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Tiny CPU")).toBeInTheDocument();
    expect(screen.getByText("采集时间")).toBeInTheDocument();
    expect(screen.getByText("启动时间")).toBeInTheDocument();
    expect(screen.getByText("0.20 / 0.30 / 0.40")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "实时" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "1天" })).toBeDisabled();

    fireEvent.click(screen.getByRole("link", { name: "返回概览" }));

    expect(window.location.pathname).toBe("/");
    expect(await screen.findByRole("heading", { name: /服务器/ })).toBeInTheDocument();
  });

  it("responds to popstate navigation", async () => {
    fetchMock.mockResolvedValueOnce(Response.json(detailSnapshot()));
    render(<DashboardRouter />);
    await screen.findByRole("heading", { name: "home-lab" });

    fetchMock.mockResolvedValueOnce(Response.json([]));
    window.history.pushState(null, "", "/");
    window.dispatchEvent(new PopStateEvent("popstate"));

    expect(await screen.findByText("还没有服务器来报到")).toBeInTheDocument();
  });
});
