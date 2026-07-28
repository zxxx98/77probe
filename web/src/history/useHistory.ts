import { useCallback, useEffect, useRef, useState } from "react";

import { apiErrorMessage, request } from "../api/client";
import type { HistoryResponse, MinuteRecord, Range } from "./types";

const CACHE_TTL_MS = 60_000;
const HISTORY_ERROR = "暂时无法获取历史指标，请稍后重试。";

type MinuteValue = [timestampMs: number, value: number | null];

interface CacheEntry {
  cachedAt: number;
  data: HistoryResponse;
}

interface HistoryState {
  key: string | null;
  loading: boolean;
  error: string | null;
  data: HistoryResponse | null;
}

export interface UseHistoryState {
  loading: boolean;
  error: string | null;
  data: HistoryResponse | null;
  retry: () => void;
}

const historyCache = new Map<string, CacheEntry>();

function cacheKey(serverID: number, range: Range): string {
  return `${serverID}:${range}`;
}

function cachedHistory(key: string): HistoryResponse | null {
  const entry = historyCache.get(key);
  if (!entry) {
    return null;
  }
  if (Date.now() - entry.cachedAt >= CACHE_TTL_MS) {
    historyCache.delete(key);
    return null;
  }
  return entry.data;
}

export function buildMinuteSeries(
  points: readonly MinuteRecord[],
  fromUnix: number,
  toUnix: number,
  selector: (point: MinuteRecord) => number | null,
): MinuteValue[] {
  if (!Number.isFinite(fromUnix) || !Number.isFinite(toUnix)) {
    return [];
  }
  const firstMinute = Math.floor(fromUnix / 60) * 60;
  const lastMinute = Math.floor(toUnix / 60) * 60;
  if (lastMinute < firstMinute) {
    return [];
  }

  const byMinute = new Map<number, MinuteRecord>();
  for (const point of points) {
    if (point.minuteUnix < fromUnix || point.minuteUnix > toUnix) {
      continue;
    }
    const minute = Math.floor(point.minuteUnix / 60) * 60;
    if (minute >= firstMinute && minute <= lastMinute) {
      byMinute.set(minute, point);
    }
  }

  const series: MinuteValue[] = [];
  for (let minute = firstMinute; minute <= lastMinute; minute += 60) {
    const point = byMinute.get(minute);
    series.push([minute * 1_000, point ? selector(point) : null]);
  }
  return series;
}

export function useHistory(
  serverID: number,
  range: Range | null,
): UseHistoryState {
  const key = range === null ? null : cacheKey(serverID, range);
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<HistoryState>(() => ({
    key,
    loading: key !== null && cachedHistory(key) === null,
    error: null,
    data: key === null ? null : cachedHistory(key),
  }));
  const generationRef = useRef(0);

  useEffect(() => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;

    if (key === null || range === null) {
      setState({ key: null, loading: false, error: null, data: null });
      return;
    }

    const cached = cachedHistory(key);
    if (cached !== null) {
      setState({ key, loading: false, error: null, data: cached });
      return;
    }

    const controller = new AbortController();
    setState({ key, loading: true, error: null, data: null });
    request<HistoryResponse>(
      `/api/servers/${serverID}/history?range=${range}`,
      { signal: controller.signal },
    )
      .then((data) => {
        if (
          controller.signal.aborted ||
          generation !== generationRef.current
        ) {
          return;
        }
        historyCache.set(key, { cachedAt: Date.now(), data });
        setState({ key, loading: false, error: null, data });
      })
      .catch((error: unknown) => {
        if (
          controller.signal.aborted ||
          generation !== generationRef.current
        ) {
          return;
        }
        setState({
          key,
          loading: false,
          error: apiErrorMessage(error, HISTORY_ERROR),
          data: null,
        });
      });

    return () => controller.abort();
  }, [attempt, key, range, serverID]);

  const retry = useCallback(() => {
    if (key !== null) {
      historyCache.delete(key);
      setAttempt((value) => value + 1);
    }
  }, [key]);

  if (key === null) {
    return { loading: false, error: null, data: null, retry };
  }
  if (state.key !== key) {
    const cached = cachedHistory(key);
    return {
      loading: cached === null,
      error: null,
      data: cached,
      retry,
    };
  }
  return {
    loading: state.loading,
    error: state.error,
    data: state.data,
    retry,
  };
}
