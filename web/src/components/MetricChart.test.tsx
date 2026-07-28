import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

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

  emit() {
    this.callback([], this as unknown as ResizeObserver);
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

const average: MetricChartSeries = {
  name: "平均",
  role: "primary",
  data: [
    [600_000, 10],
    [660_000, null],
    [720_000, 30],
  ],
};

const maximum: MetricChartSeries = {
  name: "峰值",
  role: "maximum",
  data: [
    [600_000, 20],
    [660_000, null],
    [720_000, 50],
  ],
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
        series={[{ ...average, data: [...average.data, [780_000, 40]] }, maximum]}
        formatter={metricChartFormatters.percent}
      />,
    );

    expect(init).toHaveBeenCalledTimes(1);
    expect(chart.setOption).toHaveBeenCalledTimes(2);
  });

  it("provides an adjacent accessible current, average, and maximum summary", () => {
    render(
      <MetricChart
        title="CPU 使用率"
        series={[average, maximum]}
        formatter={metricChartFormatters.percent}
      />,
    );

    const summary = screen.getByRole("group", { name: "CPU 使用率摘要" });
    expect(summary).toHaveTextContent("当前30.0%");
    expect(summary).toHaveTextContent("平均20.0%");
    expect(summary).toHaveTextContent("最大50.0%");
    expect(
      screen.getByRole("img", { name: "CPU 使用率趋势图" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("group", { name: "CPU 使用率各序列当前值" }),
    ).toHaveTextContent("平均30.0%");
  });

  it("shows a missing current value for a trailing gap and exposes every series in text", () => {
    render(
      <MetricChart
        title="网络速率"
        series={[
          {
            name: "上传",
            role: "primary",
            data: [
              [600_000, 1_024],
              [660_000, null],
            ],
          },
          {
            name: "下载",
            role: "context",
            data: [
              [600_000, 2_048],
              [660_000, 4_096],
            ],
          },
        ]}
        formatter={metricChartFormatters.rate}
      />,
    );

    expect(
      screen.getByRole("group", { name: "网络速率摘要" }),
    ).toHaveTextContent("当前—");
    const seriesValues = screen.getByRole("group", {
      name: "网络速率各序列当前值",
    });
    expect(seriesValues).toHaveTextContent("上传—");
    expect(seriesValues).toHaveTextContent("下载4.00 KiB/s");
  });

  it("disables chart animation when reduced motion is preferred", () => {
    vi.stubGlobal("matchMedia", matchMedia(true));

    render(
      <MetricChart
        title="CPU 使用率"
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
        series={[average]}
        formatter={metricChartFormatters.percent}
      />,
    );

    expect(TestResizeObserver.instance?.observe).toHaveBeenCalledTimes(1);
    act(() => TestResizeObserver.instance?.emit());
    expect(chart.resize).toHaveBeenCalledTimes(1);

    unmount();
    expect(TestResizeObserver.instance?.disconnect).toHaveBeenCalledTimes(1);
    expect(chart.dispose).toHaveBeenCalledTimes(1);
  });

  it("falls back to window resize when ResizeObserver is unavailable", () => {
    vi.stubGlobal("ResizeObserver", undefined);
    const { unmount } = render(
      <MetricChart
        title="CPU 使用率"
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
