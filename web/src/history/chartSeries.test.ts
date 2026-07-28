import { describe, expect, it } from "vitest";

import type { HistoryResponse, MinutePayload, MinuteRecord } from "./types";
import {
  chartPointBudget,
  getAlignedHistoryIndex,
  prepareChartSeries,
  type MetricSeriesDefinition,
} from "./chartSeries";

function record(
  minuteUnix: number,
  average: number,
  maximum: number,
  load5: number,
): MinuteRecord {
  return {
    serverId: 7,
    minuteUnix,
    payload: {
      cpuUsage: { average, maximum },
      load5: { average: load5, maximum: load5 + 1 },
    } as MinutePayload,
  };
}

describe("prepareChartSeries", () => {
  it("preserves every separated gap run inside one downsampling bucket", () => {
    const minuteCount = 1_000;
    const gapIndices = new Set([5, 6, 10, 11]);
    const points: MinuteRecord[] = [];
    for (let index = 0; index < minuteCount; index += 1) {
      if (gapIndices.has(index)) {
        continue;
      }
      points.push(record(index * 60, index, index + 10, index));
    }
    const definitions: MetricSeriesDefinition[] = [
      {
        name: "CPU 平均",
        role: "primary",
        pairOrdinal: 0,
        selector: (point) => point.payload.cpuUsage.average,
      },
      {
        name: "Load 5",
        role: "context",
        pairOrdinal: 1,
        selector: (point) => point.payload.load5.average,
      },
    ];

    const prepared = prepareChartSeries(
      {
        fromUnix: 0,
        toUnix: (minuteCount - 1) * 60,
        points,
      },
      definitions,
      80,
    );

    expect(prepared.minuteCount).toBeGreaterThan(prepared.pointBudget);
    for (const series of prepared.series) {
      expect(
        series.data
          .filter(([, value]) => value === null)
          .map(([timestamp]) => timestamp),
      ).toEqual([5 * 60_000, 10 * 60_000]);
      expect(series.data).toContainEqual([0, 0]);
      expect(series.data).toContainEqual([7 * 60_000, 7]);
      expect(series.data).toContainEqual([12 * 60_000, 12]);
      expect(series.data).toContainEqual([
        (minuteCount - 1) * 60_000,
        minuteCount - 1,
      ]);
      const timestamps = series.data.map(([timestamp]) => timestamp);
      expect(timestamps.indexOf(5 * 60_000)).toBeLessThan(
        timestamps.indexOf(7 * 60_000),
      );
      expect(timestamps.indexOf(7 * 60_000)).toBeLessThan(
        timestamps.indexOf(10 * 60_000),
      );
    }
  });

  it("adds exact gap breaks to the finite sampling budget on pathological history", () => {
    const fromUnix = 0;
    const toUnix = 30 * 24 * 60 * 60;
    const minuteCount = toUnix / 60 + 1;
    const alternatingGapStart = 10_000;
    const alternatingGapMinutes = 240;
    const gapRunCount = alternatingGapMinutes / 2;
    const points: MinuteRecord[] = [];
    for (let index = 0; index < minuteCount; index += 1) {
      const inAlternatingGaps =
        index >= alternatingGapStart &&
        index < alternatingGapStart + alternatingGapMinutes &&
        (index - alternatingGapStart) % 2 === 1;
      if (!inAlternatingGaps) {
        points.push(record(index * 60, index % 100, index % 100, index % 7));
      }
    }

    const prepared = prepareChartSeries(
      { fromUnix, toUnix, points },
      [
        {
          name: "CPU 平均",
          role: "primary",
          pairOrdinal: 0,
          selector: (point) => point.payload.cpuUsage.average,
        },
      ],
      800,
    );
    const data = prepared.series[0]!.data;
    const timestamps = new Set(data.map(([timestamp]) => timestamp));

    expect(data.filter(([, value]) => value === null)).toHaveLength(gapRunCount);
    for (let offset = 1; offset < alternatingGapMinutes; offset += 2) {
      expect(timestamps).toContain((alternatingGapStart + offset) * 60_000);
      expect(timestamps).toContain((alternatingGapStart + offset + 1) * 60_000);
    }
    expect(data.length).toBeLessThanOrEqual(
      prepared.pointBudget + gapRunCount * 2,
    );
    expect(data.length).toBeLessThan(3_000);
    expect(data.length).toBeLessThan(minuteCount / 10);
  });

  it("bounds a full 30-day chart while preserving peaks, gaps, endpoints, order, and stats", () => {
    const fromUnix = 0;
    const toUnix = 30 * 24 * 60 * 60;
    const minuteCount = toUnix / 60 + 1;
    const gapStart = 20_000;
    const gapEnd = 20_004;
    const peakIndex = 30_000;
    const points: MinuteRecord[] = [];
    for (let index = 0; index < minuteCount; index += 1) {
      if (index >= gapStart && index <= gapEnd) {
        continue;
      }
      const average = index % 100;
      points.push(
        record(
          index * 60,
          average,
          index === peakIndex ? 9_999 : average + 10,
          200 + (index % 7),
        ),
      );
    }
    const history: HistoryResponse = { fromUnix, toUnix, points };
    let selectorCalls = 0;
    const definitions: MetricSeriesDefinition[] = [
      {
        name: "CPU 平均",
        role: "primary",
        pairOrdinal: 0,
        selector: (point) => {
          selectorCalls += 1;
          return point.payload.cpuUsage.average;
        },
      },
      {
        name: "CPU 峰值",
        role: "maximum",
        pairOrdinal: 0,
        selector: (point) => {
          selectorCalls += 1;
          return point.payload.cpuUsage.maximum;
        },
      },
      {
        name: "Load 5",
        role: "context",
        pairOrdinal: 1,
        selector: (point) => {
          selectorCalls += 1;
          return point.payload.load5.average;
        },
      },
    ];

    const prepared = prepareChartSeries(history, definitions, 800);
    const budget = chartPointBudget(800);

    expect(prepared.valid).toBe(true);
    expect(prepared.minuteCount).toBe(43_201);
    expect(prepared.pointBudget).toBe(budget);
    expect(getAlignedHistoryIndex(history)).toBe(getAlignedHistoryIndex(history));
    for (const series of prepared.series) {
      expect(series.data.length).toBeLessThanOrEqual(budget);
      expect(series.data[0]?.[0]).toBe(fromUnix * 1_000);
      expect(series.data.at(-1)?.[0]).toBe(toUnix * 1_000);
      expect(
        series.data.every(
          (point, index) => index === 0 || point[0] > series.data[index - 1]![0],
        ),
      ).toBe(true);
      expect(
        series.data.some(
          ([timestamp, value]) =>
            timestamp >= gapStart * 60_000 &&
            timestamp <= gapEnd * 60_000 &&
            value === null,
        ),
      ).toBe(true);
    }
    expect(prepared.series[1]?.data).toContainEqual([
      peakIndex * 60_000,
      9_999,
    ]);
    expect(prepared.series[1]?.stats.maximum).toBe(9_999);
    expect(prepared.series[2]?.stats.current).toBe(203);
    expect(prepared.series[2]?.stats.average).toBeGreaterThanOrEqual(200);
    expect(prepared.series[2]?.stats.maximum).toBe(206);
    expect(selectorCalls).toBeLessThanOrEqual(
      minuteCount * definitions.length + budget * definitions.length,
    );
  });

  it("returns bounded empty output for invalid ranges and ignores malformed points and values", () => {
    const invalid = prepareChartSeries(
      { fromUnix: 0, toUnix: 30 * 24 * 60 * 60 + 60, points: [] },
      [
        {
          name: "CPU",
          role: "primary",
          pairOrdinal: 0,
          selector: (point) => point.payload.cpuUsage.average,
        },
      ],
      Number.MAX_SAFE_INTEGER,
    );
    expect(invalid).toMatchObject({ valid: false, minuteCount: 0, series: [] });

    const history: HistoryResponse = {
      fromUnix: 600,
      toUnix: 720,
      points: [record(600, 10, 15, 1), record(601, 99, 99, 99), record(720, 30, 35, 3)],
    };
    const prepared = prepareChartSeries(
      history,
      [
        {
          name: "CPU",
          role: "primary",
          pairOrdinal: 0,
          selector: (point) =>
            point.minuteUnix === 720 ? Number.POSITIVE_INFINITY : point.payload.cpuUsage.average,
        },
      ],
      400,
    );

    expect(prepared.series[0]?.data).toEqual([
      [600_000, 10],
      [660_000, null],
      [720_000, null],
    ]);
    expect(prepared.series[0]?.stats).toEqual({
      current: 10,
      average: 10,
      maximum: 10,
    });
  });
});
