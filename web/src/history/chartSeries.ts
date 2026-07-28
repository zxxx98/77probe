import type { HistoryResponse, MinuteRecord } from "./types";
import { isValidHistoryRange } from "./useHistory";

export type MetricSeriesRole =
  | "primary"
  | "maximum"
  | "context"
  | "context-maximum";

export interface MetricSeriesDefinition {
  name: string;
  role: MetricSeriesRole;
  pairOrdinal: number;
  selector: (point: MinuteRecord) => number | null;
}

export interface MetricSeriesStats {
  /** Most recent finite completed-minute value in the selected range. */
  current: number | null;
  average: number | null;
  maximum: number | null;
}

export type MetricChartPoint = [timestampMs: number, value: number | null];

export interface PreparedMetricSeries extends MetricSeriesDefinition {
  data: MetricChartPoint[];
  stats: MetricSeriesStats;
}

export interface PreparedChartSeries {
  valid: boolean;
  minuteCount: number;
  pointBudget: number;
  series: PreparedMetricSeries[];
}

export interface AlignedHistoryIndex {
  fromUnix: number;
  toUnix: number;
  minuteCount: number;
  records: Array<MinuteRecord | null>;
}

const MIN_POINT_BUDGET = 160;
const MAX_POINT_BUDGET = 1_600;
const DEFAULT_CHART_WIDTH = 640;
const POINTS_PER_PIXEL = 2;
const FINITE_CANDIDATES_PER_BUCKET = 4;

const alignedIndexCache = new WeakMap<
  HistoryResponse,
  AlignedHistoryIndex | null
>();

export function chartPointBudget(width: number): number {
  const safeWidth = Number.isFinite(width) && width > 0 ? width : DEFAULT_CHART_WIDTH;
  return Math.min(
    MAX_POINT_BUDGET,
    Math.max(MIN_POINT_BUDGET, Math.round(safeWidth * POINTS_PER_PIXEL)),
  );
}

export function getAlignedHistoryIndex(
  history: HistoryResponse,
): AlignedHistoryIndex | null {
  if (alignedIndexCache.has(history)) {
    return alignedIndexCache.get(history) ?? null;
  }
  if (
    !isValidHistoryRange(history.fromUnix, history.toUnix) ||
    !Array.isArray(history.points)
  ) {
    alignedIndexCache.set(history, null);
    return null;
  }

  const minuteCount = (history.toUnix - history.fromUnix) / 60 + 1;
  const records = Array<MinuteRecord | null>(minuteCount).fill(null);
  for (const value of history.points as unknown[]) {
    if (value === null || Array.isArray(value) || typeof value !== "object") {
      continue;
    }
    const point = value as MinuteRecord;
    if (
      !Number.isSafeInteger(point.minuteUnix) ||
      point.minuteUnix % 60 !== 0 ||
      point.minuteUnix < history.fromUnix ||
      point.minuteUnix > history.toUnix
    ) {
      continue;
    }
    const index = (point.minuteUnix - history.fromUnix) / 60;
    records[index] = point;
  }

  const aligned = {
    fromUnix: history.fromUnix,
    toUnix: history.toUnix,
    minuteCount,
    records,
  };
  alignedIndexCache.set(history, aligned);
  return aligned;
}

function selectedValue(
  definition: MetricSeriesDefinition,
  point: MinuteRecord | null,
): number | null {
  if (!point) {
    return null;
  }
  try {
    const value = definition.selector(point);
    return value !== null && Number.isFinite(value) ? value : null;
  } catch {
    return null;
  }
}

interface StatsAccumulator {
  sum: number;
  count: number;
  maximum: number | null;
  current: number | null;
}

interface BucketAccumulator {
  first: number | null;
  last: number | null;
  minimumIndex: number | null;
  minimumValue: number;
  maximumIndex: number | null;
  maximumValue: number;
}

function newBucketAccumulator(): BucketAccumulator {
  return {
    first: null,
    last: null,
    minimumIndex: null,
    minimumValue: Number.POSITIVE_INFINITY,
    maximumIndex: null,
    maximumValue: Number.NEGATIVE_INFINITY,
  };
}

function addBucketCandidates(
  candidates: Set<number>,
  bucket: BucketAccumulator,
) {
  for (const index of [
    bucket.first,
    bucket.last,
    bucket.minimumIndex,
    bucket.maximumIndex,
  ]) {
    if (index !== null) {
      candidates.add(index);
    }
  }
}

/**
 * Downsampled buckets retain first/last finite values plus min/max within the
 * width-derived finite-sample budget. Every distinct null run retains its
 * first null, while adjacent finite runs retain their first and last samples.
 * With P=`chartPointBudget(width)`, G=gap-run count, R=finite-run count, and
 * B=floor((P-2)/4), the per-series bound is 4B+2+G+2R <= P+G+2R. Exact
 * arbitrary transitions therefore cannot honestly have a strict P-point
 * bound (and R <= G+1 gives the transition-only bound P+3G+2).
 */
export function prepareChartSeries(
  history: HistoryResponse,
  definitions: readonly MetricSeriesDefinition[],
  width: number,
): PreparedChartSeries {
  const pointBudget = chartPointBudget(width);
  const aligned = getAlignedHistoryIndex(history);
  if (!aligned) {
    return { valid: false, minuteCount: 0, pointBudget, series: [] };
  }

  const { minuteCount, records, fromUnix } = aligned;
  const candidates = definitions.map(() => new Set<number>([0, minuteCount - 1]));
  const stats: StatsAccumulator[] = definitions.map(() => ({
    sum: 0,
    count: 0,
    maximum: null,
    current: null,
  }));
  const gapsOpen = definitions.map(() => false);
  const lastFiniteIndices = definitions.map<number | null>(() => null);

  if (minuteCount <= pointBudget) {
    for (const selected of candidates) {
      for (let index = 0; index < minuteCount; index += 1) {
        selected.add(index);
      }
    }
  }

  const bucketCount =
    minuteCount <= pointBudget
      ? 1
      : Math.max(
          1,
          Math.floor((pointBudget - 2) / FINITE_CANDIDATES_PER_BUCKET),
        );
  const bucketSize = Math.ceil(minuteCount / bucketCount);

  for (let bucketIndex = 0; bucketIndex < bucketCount; bucketIndex += 1) {
    const start = bucketIndex * bucketSize;
    const end = Math.min(minuteCount, start + bucketSize);
    const buckets = definitions.map(() => newBucketAccumulator());

    for (let index = start; index < end; index += 1) {
      const point = records[index] ?? null;
      for (let seriesIndex = 0; seriesIndex < definitions.length; seriesIndex += 1) {
        const definition = definitions[seriesIndex]!;
        const value = selectedValue(definition, point);
        const aggregate = stats[seriesIndex]!;
        const bucket = buckets[seriesIndex]!;
        if (value === null) {
          if (!gapsOpen[seriesIndex]) {
            const lastFiniteIndex = lastFiniteIndices[seriesIndex];
            if (lastFiniteIndex !== null) {
              candidates[seriesIndex]!.add(lastFiniteIndex);
            }
            candidates[seriesIndex]!.add(index);
            gapsOpen[seriesIndex] = true;
          }
          continue;
        }
        if (gapsOpen[seriesIndex]) {
          candidates[seriesIndex]!.add(index);
        }
        gapsOpen[seriesIndex] = false;
        lastFiniteIndices[seriesIndex] = index;
        aggregate.current = value;

        aggregate.sum += value;
        aggregate.count += 1;
        aggregate.maximum =
          aggregate.maximum === null
            ? value
            : Math.max(aggregate.maximum, value);
        bucket.first ??= index;
        bucket.last = index;
        if (value < bucket.minimumValue) {
          bucket.minimumValue = value;
          bucket.minimumIndex = index;
        }
        if (value > bucket.maximumValue) {
          bucket.maximumValue = value;
          bucket.maximumIndex = index;
        }
      }
    }

    if (minuteCount > pointBudget) {
      for (let seriesIndex = 0; seriesIndex < definitions.length; seriesIndex += 1) {
        addBucketCandidates(candidates[seriesIndex]!, buckets[seriesIndex]!);
      }
    }
  }

  const prepared = definitions.map((definition, seriesIndex) => {
    const indices = [...candidates[seriesIndex]!].sort((left, right) => left - right);
    const data = indices.map<MetricChartPoint>((index) => [
      (fromUnix + index * 60) * 1_000,
      selectedValue(definition, records[index] ?? null),
    ]);
    const aggregate = stats[seriesIndex]!;
    return {
      ...definition,
      data,
      stats: {
        current: aggregate.current,
        average:
          aggregate.count === 0 ? null : aggregate.sum / aggregate.count,
        maximum: aggregate.maximum,
      },
    };
  });

  return {
    valid: true,
    minuteCount,
    pointBudget,
    series: prepared,
  };
}
