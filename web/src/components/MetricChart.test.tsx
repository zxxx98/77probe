import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { HistoryResponse, MinutePayload, MinuteRecord } from "../history/types";

const chart = vi.hoisted(() => ({
  setOption: vi.fn(),
  resize: vi.fn(),
  dispose: vi.fn(),
}));
const init = vi.hoisted(() => vi.fn(() => chart));

vi.mock("echarts/charts", () => ({ LineChart: {} }));
vi.mock("echarts/components", () => ({
  GridComponent: {},
  LegendComponent: {},
  TooltipComponent: {},
}));
vi.mock("echarts/core", () => ({ init, use: vi.fn() }));
vi.mock("echarts/renderers", () => ({ CanvasRenderer: {} }));

import {
  MetricChart,
  metricChartFormatters,
  type MetricChartSeries,
} from "./MetricChart";

class TestResizeObserver {
  static instance: TestResizeObserver | null = null;
  observe = vi.fn();
  disconnect = vi.fn();

  constructor(private readonly callback: ResizeObserverCallback) {
    TestResizeObserver.instance = this;
  }

  emit(width = 800) {
    this.callback(
      [{ contentRect: { width } } as ResizeObserverEntry],
      this as unknown as ResizeObserver,
    );
  }
}

function matchMedia(matches: boolean): typeof window.matchMedia {
  return vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

function relativeLuminance(hex: string): number {
  const channels = hex
    .match(/[\da-f]{2}/gi)
    ?.map((channel) => Number.parseInt(channel, 16) / 255)
    .map((channel) =>
      channel <= 0.04045
        ? channel / 12.92
        : ((channel + 0.055) / 1.055) ** 2.4,
    );
  if (!channels || channels.length !== 3) {
    throw new Error(`invalid color: ${hex}`);
  }
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function contrastOnWhite(hex: string): number {
  return 1.05 / (relativeLuminance(hex) + 0.05);
}

function minute(
  minuteUnix: number,
  cpuAverage: number,
  cpuMaximum: number,
  upload: number,
  download: number,
): MinuteRecord {
  return {
    serverId: 7,
    minuteUnix,
    payload: {
      cpuUsage: { average: cpuAverage, maximum: cpuMaximum },
      uploadBps: { average: upload, maximum: upload + 1_024 },
      downloadBps: { average: download, maximum: download + 1_024 },
      load1: { average: 1, maximum: 2 },
      load5: { average: 5, maximum: 6 },
      load15: { average: 15, maximum: 16 },
    } as unknown as MinutePayload,
  };
}

const history: HistoryResponse = {
  fromUnix: 600,
  toUnix: 720,
  points: [minute(600, 10, 20, 1_024, 2_048), minute(720, 30, 50, 3_072, 4_096)],
};

function downsampledHistory(gaps: ReadonlySet<number>): HistoryResponse {
  const minuteCount = 1_000;
  const points: MinuteRecord[] = [];
  for (let index = 0; index < minuteCount; index += 1) {
    if (!gaps.has(index)) {
      points.push(minute(index * 60, index, index + 10, index, index));
    }
  }
  return {
    fromUnix: 0,
    toUnix: (minuteCount - 1) * 60,
    points,
  };
}

const average: MetricChartSeries = {
  name: "平均",
  role: "primary",
  pairOrdinal: 0,
  selector: (point) => point.payload.cpuUsage.average,
};

const maximum: MetricChartSeries = {
  name: "峰值",
  role: "maximum",
  pairOrdinal: 0,
  selector: (point) => point.payload.cpuUsage.maximum,
};

beforeEach(() => {
  init.mockClear();
  chart.setOption.mockClear();
  chart.resize.mockClear();
  chart.dispose.mockClear();
  TestResizeObserver.instance = null;
  vi.stubGlobal("ResizeObserver", TestResizeObserver);
  vi.stubGlobal("matchMedia", matchMedia(false));
});

describe("MetricChart", () => {
  it("initializes once and updates gap-preserving average and maximum lines", () => {
    const { rerender } = render(
      <MetricChart
        title="CPU 使用率"
        history={history}
        series={[average, maximum]}
        formatter={metricChartFormatters.percent}
      />,
    );

    expect(init).toHaveBeenCalledTimes(1);
    expect(chart.setOption).toHaveBeenCalledTimes(1);
    const option = chart.setOption.mock.calls[0]?.[0] as {
      animation: boolean;
      series: Array<{
        connectNulls: boolean;
        lineStyle: { type: string; opacity: number; color: string };
      }>;
    };
    expect(option.animation).toBe(true);
    expect(option.series).toHaveLength(2);
    expect(option.series.every((line) => line.connectNulls === false)).toBe(true);
    expect(option.series[0]?.lineStyle.type).toBe("solid");
    expect(option.series[0]?.lineStyle.opacity).toBe(1);
    expect(option.series[0]?.lineStyle.color).toBe("#8a3158");
    expect(option.series[1]?.lineStyle.opacity).toBe(1);
    expect(option.series[1]?.lineStyle.color).not.toBe("#8a3158");
    expect(contrastOnWhite(option.series[1]?.lineStyle.color)).toBeGreaterThanOrEqual(3);

    rerender(
      <MetricChart
        title="CPU 使用率"
        history={{
          fromUnix: 600,
          toUnix: 780,
          points: [...history.points, minute(780, 40, 60, 4_096, 5_120)],
        }}
        series={[average, maximum]}
        formatter={metricChartFormatters.percent}
      />,
    );

    expect(init).toHaveBeenCalledTimes(1);
    expect(chart.setOption).toHaveBeenCalledTimes(2);
  });

  it("keeps a three-minute finite run drawable between downsampled gaps", () => {
    render(
      <MetricChart
        title="CPU 使用率"
        history={downsampledHistory(new Set([5, 6, 10, 11]))}
        series={[average]}
        formatter={metricChartFormatters.percent}
      />,
    );
    act(() => TestResizeObserver.instance?.emit(80));

    const option = chart.setOption.mock.calls.at(-1)?.[0] as {
      series: Array<{
        connectNulls: boolean;
        data: Array<[number, number | null]>;
      }>;
    };
    const line = option.series[0]!;
    expect(line.connectNulls).toBe(false);
    expect(
      line.data.filter(
        ([timestamp]) => timestamp >= 7 * 60_000 && timestamp <= 9 * 60_000,
      ),
    ).toEqual([
      [7 * 60_000, 7],
      [9 * 60_000, 9],
    ]);
  });

  it("shows an isolated finite minute without adding symbols to normal points", () => {
    render(
      <MetricChart
        title="CPU 使用率"
        history={downsampledHistory(new Set([5, 6, 8, 9]))}
        series={[average]}
        formatter={metricChartFormatters.percent}
      />,
    );
    act(() => TestResizeObserver.instance?.emit(80));

    const option = chart.setOption.mock.calls.at(-1)?.[0] as {
      series: Array<{
        data: Array<[number, number | null]>;
        showSymbol: boolean;
        symbolSize: (
          value: [number, number | null],
          params: { dataIndex: number },
        ) => number;
      }>;
    };
    const line = option.series[0]!;
    const isolatedIndex = line.data.findIndex(
      ([timestamp]) => timestamp === 7 * 60_000,
    );
    const connectedIndex = line.data.findIndex(
      ([timestamp]) => timestamp === 4 * 60_000,
    );
    const gapIndex = line.data.findIndex(
      ([timestamp]) => timestamp === 5 * 60_000,
    );

    expect(line.showSymbol).toBe(true);
    expect(line.symbolSize(line.data[isolatedIndex]!, { dataIndex: isolatedIndex }))
      .toBeGreaterThan(0);
    expect(line.symbolSize(line.data[connectedIndex]!, { dataIndex: connectedIndex }))
      .toBe(0);
    expect(line.symbolSize(line.data[gapIndex]!, { dataIndex: gapIndex })).toBe(0);
  });

  it("provides an adjacent accessible recent, average, and maximum summary", () => {
    render(
      <MetricChart
        title="CPU 使用率"
        history={history}
        series={[average, maximum]}
        formatter={metricChartFormatters.percent}
      />,
    );

    const summary = screen.getByRole("group", { name: "CPU 使用率摘要" });
    expect(summary).toHaveTextContent("最近值30.0%");
    expect(summary).toHaveTextContent("平均20.0%");
    expect(summary).toHaveTextContent("最大50.0%");
    expect(
      screen.getByRole("img", { name: "CPU 使用率趋势图" }),
    ).toBeInTheDocument();
    const stats = screen.getByRole("table", { name: "CPU 使用率各序列统计" });
    expect(stats).toHaveTextContent("序列最近值平均最大");
    expect(screen.getByRole("row", { name: "平均 30.0% 20.0% 30.0%" }))
      .toBeInTheDocument();
    expect(screen.getByRole("row", { name: "峰值 50.0% 35.0% 50.0%" }))
      .toBeInTheDocument();
  });

  it("uses each series recent finite value without filling a trailing chart gap", () => {
    render(
      <MetricChart
        title="网络速率"
        history={{
          fromUnix: 600,
          toUnix: 660,
          points: [
            minute(600, 0, 0, 1_024, 2_048),
            minute(660, 0, 0, Number.NaN, 4_096),
          ],
        }}
        series={[
          {
            name: "上传",
            role: "primary",
            pairOrdinal: 0,
            selector: (point) => point.payload.uploadBps.average,
          },
          {
            name: "下载",
            role: "context",
            pairOrdinal: 1,
            selector: (point) => point.payload.downloadBps.average,
          },
        ]}
        formatter={metricChartFormatters.rate}
      />,
    );

    const summary = screen.getByRole("group", { name: "网络速率摘要" });
    expect(summary).toHaveTextContent("最近值1.00 KiB/s");
    expect(summary).not.toHaveTextContent("当前");
    expect(screen.getByRole("row", {
      name: "上传 1.00 KiB/s 1.00 KiB/s 1.00 KiB/s",
    })).toBeInTheDocument();
    expect(
      screen.getByRole("row", {
        name: "下载 4.00 KiB/s 3.00 KiB/s 4.00 KiB/s",
      }),
    ).toBeInTheDocument();
    const option = chart.setOption.mock.calls[0]?.[0] as {
      series: Array<{ data: Array<[number, number | null]> }>;
    };
    expect(option.series[0]?.data.at(-1)).toEqual([660_000, null]);
    expect(option.series[1]?.data.at(-1)).toEqual([660_000, 4_096]);
  });

  it("styles all six load lines by explicit logical pair identity", () => {
    const loadSeries: MetricChartSeries[] = [
      { name: "Load 1", role: "primary", pairOrdinal: 0, selector: (p) => p.payload.load1.average },
      { name: "Load 1 峰值", role: "maximum", pairOrdinal: 0, selector: (p) => p.payload.load1.maximum },
      { name: "Load 5", role: "context", pairOrdinal: 1, selector: (p) => p.payload.load5.average },
      { name: "Load 5 峰值", role: "context-maximum", pairOrdinal: 1, selector: (p) => p.payload.load5.maximum },
      { name: "Load 15", role: "context", pairOrdinal: 2, selector: (p) => p.payload.load15.average },
      { name: "Load 15 峰值", role: "context-maximum", pairOrdinal: 2, selector: (p) => p.payload.load15.maximum },
    ];
    render(
      <MetricChart
        title="系统负载"
        history={history}
        series={loadSeries}
        formatter={metricChartFormatters.load}
      />,
    );

    const option = chart.setOption.mock.calls[0]?.[0] as {
      series: Array<{
        name: string;
        lineStyle: { color: string; type: string };
      }>;
    };
    const identities = Object.fromEntries(
      option.series.map((line) => [line.name, line.lineStyle]),
    );
    expect(identities).toEqual({
      "Load 1": expect.objectContaining({ color: "#8a3158", type: "solid" }),
      "Load 1 峰值": expect.objectContaining({ color: "#c07b94", type: "dashed" }),
      "Load 5": expect.objectContaining({ color: "#486a78", type: "solid" }),
      "Load 5 峰值": expect.objectContaining({ color: "#6f8f9b", type: "dashed" }),
      "Load 15": expect.objectContaining({ color: "#6d5c7d", type: "solid" }),
      "Load 15 峰值": expect.objectContaining({ color: "#89769a", type: "dashed" }),
    });
    expect(
      option.series.every((line) => contrastOnWhite(line.lineStyle.color) >= 3),
    ).toBe(true);
  });

  it("disables chart animation when reduced motion is preferred", () => {
    vi.stubGlobal("matchMedia", matchMedia(true));

    render(
      <MetricChart
        title="CPU 使用率"
        history={history}
        series={[average]}
        formatter={metricChartFormatters.percent}
      />,
    );

    expect(chart.setOption).toHaveBeenCalledWith(
      expect.objectContaining({ animation: false, animationDuration: 0 }),
      expect.anything(),
    );
  });

  it("resizes through ResizeObserver and disposes on unmount", () => {
    const { unmount } = render(
      <MetricChart
        title="CPU 使用率"
        history={history}
        series={[average]}
        formatter={metricChartFormatters.percent}
      />,
    );

    expect(TestResizeObserver.instance?.observe).toHaveBeenCalledTimes(1);
    act(() => TestResizeObserver.instance?.emit(390));
    expect(chart.resize).toHaveBeenCalledTimes(1);
    expect(chart.setOption).toHaveBeenCalledTimes(2);

    unmount();
    expect(TestResizeObserver.instance?.disconnect).toHaveBeenCalledTimes(1);
    expect(chart.dispose).toHaveBeenCalledTimes(1);
  });

  it("falls back to window resize when ResizeObserver is unavailable", () => {
    vi.stubGlobal("ResizeObserver", undefined);
    const { unmount } = render(
      <MetricChart
        title="CPU 使用率"
        history={history}
        series={[average]}
        formatter={metricChartFormatters.percent}
      />,
    );

    act(() => window.dispatchEvent(new Event("resize")));
    expect(chart.resize).toHaveBeenCalledTimes(1);
    unmount();
  });

  it("formats percentages, rates, bytes, and load values", () => {
    expect(metricChartFormatters.percent(12.34)).toBe("12.3%");
    expect(metricChartFormatters.rate(1_024)).toBe("1.00 KiB/s");
    expect(metricChartFormatters.bytes(1_024)).toBe("1.00 KiB");
    expect(metricChartFormatters.load(1.234)).toBe("1.23");
  });
});
