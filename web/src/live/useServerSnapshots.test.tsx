import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useServerSnapshots } from "./useServerSnapshots";

const fetchMock = vi.mocked(fetch);

function snapshot(
  overrides: Partial<{
    serverId: number;
    serverName: string;
    online: boolean;
    cpuUsage: number;
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
      cpu: {
        usagePercent: overrides.cpuUsage ?? 12,
        load1: 0.2,
        load5: 0.3,
        load15: 0.4,
      },
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
      diskIo: {
        readBytesPerSecond: 1_024,
        writeBytesPerSecond: 2_048,
      },
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
  static instances: FakeEventSource[] = [];

  readonly url: string;
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  closed = false;
  private listeners = new Map<string, Set<(event: MessageEvent<string>) => void>>();

  constructor(url: string | URL) {
    this.url = String(url);
    FakeEventSource.instances.push(this);
  }

  addEventListener(
    type: string,
    listener: (event: MessageEvent<string>) => void,
  ) {
    const listeners = this.listeners.get(type) ?? new Set();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  close() {
    this.closed = true;
  }

  open() {
    this.onopen?.(new Event("open"));
  }

  fail() {
    this.onerror?.(new Event("error"));
  }

  emit(type: "snapshot.updated" | "snapshot.offline", value: ReturnType<typeof snapshot>) {
    const event = new MessageEvent(type, {
      data: JSON.stringify({ type, snapshot: value }),
    });
    this.listeners.get(type)?.forEach((listener) => listener(event));
  }
}

function deferredResponse() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("useServerSnapshots", () => {
  beforeEach(() => {
    FakeEventSource.instances = [];
    vi.stubGlobal("EventSource", FakeEventSource as unknown as typeof EventSource);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.stubGlobal("fetch", fetchMock);
  });

  it("fetches initial snapshots and opens exactly one live stream", async () => {
    fetchMock.mockResolvedValueOnce(Response.json([snapshot()]));

    const { result } = renderHook(() => useServerSnapshots());

    await waitFor(() => expect(result.current.snapshots).toHaveLength(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/servers/status",
      expect.objectContaining({
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        signal: expect.any(AbortSignal),
      }),
    );
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(FakeEventSource.instances[0].url).toBe("/api/live");
  });

  it("exposes exactly the documented public contract", () => {
    fetchMock.mockImplementationOnce(() => new Promise<Response>(() => {}));

    const { result } = renderHook(() => useServerSnapshots());

    expect(Object.keys(result.current).sort()).toEqual([
      "connected",
      "error",
      "refresh",
      "snapshots",
    ]);
  });

  it("reports the EventSource connection while the initial fetch is still pending", () => {
    fetchMock.mockImplementationOnce(() => new Promise<Response>(() => {}));

    const { result } = renderHook(() => useServerSnapshots());

    act(() => FakeEventSource.instances[0].open());

    expect(result.current.connected).toBe(true);
    expect(result.current.snapshots).toEqual([]);

    act(() => FakeEventSource.instances[0].fail());

    expect(result.current.connected).toBe(false);
  });

  it("merges an SSE snapshot by server id", async () => {
    fetchMock.mockResolvedValueOnce(Response.json([snapshot({ cpuUsage: 12 })]));
    const { result } = renderHook(() => useServerSnapshots());
    await waitFor(() => expect(result.current.snapshots).toHaveLength(1));

    act(() => {
      FakeEventSource.instances[0].emit(
        "snapshot.updated",
        snapshot({ serverId: 1, online: true, cpuUsage: 42 }),
      );
    });

    await waitFor(() =>
      expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(42),
    );
  });

  it("does not let a stale initial fetch overwrite a newer SSE snapshot", async () => {
    const initialResponse = deferredResponse();
    fetchMock.mockImplementationOnce(() => initialResponse.promise);
    const { result } = renderHook(() => useServerSnapshots());

    act(() => {
      FakeEventSource.instances[0].emit(
        "snapshot.updated",
        snapshot({ serverId: 1, cpuUsage: 42 }),
      );
    });
    expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(42);

    await act(async () => {
      initialResponse.resolve(Response.json([snapshot({ serverId: 1, cpuUsage: 12 })]));
    });

    expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(42);
  });

  it("does not let an older refresh overwrite a newer refresh", async () => {
    const olderResponse = deferredResponse();
    const newerResponse = deferredResponse();
    fetchMock
      .mockResolvedValueOnce(Response.json([snapshot({ cpuUsage: 10 })]))
      .mockImplementationOnce(() => olderResponse.promise)
      .mockImplementationOnce(() => newerResponse.promise);
    const { result } = renderHook(() => useServerSnapshots());
    await waitFor(() => expect(result.current.snapshots).toHaveLength(1));

    let olderRefresh!: Promise<void>;
    let newerRefresh!: Promise<void>;
    act(() => {
      olderRefresh = result.current.refresh();
      newerRefresh = result.current.refresh();
    });

    await act(async () => {
      newerResponse.resolve(Response.json([snapshot({ cpuUsage: 30 })]));
      await newerRefresh;
    });
    expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(30);

    await act(async () => {
      olderResponse.resolve(Response.json([snapshot({ cpuUsage: 20 })]));
      await olderRefresh;
    });
    expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(30);
  });

  it("preserves a server introduced by SSE after a refresh begins", async () => {
    const refreshResponse = deferredResponse();
    fetchMock
      .mockResolvedValueOnce(
        Response.json([snapshot({ serverId: 1, serverName: "alpha" })]),
      )
      .mockImplementationOnce(() => refreshResponse.promise);
    const { result } = renderHook(() => useServerSnapshots());
    await waitFor(() => expect(result.current.snapshots).toHaveLength(1));

    let refreshPromise!: Promise<void>;
    act(() => {
      refreshPromise = result.current.refresh();
    });
    act(() => {
      FakeEventSource.instances[0].emit(
        "snapshot.updated",
        snapshot({ serverId: 2, serverName: "beta", cpuUsage: 42 }),
      );
    });

    await act(async () => {
      refreshResponse.resolve(
        Response.json([snapshot({ serverId: 1, serverName: "alpha" })]),
      );
      await refreshPromise;
    });

    expect(result.current.snapshots.map(({ serverId }) => serverId)).toEqual([1, 2]);
  });

  it("applies offline events and keeps offline servers first", async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json([
        snapshot({ serverId: 1, serverName: "alpha" }),
        snapshot({ serverId: 2, serverName: "beta" }),
      ]),
    );
    const { result } = renderHook(() => useServerSnapshots());
    await waitFor(() => expect(result.current.snapshots).toHaveLength(2));

    act(() => {
      FakeEventSource.instances[0].emit(
        "snapshot.offline",
        snapshot({ serverId: 2, serverName: "beta", online: false }),
      );
    });

    expect(result.current.snapshots.map(({ serverName }) => serverName)).toEqual([
      "beta",
      "alpha",
    ]);
    expect(result.current.snapshots[0].online).toBe(false);
  });

  it("refetches after reconnect without discarding current data", async () => {
    const reconnectResponse = deferredResponse();
    fetchMock
      .mockResolvedValueOnce(Response.json([snapshot({ cpuUsage: 12 })]))
      .mockImplementationOnce(() => reconnectResponse.promise);
    const { result } = renderHook(() => useServerSnapshots());
    await waitFor(() => expect(result.current.snapshots).toHaveLength(1));

    act(() => FakeEventSource.instances[0].open());
    act(() => FakeEventSource.instances[0].fail());
    expect(result.current.connected).toBe(false);

    act(() => FakeEventSource.instances[0].open());
    expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(12);
    expect(fetchMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      reconnectResponse.resolve(Response.json([snapshot({ cpuUsage: 33 })]));
    });
    await waitFor(() =>
      expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(33),
    );
    expect(result.current.connected).toBe(true);
  });

  it("refetches when the first successful open follows a connection error", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json([snapshot({ cpuUsage: 12 })]))
      .mockResolvedValueOnce(Response.json([snapshot({ cpuUsage: 33 })]));
    const { result } = renderHook(() => useServerSnapshots());
    await waitFor(() => expect(result.current.snapshots).toHaveLength(1));

    act(() => FakeEventSource.instances[0].fail());
    act(() => FakeEventSource.instances[0].open());

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(result.current.snapshots[0].report.cpu.usagePercent).toBe(33),
    );
  });

  it("preserves current snapshots when a refresh fails", async () => {
    fetchMock
      .mockResolvedValueOnce(Response.json([snapshot()]))
      .mockResolvedValueOnce(
        Response.json({ error: "状态服务暂时不可用" }, { status: 503 }),
      );
    const { result } = renderHook(() => useServerSnapshots());
    await waitFor(() => expect(result.current.snapshots).toHaveLength(1));

    await act(async () => result.current.refresh());

    expect(result.current.snapshots).toHaveLength(1);
    expect(result.current.error).toBe("状态服务暂时不可用");
  });

  it("closes the stream and ignores a late initial response after unmount", async () => {
    const initialResponse = deferredResponse();
    fetchMock.mockImplementationOnce(() => initialResponse.promise);
    const { result, unmount } = renderHook(() => useServerSnapshots());
    const stream = FakeEventSource.instances[0];

    unmount();
    expect(stream.closed).toBe(true);

    await act(async () => {
      initialResponse.resolve(Response.json([snapshot()]));
    });
    expect(result.current.snapshots).toEqual([]);
  });
});
