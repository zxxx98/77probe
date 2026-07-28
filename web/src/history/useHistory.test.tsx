import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { HistoryResponse, MinuteRecord } from "./types";
import { buildMinuteSeries, useHistory } from "./useHistory";

const fetchMock = vi.mocked(fetch);

function point(minuteUnix: number, cpuAverage: number): MinuteRecord {
  return {
    serverId: 7,
    minuteUnix,
    payload: {
      cpuUsage: { average: cpuAverage, maximum: cpuAverage + 5 },
      load1: { average: 0.1, maximum: 0.2 },
      load5: { average: 0.2, maximum: 0.3 },
      load15: { average: 0.3, maximum: 0.4 },
      memoryUsage: { average: 40, maximum: 45 },
      swapUsage: { average: 0, maximum: 0 },
      disks: [],
      diskReadBps: { average: 1_024, maximum: 2_048 },
      diskWriteBps: { average: 2_048, maximum: 4_096 },
      uploadBps: { average: 4_096, maximum: 8_192 },
      downloadBps: { average: 8_192, maximum: 16_384 },
      totalUpload: 10_000,
      totalDownload: 20_000,
    },
  };
}

function response(
  points: MinuteRecord[],
  fromUnix = 600,
  toUnix = 720,
): HistoryResponse {
  return { fromUnix, toUnix, points };
}

function deferredResponse() {
  let resolve!: (value: Response) => void;
  const promise = new Promise<Response>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

afterEach(() => {
  vi.useRealTimers();
});

describe("buildMinuteSeries", () => {
  it("inserts null for a missing minute instead of forward filling", () => {
    const series = buildMinuteSeries(
      [point(600, 10), point(720, 30)],
      600,
      720,
      (value) => value.payload.cpuUsage.average,
    );

    expect(series).toEqual([
      [600_000, 10],
      [660_000, null],
      [720_000, 30],
    ]);
  });

  it("orders minutes, ignores points outside the range, and uses the last duplicate", () => {
    const series = buildMinuteSeries(
      [point(720, 30), point(540, 1), point(600, 10), point(600, 12), point(721, 99)],
      600,
      720,
      (value) => value.payload.cpuUsage.average,
    );

    expect(series).toEqual([
      [600_000, 12],
      [660_000, null],
      [720_000, 30],
    ]);
  });

  it("rejects unsafe, off-minute, reversed, and oversized ranges", () => {
    const thirtyDays = 30 * 24 * 60 * 60;

    expect(buildMinuteSeries([], 600, 600 + thirtyDays + 60, () => 1)).toEqual(
      [],
    );
    expect(buildMinuteSeries([], 601, 720, () => 1)).toEqual([]);
    expect(buildMinuteSeries([], 720, 600, () => 1)).toEqual([]);
    expect(
      buildMinuteSeries(
        [],
        Number.MAX_SAFE_INTEGER - 60,
        Number.MAX_SAFE_INTEGER,
        () => 1,
      ),
    ).toEqual([]);
    const alignedNearMax = Math.floor(Number.MAX_SAFE_INTEGER / 60) * 60;
    expect(
      buildMinuteSeries([], alignedNearMax - 60, alignedNearMax, () => 1),
    ).toEqual([]);
  });

  it("ignores off-minute points and nonfinite selector values", () => {
    const points = [point(600, 10), point(601, 99), point(660, 20), point(720, 30)];

    expect(
      buildMinuteSeries(points, 600, 720, (value) =>
        value.minuteUnix === 660 ? Number.NaN : value.payload.cpuUsage.average,
      ),
    ).toEqual([
      [600_000, 10],
      [660_000, null],
      [720_000, 30],
    ]);
    expect(
      buildMinuteSeries([point(600, 10)], 600, 600, () => Number.POSITIVE_INFINITY),
    ).toEqual([[600_000, null]]);
  });
});

describe("useHistory", () => {
  it("does not request history for the real-time range", () => {
    const { result } = renderHook(() => useHistory(7, null));

    expect(result.current).toMatchObject({
      loading: false,
      error: null,
      data: null,
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("fetches the selected server and range", async () => {
    fetchMock.mockResolvedValueOnce(Response.json(response([point(600, 10)])));

    const { result } = renderHook(() => useHistory(7, "1d"));

    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toEqual(response([point(600, 10)]));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/servers/7/history?range=1d",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it("aborts the previous key and ignores its stale response", async () => {
    const oldRequest = deferredResponse();
    const newData = response([point(720, 70)]);
    fetchMock
      .mockImplementationOnce((_input, init) => {
        expect(init?.signal).toBeInstanceOf(AbortSignal);
        return oldRequest.promise;
      })
      .mockResolvedValueOnce(Response.json(newData));

    const initialProps: { serverId: number; range: "1d" | "7d" } = {
      serverId: 307,
      range: "1d",
    };
    const { result, rerender } = renderHook(
      ({ serverId, range }: { serverId: number; range: "1d" | "7d" }) =>
        useHistory(serverId, range),
      { initialProps },
    );
    const oldSignal = fetchMock.mock.calls[0]?.[1]?.signal;

    rerender({ serverId: 308, range: "7d" });

    expect(oldSignal?.aborted).toBe(true);
    expect(result.current.data).toBeNull();
    await waitFor(() => expect(result.current.data).toEqual(newData));

    await act(async () => {
      oldRequest.resolve(Response.json(response([point(600, 10)])));
    });

    expect(result.current.data).toEqual(newData);
    expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
      "/api/servers/307/history?range=1d",
      "/api/servers/308/history?range=7d",
    ]);
  });

  it("aborts an in-flight request on unmount", () => {
    const pending = deferredResponse();
    fetchMock.mockImplementationOnce(() => pending.promise);

    const { unmount } = renderHook(() => useHistory(17, "30d"));
    const signal = fetchMock.mock.calls[0]?.[1]?.signal;
    unmount();

    expect(signal?.aborted).toBe(true);
  });

  it("caches successful responses per server and range for 60 seconds", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-28T00:00:00Z"));
    const oneDay = response([point(600, 10)]);
    const sevenDays = response([point(720, 70)]);
    fetchMock
      .mockResolvedValueOnce(Response.json(oneDay))
      .mockResolvedValueOnce(Response.json(sevenDays))
      .mockResolvedValueOnce(Response.json(oneDay));

    const first = renderHook(() => useHistory(107, "1d"));
    await act(async () => Promise.resolve());
    expect(first.result.current.data).toEqual(oneDay);
    first.unmount();

    vi.setSystemTime(new Date("2026-07-28T00:00:59Z"));
    const cached = renderHook(() => useHistory(107, "1d"));
    expect(cached.result.current).toMatchObject({ loading: false, data: oneDay });
    cached.unmount();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const otherKey = renderHook(() => useHistory(107, "7d"));
    await act(async () => Promise.resolve());
    expect(otherKey.result.current.data).toEqual(sevenDays);
    otherKey.unmount();
    expect(fetchMock).toHaveBeenCalledTimes(2);

    vi.setSystemTime(new Date("2026-07-28T00:01:01Z"));
    const expired = renderHook(() => useHistory(107, "1d"));
    expect(expired.result.current.loading).toBe(true);
    await act(async () => Promise.resolve());
    expect(expired.result.current.data).toEqual(oneDay);
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("returns a safe error and retries the current key", async () => {
    fetchMock
      .mockResolvedValueOnce(new Response("upstream exploded", { status: 500 }))
      .mockResolvedValueOnce(Response.json(response([point(600, 42)])));

    const { result } = renderHook(() => useHistory(207, "1d"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBe("暂时无法获取历史指标，请稍后重试。");
    expect(result.current.data).toBeNull();

    act(() => result.current.retry());

    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.data).not.toBeNull());
    expect(result.current.error).toBeNull();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("rejects a malformed history response with the normal safe error", async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json({ fromUnix: 601, toUnix: 720, points: [] }),
    );

    const { result } = renderHook(() => useHistory(507, "1d"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data).toBeNull();
    expect(result.current.error).toBe("暂时无法获取历史指标，请稍后重试。");
  });

  it("expires a cache entry when the clock moves backwards", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-28T00:01:00Z"));
    const cached = response([point(600, 10)]);
    fetchMock
      .mockResolvedValueOnce(Response.json(cached))
      .mockResolvedValueOnce(Response.json(cached));

    const first = renderHook(() => useHistory(607, "1d"));
    await act(async () => Promise.resolve());
    expect(first.result.current.data).toEqual(cached);
    first.unmount();

    vi.setSystemTime(new Date("2026-07-28T00:00:59Z"));
    const rolledBack = renderHook(() => useHistory(607, "1d"));
    expect(rolledBack.result.current.loading).toBe(true);
    await act(async () => Promise.resolve());

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(rolledBack.result.current.data).toEqual(cached);
  });
});
