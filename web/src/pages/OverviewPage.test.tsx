import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OverviewPage } from "./OverviewPage";

const fetchMock = vi.mocked(fetch);

function snapshot(
  overrides: Partial<{
    serverId: number;
    serverName: string;
    online: boolean;
  }> = {},
) {
  return {
    serverId: overrides.serverId ?? 1,
    serverName: overrides.serverName ?? "home-lab",
    online: overrides.online ?? true,
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
          usedBytes: 91_268_055_040,
        },
        {
          mountpoint: "/data",
          totalBytes: 214_748_364_800,
          usedBytes: 128_849_018_880,
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

class FakeEventSource {
  static instance: FakeEventSource | undefined;
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor() {
    FakeEventSource.instance = this;
  }

  addEventListener() {}
  close() {}

  open() {
    this.onopen?.(new Event("open"));
  }

  fail() {
    this.onerror?.(new Event("error"));
  }
}

describe("OverviewPage", () => {
  beforeEach(() => {
    FakeEventSource.instance = undefined;
    vi.stubGlobal("EventSource", FakeEventSource as unknown as typeof EventSource);
  });

  it("places offline servers before online servers", async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json([
        snapshot({ serverId: 1, serverName: "online", online: true }),
        snapshot({ serverId: 2, serverName: "offline", online: false }),
      ]),
    );

    render(<OverviewPage onNavigate={vi.fn()} />);

    const rows = await screen.findAllByTestId("server-row");
    expect(rows[0]).toHaveTextContent("offline");
    expect(rows[1]).toHaveTextContent("online");
  });

  it("shows an instructive empty state", async () => {
    fetchMock.mockResolvedValueOnce(Response.json([]));

    render(<OverviewPage onNavigate={vi.fn()} />);

    act(() => FakeEventSource.instance?.open());

    expect(await screen.findByText("还没有服务器来报到")).toBeInTheDocument();
    expect(screen.getByText(/添加服务器后/)).toBeInTheDocument();
  });

  it("shows an initial fetch error with a retry action", async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json({ error: "状态服务暂时不可用" }, { status: 503 }),
    );

    render(<OverviewPage onNavigate={vi.fn()} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "状态服务暂时不可用",
    );
    expect(
      screen.getByRole("heading", { name: "暂时无法确认服务器状态" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "服务器们都很安稳" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重新获取" })).toBeEnabled();
  });

  it("uses checking copy before the initial status request settles", () => {
    fetchMock.mockImplementationOnce(() => new Promise<Response>(() => {}));

    render(<OverviewPage onNavigate={vi.fn()} />);

    expect(
      screen.getByRole("heading", { name: "正在确认服务器状态" }),
    ).toBeInTheDocument();
    expect(screen.getByText("正在看看每台服务器的近况。"))
      .toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "服务器们都很安稳" }),
    ).not.toBeInTheDocument();
  });

  it("keeps stale current data visible when the live connection drops", async () => {
    fetchMock.mockResolvedValueOnce(Response.json([snapshot()]));
    render(<OverviewPage onNavigate={vi.fn()} />);
    expect(await screen.findByText("home-lab")).toBeInTheDocument();

    act(() => FakeEventSource.instance?.fail());

    expect(screen.getByRole("status")).toHaveTextContent("实时连接已断开");
    expect(screen.getByText("home-lab")).toBeInTheDocument();
  });

  it("marks cumulative fields as tablet-optional while core mobile metrics remain", async () => {
    fetchMock.mockResolvedValueOnce(Response.json([snapshot()]));
    render(<OverviewPage onNavigate={vi.fn()} />);

    const row = await screen.findByTestId("server-row");
    expect(row.querySelectorAll(".server-row-field--cumulative")).toHaveLength(2);
    expect(row.querySelectorAll(".server-row-field--mobile-core")).toHaveLength(5);
    expect(screen.getByText("85.0%")).toBeInTheDocument();
  });

  it("navigates to a server detail using the supplied history callback", async () => {
    const onNavigate = vi.fn();
    fetchMock.mockResolvedValueOnce(Response.json([snapshot({ serverId: 7 })]));
    render(<OverviewPage onNavigate={onNavigate} />);

    const link = await screen.findByRole("link", { name: /home-lab/ });
    link.click();

    expect(onNavigate).toHaveBeenCalledWith("/servers/7");
  });
});
