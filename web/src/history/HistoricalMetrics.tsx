import {
  MetricChart,
  metricChartFormatters,
  type MetricChartSeries,
} from "../components/MetricChart";
import type { HistoryResponse, Pair } from "./types";

function pairSeries(
  selector: (point: HistoryResponse["points"][number]) => Pair | null,
): MetricChartSeries[] {
  return [
    {
      name: "平均",
      role: "primary",
      pairOrdinal: 0,
      selector: (point) => selector(point)?.average ?? null,
    },
    {
      name: "峰值",
      role: "maximum",
      pairOrdinal: 0,
      selector: (point) => selector(point)?.maximum ?? null,
    },
  ];
}

function valueSeries(
  lines: Array<{
    name: string;
    role: MetricChartSeries["role"];
    pairOrdinal: number;
    selector: (point: HistoryResponse["points"][number]) => number | null;
  }>,
): MetricChartSeries[] {
  return lines;
}

function diskPair(
  point: HistoryResponse["points"][number],
  mountpoint: string,
): Pair | null {
  const disks = Array.isArray(point.payload?.disks) ? point.payload.disks : [];
  for (let index = disks.length - 1; index >= 0; index -= 1) {
    const disk = disks[index];
    if (disk?.mountpoint === mountpoint) {
      return disk.usage;
    }
  }
  return null;
}

export function HistoricalMetrics({ history }: { history: HistoryResponse }) {
  const mountpointSet = new Set<string>();
  for (const point of history.points) {
    const disks = Array.isArray(point?.payload?.disks) ? point.payload.disks : [];
    for (const disk of disks) {
      if (typeof disk?.mountpoint === "string") {
        mountpointSet.add(disk.mountpoint);
      }
    }
  }
  const mountpoints = [...mountpointSet].sort((left, right) =>
    left.localeCompare(right),
  );

  return (
    <div className="historical-metric-groups">
      <article className="historical-metric-group">
        <div className="historical-group-heading">
          <h3>处理器与负载</h3>
          <p>CPU 保留分钟平均与峰值，负载展示 1 / 5 / 15 分钟趋势。</p>
        </div>
        <MetricChart
          title="CPU 使用率"
          history={history}
          series={pairSeries((point) => point.payload.cpuUsage)}
          formatter={metricChartFormatters.percent}
        />
        <MetricChart
          title="系统负载"
          history={history}
          series={valueSeries([
            {
              name: "Load 1",
              role: "primary",
              pairOrdinal: 0,
              selector: (point) => point.payload.load1.average,
            },
            {
              name: "Load 1 峰值",
              role: "maximum",
              pairOrdinal: 0,
              selector: (point) => point.payload.load1.maximum,
            },
            {
              name: "Load 5",
              role: "context",
              pairOrdinal: 1,
              selector: (point) => point.payload.load5.average,
            },
            {
              name: "Load 5 峰值",
              role: "context-maximum",
              pairOrdinal: 1,
              selector: (point) => point.payload.load5.maximum,
            },
            {
              name: "Load 15",
              role: "context",
              pairOrdinal: 2,
              selector: (point) => point.payload.load15.average,
            },
            {
              name: "Load 15 峰值",
              role: "context-maximum",
              pairOrdinal: 2,
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
          history={history}
          series={pairSeries((point) => point.payload.memoryUsage)}
          formatter={metricChartFormatters.percent}
        />
        <MetricChart
          title="Swap 使用率"
          history={history}
          series={pairSeries((point) => point.payload.swapUsage)}
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
              history={history}
              series={pairSeries((point) => diskPair(point, mountpoint))}
              formatter={metricChartFormatters.percent}
            />
          ))
        ) : (
          <p className="historical-subempty">这个范围内没有持久磁盘数据。</p>
        )}
        <MetricChart
          title="磁盘 I/O"
          history={history}
          series={valueSeries([
            {
              name: "读取",
              role: "primary",
              pairOrdinal: 0,
              selector: (point) => point.payload.diskReadBps.average,
            },
            {
              name: "读取峰值",
              role: "maximum",
              pairOrdinal: 0,
              selector: (point) => point.payload.diskReadBps.maximum,
            },
            {
              name: "写入",
              role: "context",
              pairOrdinal: 1,
              selector: (point) => point.payload.diskWriteBps.average,
            },
            {
              name: "写入峰值",
              role: "context-maximum",
              pairOrdinal: 1,
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
          history={history}
          series={valueSeries([
            {
              name: "上传",
              role: "primary",
              pairOrdinal: 0,
              selector: (point) => point.payload.uploadBps.average,
            },
            {
              name: "上传峰值",
              role: "maximum",
              pairOrdinal: 0,
              selector: (point) => point.payload.uploadBps.maximum,
            },
            {
              name: "下载",
              role: "context",
              pairOrdinal: 1,
              selector: (point) => point.payload.downloadBps.average,
            },
            {
              name: "下载峰值",
              role: "context-maximum",
              pairOrdinal: 1,
              selector: (point) => point.payload.downloadBps.maximum,
            },
          ])}
          formatter={metricChartFormatters.rate}
        />
        <MetricChart
          title="累计网络流量"
          history={history}
          series={valueSeries([
            {
              name: "累计上传",
              role: "primary",
              pairOrdinal: 0,
              selector: (point) => point.payload.totalUpload,
            },
            {
              name: "累计下载",
              role: "context",
              pairOrdinal: 1,
              selector: (point) => point.payload.totalDownload,
            },
          ])}
          formatter={metricChartFormatters.bytes}
        />
      </article>
    </div>
  );
}
