import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { useEffect, useId, useRef, useState } from "react";

import { formatBytes, formatRate } from "../utils/format";

export type MetricChartPoint = [timestampMs: number, value: number | null];

export interface MetricChartSeries {
  name: string;
  data: MetricChartPoint[];
  role?: "primary" | "maximum" | "context" | "context-maximum";
}

interface MetricChartProps {
  title: string;
  series: MetricChartSeries[];
  formatter: (value: number) => string;
}

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";
const PRIMARY_COLOR = "#8a3158";
const MAXIMUM_COLOR = "#c07b94";
const CONTEXT_COLORS = ["#586973", "#6b7880"];
const CONTEXT_MAXIMUM_COLOR = "#87939a";

use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

export const metricChartFormatters = {
  percent: (value: number) => `${value.toFixed(1)}%`,
  rate: (value: number) => formatRate(value),
  bytes: (value: number) => formatBytes(value),
  load: (value: number) => value.toFixed(2),
};

function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() =>
    typeof window.matchMedia === "function"
      ? window.matchMedia(REDUCED_MOTION_QUERY).matches
      : false,
  );

  useEffect(() => {
    if (typeof window.matchMedia !== "function") {
      return;
    }
    const media = window.matchMedia(REDUCED_MOTION_QUERY);
    const handleChange = (event: MediaQueryListEvent) => setReduced(event.matches);
    media.addEventListener?.("change", handleChange);
    return () => media.removeEventListener?.("change", handleChange);
  }, []);

  return reduced;
}

function values(series: MetricChartSeries | undefined): number[] {
  if (!series) {
    return [];
  }
  return series.data.flatMap(([, value]) =>
    value !== null && Number.isFinite(value) ? [value] : [],
  );
}

function current(series: MetricChartSeries | undefined): number | null {
  const value = series?.data.at(-1)?.[1];
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return null;
  }
  return value;
}

function display(value: number | null, formatter: (value: number) => string) {
  return value === null ? "—" : formatter(value);
}

export function MetricChart({ title, series, formatter }: MetricChartProps) {
  const headingId = useId();
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<EChartsType | null>(null);
  const reducedMotion = useReducedMotion();

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }

    const chart = init(container);
    chartRef.current = chart;
    const resize = () => chart.resize();
    let observer: ResizeObserver | null = null;
    if (typeof ResizeObserver === "function") {
      observer = new ResizeObserver(resize);
      observer.observe(container);
    } else {
      window.addEventListener("resize", resize);
    }

    return () => {
      observer?.disconnect();
      if (!observer) {
        window.removeEventListener("resize", resize);
      }
      chart.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    chartRef.current?.setOption(
      {
        animation: !reducedMotion,
        animationDuration: reducedMotion ? 0 : 260,
        color: [
          PRIMARY_COLOR,
          MAXIMUM_COLOR,
          ...CONTEXT_COLORS,
          CONTEXT_MAXIMUM_COLOR,
        ],
        grid: { top: 42, right: 18, bottom: 34, left: 54 },
        legend: {
          top: 0,
          left: 0,
          itemWidth: 18,
          itemHeight: 3,
          textStyle: { color: "#6c6268", fontSize: 12 },
        },
        tooltip: {
          trigger: "axis",
          valueFormatter: formatter,
        },
        xAxis: {
          type: "time",
          boundaryGap: false,
          axisLine: { lineStyle: { color: "#ded8dc" } },
          axisLabel: { color: "#746a70", hideOverlap: true },
          splitLine: { show: false },
        },
        yAxis: {
          type: "value",
          axisLabel: { color: "#746a70", formatter },
          splitLine: { lineStyle: { color: "#eee9ec" } },
        },
        series: series.map((line, index) => {
          const color =
            line.role === "maximum"
              ? MAXIMUM_COLOR
              : line.role === "context-maximum"
                ? CONTEXT_MAXIMUM_COLOR
                : line.role === "primary" || index === 0
                  ? PRIMARY_COLOR
                  : CONTEXT_COLORS[(index - 1) % CONTEXT_COLORS.length];
          const isMaximum =
            line.role === "maximum" || line.role === "context-maximum";
          return {
            name: line.name,
            type: "line",
            data: line.data,
            connectNulls: false,
            showSymbol: false,
            symbol: "circle",
            itemStyle: { color },
            lineStyle: {
              color,
              type: isMaximum ? "dashed" : "solid",
              width: line.role === "primary" || index === 0 ? 2.25 : 1.5,
              opacity: 1,
            },
            emphasis: { focus: "series" },
          };
        }),
      },
      { notMerge: true, lazyUpdate: true },
    );
  }, [formatter, reducedMotion, series]);

  const primary = series.find((line) => line.role === "primary") ?? series[0];
  const maximumSeries = series.find((line) => line.role === "maximum");
  const primaryValues = values(primary);
  const maximumValues = values(maximumSeries ?? primary);
  const average =
    primaryValues.length === 0
      ? null
      : primaryValues.reduce((sum, value) => sum + value, 0) /
        primaryValues.length;
  const maximum =
    maximumValues.length === 0 ? null : Math.max(...maximumValues);

  return (
    <section className="metric-chart" aria-labelledby={headingId}>
      <div className="metric-chart-heading">
        <h4 id={headingId}>{title}</h4>
        <dl className="metric-chart-summary" role="group" aria-label={`${title}摘要`}>
          <div>
            <dt>当前</dt>
            <dd>{display(current(primary), formatter)}</dd>
          </div>
          <div>
            <dt>平均</dt>
            <dd>{display(average, formatter)}</dd>
          </div>
          <div>
            <dt>最大</dt>
            <dd>{display(maximum, formatter)}</dd>
          </div>
        </dl>
      </div>
      <dl
        className="metric-chart-series-values"
        role="group"
        aria-label={`${title}各序列当前值`}
      >
        {series.map((line, index) => (
          <div key={`${line.name}-${index}`}>
            <dt>{line.name}</dt>
            <dd>{display(current(line), formatter)}</dd>
          </div>
        ))}
      </dl>
      <div
        ref={containerRef}
        className="metric-chart-canvas"
        role="img"
        aria-label={`${title}趋势图`}
      />
    </section>
  );
}
