import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ServerRecord } from "./api";
import { useServerCollection } from "./useServerCollection";

const serverA: ServerRecord = {
  id: 7,
  name: "home-lab",
  enabled: true,
  agentVersion: "0.1.0",
  createdAt: "2026-07-26T04:00:00Z",
  updatedAt: "2026-07-26T04:00:00Z",
};

const serverB: ServerRecord = {
  ...serverA,
  id: 8,
  name: "office-lab",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe("useServerCollection", () => {
  it("tracks overlapping operations independently by server", async () => {
    const load = vi.fn().mockResolvedValue([serverA, serverB]);
    const first = deferred<ServerRecord>();
    const second = deferred<ServerRecord>();
    const { result } = renderHook(() => useServerCollection(load));
    await waitFor(() => expect(result.current.loadState).toBe("success"));

    let firstRequest!: Promise<unknown>;
    let secondRequest!: Promise<unknown>;
    act(() => {
      firstRequest = result.current.updateServer(
        serverA.id,
        "toggle",
        () => first.promise,
      );
      secondRequest = result.current.updateServer(
        serverB.id,
        "rename",
        () => second.promise,
      );
    });

    expect(result.current.pendingByServer[serverA.id]?.action).toBe("toggle");
    expect(result.current.pendingByServer[serverB.id]?.action).toBe("rename");

    await act(async () => {
      first.resolve({ ...serverA, enabled: false });
      await firstRequest;
    });

    expect(result.current.pendingByServer[serverA.id]).toBeUndefined();
    expect(result.current.pendingByServer[serverB.id]?.action).toBe("rename");

    await act(async () => {
      second.resolve({ ...serverB, name: "office-new" });
      await secondRequest;
    });

    expect(result.current.pendingByServer[serverB.id]).toBeUndefined();
    expect(result.current.servers).toEqual([
      { ...serverA, enabled: false },
      { ...serverB, name: "office-new" },
    ]);
  });

  it("ignores an older whole-record response for the same server", async () => {
    const load = vi.fn().mockResolvedValue([serverA]);
    const older = deferred<ServerRecord>();
    const newer = deferred<ServerRecord>();
    const { result } = renderHook(() => useServerCollection(load));
    await waitFor(() => expect(result.current.loadState).toBe("success"));

    let olderRequest!: Promise<unknown>;
    let newerRequest!: Promise<unknown>;
    act(() => {
      olderRequest = result.current.updateServer(
        serverA.id,
        "rename",
        () => older.promise,
      );
      newerRequest = result.current.updateServer(
        serverA.id,
        "toggle",
        () => newer.promise,
      );
    });

    await act(async () => {
      older.resolve({ ...serverA, name: "stale-name" });
      await olderRequest;
    });
    expect(result.current.pendingByServer[serverA.id]?.action).toBe("toggle");
    expect(result.current.servers[0]).toEqual(serverA);

    await act(async () => {
      newer.resolve({ ...serverA, enabled: false, name: "newest-name" });
      await newerRequest;
    });
    expect(result.current.servers[0]).toEqual({
      ...serverA,
      enabled: false,
      name: "newest-name",
    });
  });

  it("does not let a list response overwrite a mutation started later", async () => {
    const staleList = deferred<ServerRecord[]>();
    const mutation = deferred<ServerRecord>();
    const load = vi
      .fn<() => Promise<ServerRecord[]>>()
      .mockResolvedValueOnce([serverA])
      .mockReturnValueOnce(staleList.promise);
    const { result } = renderHook(() => useServerCollection(load));
    await waitFor(() => expect(result.current.loadState).toBe("success"));

    let reloadRequest!: Promise<void>;
    let mutationRequest!: Promise<unknown>;
    act(() => {
      reloadRequest = result.current.loadServers();
      mutationRequest = result.current.updateServer(
        serverA.id,
        "toggle",
        () => mutation.promise,
      );
    });

    await act(async () => {
      mutation.resolve({ ...serverA, enabled: false });
      await mutationRequest;
    });
    await act(async () => {
      staleList.resolve([serverA]);
      await reloadRequest;
    });

    expect(result.current.servers[0]).toEqual({ ...serverA, enabled: false });
  });

  it("does not let a list started during a mutation overwrite its result", async () => {
    const staleList = deferred<ServerRecord[]>();
    const mutation = deferred<ServerRecord>();
    const load = vi
      .fn<() => Promise<ServerRecord[]>>()
      .mockResolvedValueOnce([serverA])
      .mockReturnValueOnce(staleList.promise);
    const { result } = renderHook(() => useServerCollection(load));
    await waitFor(() => expect(result.current.loadState).toBe("success"));

    let reloadRequest!: Promise<void>;
    let mutationRequest!: Promise<unknown>;
    act(() => {
      mutationRequest = result.current.updateServer(
        serverA.id,
        "toggle",
        () => mutation.promise,
      );
    });
    act(() => {
      reloadRequest = result.current.loadServers();
    });

    await act(async () => {
      mutation.resolve({ ...serverA, enabled: false });
      await mutationRequest;
    });
    await act(async () => {
      staleList.resolve([serverA]);
      await reloadRequest;
    });

    expect(result.current.servers[0]).toEqual({ ...serverA, enabled: false });
  });
});
