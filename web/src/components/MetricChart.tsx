import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { init, use, type EChartsType } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import { useEffect, useId, useMemo, useRef, useState } from "react";

import {
  prepareChartSeries,
  type MetricSeriesDefinition,
  type PreparedMetricSeries,
} from "../history/chartSeries";
import type { HistoryResponse } from "../history/types";
import { formatBytes, formatRate } from "../utils/format";

export type MetricChartSeries = MetricSeriesDefinition;

interface MetricChartProps {
  title: string;
  history: HistoryResponse;
  series: MetricChartSeries[];
  formatter: (value: number) => string;
}

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";
const AVERAGE_COLORS = ["#8a3158", "#486a78", "#6d5c7d"];
const MAXIMUM_COLORS = ["#c07b94", "#6f8f9b", "#89769a"];

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

function display(value: number | null, formatter: (value: number) => string) {
  return value === null ? "—" : formatter(value);
}

function lineColor(series: PreparedMetricSeries): string {
  const palette =
    series.role === "maximum" || series.role === "context-maximum"
      ? MAXIMUM_COLORS
      : AVERAGE_COLORS;
  const ordinal =
    Number.isSafeInteger(series.pairOrdinal) && series.pairOrdinal >= 0
      ? series.pairOrdinal
      : 0;
  return palette[ordinal % palette.length]!;
}

export function MetricChart({
  title,
  history,
  series,
  formatter,
}: MetricChartProps) {
  const headingId = useId();
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<EChartsType | null>(null);
  const [chartWidth, setChartWidth] = useState(0);
  const reducedMotion = useReducedMotion();
  const prepared = useMemo(
    () => prepareChartSeries(history, series, chartWidth),
    [chartWidth, history, series],
  );

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }

    const chart = init(container);
    chartRef.current = chart;
    const resize = (width?: number) => {
      const measured =
        width && Number.isFinite(width) && width > 0
          ? width
          : container.getBoundingClientRect().width || container.clientWidth;
      if (measured > 0) {
        setChartWidth((current) =>
          Math.abs(current - measured) >= 1 ? measured : current,
        );
      }
      chart.resize();
    };
    const resizeFromWindow = () => resize();
    let observer: ResizeObserver | null = null;
    if (typeof ResizeObserver === "function") {
      observer = new ResizeObserver((entries) =>
        resize(entries[0]?.contentRect.width),
      );
      observer.observe(container);
    } else {
      window.addEventListener("resize", resizeFromWindow);
    }

    return () => {
      observer?.disconnect();
      if (!observer) {
        window.removeEventListener("resize", resizeFromWindow);
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
        series: prepared.series.map((line) => {
          const color = lineColor(line);
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
              width: line.role === "primary" ? 2.25 : 1.5,
              opacity: 1,
            },
            emphasis: { focus: "series" },
            progressive: 500,
            progressiveThreshold: 1_000,
          };
        }),
      },
      { notMerge: true, lazyUpdate: true },
    );
  }, [formatter, prepared.series, reducedMotion]);

  const primary =
    prepared.series.find((line) => line.role === "primary") ??
    prepared.series[0];
  const maximumSeries = prepared.series.find(
    (line) =>
      line.role === "maximum" && line.pairOrdinal === primary?.pairOrdinal,
  );

  return (
    <section className="metric-chart" aria-labelledby={headingId}>
      <div className="metric-chart-heading">
        <h4 id={headingId}>{title}</h4>
        <dl className="metric-chart-summary" role="group" aria-label={`${title}摘要`}>
          <div>
            <dt>最近值</dt>
            <dd>{display(primary?.stats.current ?? null, formatter)}</dd>
          </div>
          <div>
            <dt>平均</dt>
            <dd>{display(primary?.stats.average ?? null, formatter)}</dd>
          </div>
          <div>
            <dt>最大</dt>
            <dd>
              {display(
                maximumSeries?.stats.maximum ?? primary?.stats.maximum ?? null,
                formatter,
              )}
            </dd>
          </div>
        </dl>
      </div>
      <div className="metric-chart-series-table-wrap">
        <table
          className="metric-chart-series-table"
          aria-label={`${title}各序列统计`}
        >
          <thead>
            <tr>
              <th scope="col">序列</th>
              <th scope="col">最近值</th>
              <th scope="col">平均</th>
              <th scope="col">最大</th>
            </tr>
          </thead>
          <tbody>
            {prepared.series.map((line, index) => (
              <tr key={`${line.name}-${index}`}>
                <th scope="row">{line.name}</th>
                <td>{display(line.stats.current, formatter)}</td>
                <td>{display(line.stats.average, formatter)}</td>
                <td>{display(line.stats.maximum, formatter)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div
        ref={containerRef}
        className="metric-chart-canvas"
        role="img"
        aria-label={`${title}趋势图`}
      />
    </section>
  );
}
