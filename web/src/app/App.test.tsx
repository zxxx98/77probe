import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DashboardRouter } from "./App";

const fetchMock = vi.mocked(fetch);

class QuietEventSource {
  static instance: QuietEventSource | undefined;
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor() {
    QuietEventSource.instance = this;
  }

  addEventListener() {}
  close() {}

  open() {
    this.onopen?.(new Event("open"));
  }
}

function detailSnapshot(
  overrides: Partial<{ serverId: number; serverName: string }> = {},
) {
  return {
    serverId: overrides.serverId ?? 7,
    serverName: overrides.serverName ?? "home-lab",
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

function deferredResponse() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("DashboardRouter", () => {
  beforeEach(() => {
    QuietEventSource.instance = undefined;
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
    await waitFor(() => expect(QuietEventSource.instance).toBeDefined());
    act(() => QuietEventSource.instance?.open());

    expect(await screen.findByText("还没有服务器来报到")).toBeInTheDocument();
  });

  it("loads a new server on detail-to-detail popstate navigation", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json(detailSnapshot()))
      .mockResolvedValueOnce(
        Response.json(detailSnapshot({ serverId: 8, serverName: "office-lab" })),
      );
    render(<DashboardRouter />);
    await screen.findByRole("heading", { name: "home-lab" });

    window.history.pushState(null, "", "/servers/8");
    window.dispatchEvent(new PopStateEvent("popstate"));

    expect(
      await screen.findByRole("heading", { name: "office-lab" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "home-lab" }),
    ).not.toBeInTheDocument();
  });

  it("ignores a stale response from the previous detail route", async () => {
    const oldResponse = deferredResponse();
    fetchMock
      .mockImplementationOnce(() => oldResponse.promise)
      .mockResolvedValueOnce(
        Response.json(detailSnapshot({ serverId: 8, serverName: "office-lab" })),
      );
    render(<DashboardRouter />);

    window.history.pushState(null, "", "/servers/8");
    window.dispatchEvent(new PopStateEvent("popstate"));
    await screen.findByRole("heading", { name: "office-lab" });

    await act(async () => {
      oldResponse.resolve(Response.json(detailSnapshot()));
    });

    expect(screen.getByRole("heading", { name: "office-lab" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "home-lab" })).not.toBeInTheDocument();
  });

  it("does not leave the previous server visible when the destination fails", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json(detailSnapshot()))
      .mockResolvedValueOnce(
        Response.json({ error: "server not found" }, { status: 404 }),
      );
    render(<DashboardRouter />);
    await screen.findByRole("heading", { name: "home-lab" });

    window.history.pushState(null, "", "/servers/8");
    window.dispatchEvent(new PopStateEvent("popstate"));

    expect(await screen.findByRole("alert")).toHaveTextContent("server not found");
    expect(screen.queryByRole("heading", { name: "home-lab" })).not.toBeInTheDocument();
  });

  it("rejects an invalid detail destination without fetching", async () => {
    window.history.replaceState(null, "", "/servers/0");

    render(<DashboardRouter />);

    expect(
      screen.getByRole("heading", { name: "服务器地址无效" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(fetchMock).not.toHaveBeenCalled());
  });

  it("routes the server-management navigation item to the management page", async () => {
    window.history.replaceState(null, "", "/servers");
    fetchMock.mockResolvedValueOnce(Response.json([]));

    render(<DashboardRouter />);

    expect(
      await screen.findByRole("heading", { name: "服务器管理" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "服务器" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "概览" })).not.toHaveAttribute(
      "aria-current",
    );
  });
});
