import { useCallback, useEffect, useRef, useState } from "react";

import { apiErrorMessage, request } from "../api/client";
import type { HistoryResponse, MinuteRecord, Range } from "./types";

const CACHE_TTL_MS = 60_000;
const HISTORY_ERROR = "暂时无法获取历史指标，请稍后重试。";
export const MAX_HISTORY_SPAN_SECONDS = 30 * 24 * 60 * 60;

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
  const age = Date.now() - entry.cachedAt;
  if (age < 0 || age >= CACHE_TTL_MS) {
    historyCache.delete(key);
    return null;
  }
  return entry.data;
}

export function isValidHistoryRange(fromUnix: number, toUnix: number): boolean {
  return (
    Number.isSafeInteger(fromUnix) &&
    Number.isSafeInteger(toUnix) &&
    Number.isSafeInteger(fromUnix * 1_000) &&
    Number.isSafeInteger(toUnix * 1_000) &&
    fromUnix % 60 === 0 &&
    toUnix % 60 === 0 &&
    fromUnix <= toUnix &&
    toUnix - fromUnix <= MAX_HISTORY_SPAN_SECONDS
  );
}

function isHistoryResponse(value: unknown): value is HistoryResponse {
  if (value === null || Array.isArray(value) || typeof value !== "object") {
    return false;
  }
  const candidate = value as {
    fromUnix?: unknown;
    toUnix?: unknown;
    points?: unknown;
  };
  return (
    typeof candidate.fromUnix === "number" &&
    typeof candidate.toUnix === "number" &&
    isValidHistoryRange(candidate.fromUnix, candidate.toUnix) &&
    Array.isArray(candidate.points)
  );
}

export function buildMinuteSeries(
  points: readonly MinuteRecord[],
  fromUnix: number,
  toUnix: number,
  selector: (point: MinuteRecord) => number | null,
): MinuteValue[] {
  if (!isValidHistoryRange(fromUnix, toUnix)) {
    return [];
  }

  const byMinute = new Map<number, MinuteRecord>();
  for (const point of points) {
    if (
      !Number.isSafeInteger(point.minuteUnix) ||
      point.minuteUnix % 60 !== 0 ||
      point.minuteUnix < fromUnix ||
      point.minuteUnix > toUnix
    ) {
      continue;
    }
    byMinute.set(point.minuteUnix, point);
  }

  const series: MinuteValue[] = [];
  for (let minute = fromUnix; minute <= toUnix; minute += 60) {
    const point = byMinute.get(minute);
    let value: number | null = null;
    if (point) {
      try {
        const selected = selector(point);
        value = selected !== null && Number.isFinite(selected) ? selected : null;
      } catch {
        value = null;
      }
    }
    series.push([minute * 1_000, value]);
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
    request<unknown>(
      `/api/servers/${serverID}/history?range=${range}`,
      { signal: controller.signal },
    )
      .then((value) => {
        if (
          controller.signal.aborted ||
          generation !== generationRef.current
        ) {
          return;
        }
        if (!isHistoryResponse(value)) {
          throw new Error("invalid history response");
        }
        const data = value;
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
