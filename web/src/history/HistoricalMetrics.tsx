import {
  MetricChart,
  metricChartFormatters,
  type MetricChartSeries,
} from "../components/MetricChart";
import type { HistoryResponse, Pair } from "./types";
import { buildMinuteSeries } from "./useHistory";

function pairSeries(
  history: HistoryResponse,
  selector: (point: HistoryResponse["points"][number]) => Pair | null,
): MetricChartSeries[] {
  return [
    {
      name: "平均",
      role: "primary",
      data: buildMinuteSeries(
        history.points,
        history.fromUnix,
        history.toUnix,
        (point) => selector(point)?.average ?? null,
      ),
    },
    {
      name: "峰值",
      role: "maximum",
      data: buildMinuteSeries(
        history.points,
        history.fromUnix,
        history.toUnix,
        (point) => selector(point)?.maximum ?? null,
      ),
    },
  ];
}

function valueSeries(
  history: HistoryResponse,
  lines: Array<{
    name: string;
    role?: MetricChartSeries["role"];
    selector: (point: HistoryResponse["points"][number]) => number | null;
  }>,
): MetricChartSeries[] {
  return lines.map((line) => ({
    name: line.name,
    role: line.role,
    data: buildMinuteSeries(
      history.points,
      history.fromUnix,
      history.toUnix,
      line.selector,
    ),
  }));
}

function diskPair(
  point: HistoryResponse["points"][number],
  mountpoint: string,
): Pair | null {
  for (let index = point.payload.disks.length - 1; index >= 0; index -= 1) {
    const disk = point.payload.disks[index];
    if (disk?.mountpoint === mountpoint) {
      return disk.usage;
    }
  }
  return null;
}

export function HistoricalMetrics({ history }: { history: HistoryResponse }) {
  const mountpoints = Array.from(
    new Set(
      history.points.flatMap((point) =>
        point.payload.disks.map((disk) => disk.mountpoint),
      ),
    ),
  ).sort((left, right) => left.localeCompare(right));

  return (
    <div className="historical-metric-groups">
      <article className="historical-metric-group">
        <div className="historical-group-heading">
          <h3>处理器与负载</h3>
          <p>CPU 保留分钟平均与峰值，负载展示 1 / 5 / 15 分钟趋势。</p>
        </div>
        <MetricChart
          title="CPU 使用率"
          series={pairSeries(history, (point) => point.payload.cpuUsage)}
          formatter={metricChartFormatters.percent}
        />
        <MetricChart
          title="系统负载"
          series={valueSeries(history, [
            {
              name: "Load 1",
              role: "primary",
              selector: (point) => point.payload.load1.average,
            },
            {
              name: "Load 1 峰值",
              role: "maximum",
              selector: (point) => point.payload.load1.maximum,
            },
            {
              name: "Load 5",
              role: "context",
              selector: (point) => point.payload.load5.average,
            },
            {
              name: "Load 5 峰值",
              role: "context-maximum",
              selector: (point) => point.payload.load5.maximum,
            },
            {
              name: "Load 15",
              role: "context",
              selector: (point) => point.payload.load15.average,
            },
            {
              name: "Load 15 峰值",
              role: "context-maximum",
              selector: (point) => point.payload.load15.maximum,
            },
          ])}
          formatter={metricChartFormatters.load}
        />
      </article>

      <article className="historical-metric-group">
        <div className="historical-group-heading">
          <h3>内存与交换空间</h3>
          <p>使用率按分钟聚合；未配置 Swap 时以零值安全展示。</p>
        </div>
        <MetricChart
          title="内存使用率"
          series={pairSeries(history, (point) => point.payload.memoryUsage)}
          formatter={metricChartFormatters.percent}
        />
        <MetricChart
          title="Swap 使用率"
          series={pairSeries(history, (point) => point.payload.swapUsage)}
          formatter={metricChartFormatters.percent}
        />
      </article>

      <article className="historical-metric-group">
        <div className="historical-group-heading">
          <h3>磁盘</h3>
          <p>每个挂载点独立成图；消失的挂载点保留为空白分钟。</p>
        </div>
        {mountpoints.length > 0 ? (
          mountpoints.map((mountpoint) => (
            <MetricChart
              key={mountpoint}
              title={`${mountpoint} 使用率`}
              series={pairSeries(history, (point) => diskPair(point, mountpoint))}
              formatter={metricChartFormatters.percent}
            />
          ))
        ) : (
          <p className="historical-subempty">这个范围内没有持久磁盘数据。</p>
        )}
        <MetricChart
          title="磁盘 I/O"
          series={valueSeries(history, [
            {
              name: "读取",
              role: "primary",
              selector: (point) => point.payload.diskReadBps.average,
            },
            {
              name: "读取峰值",
              role: "maximum",
              selector: (point) => point.payload.diskReadBps.maximum,
            },
            {
              name: "写入",
              role: "context",
              selector: (point) => point.payload.diskWriteBps.average,
            },
            {
              name: "写入峰值",
              role: "context-maximum",
              selector: (point) => point.payload.diskWriteBps.maximum,
            },
          ])}
          formatter={metricChartFormatters.rate}
        />
      </article>

      <article className="historical-metric-group">
        <div className="historical-group-heading">
          <h3>网络</h3>
          <p>速率与累计流量分开显示，便于区分瞬时吞吐和长期用量。</p>
        </div>
        <MetricChart
          title="网络速率"
          series={valueSeries(history, [
            {
              name: "上传",
              role: "primary",
              selector: (point) => point.payload.uploadBps.average,
            },
            {
              name: "上传峰值",
              role: "maximum",
              selector: (point) => point.payload.uploadBps.maximum,
            },
            {
              name: "下载",
              role: "context",
              selector: (point) => point.payload.downloadBps.average,
            },
            {
              name: "下载峰值",
              role: "context-maximum",
              selector: (point) => point.payload.downloadBps.maximum,
            },
          ])}
          formatter={metricChartFormatters.rate}
        />
        <MetricChart
          title="累计网络流量"
          series={valueSeries(history, [
            {
              name: "累计上传",
              role: "primary",
              selector: (point) => point.payload.totalUpload,
            },
            {
              name: "累计下载",
              role: "context",
              selector: (point) => point.payload.totalDownload,
            },
          ])}
          formatter={metricChartFormatters.bytes}
        />
      </article>
    </div>
  );
}
