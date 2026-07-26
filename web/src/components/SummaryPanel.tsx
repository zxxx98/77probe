import type { ServerSnapshot } from "../api/types";
import { formatBytes, formatRate } from "../utils/format";

interface SummaryPanelProps {
  snapshots: ServerSnapshot[];
}

export function SummaryPanel({ snapshots }: SummaryPanelProps) {
  const online = snapshots.filter((snapshot) => snapshot.online).length;
  const abnormal = snapshots.length - online;
  const traffic = snapshots.reduce(
    (totals, snapshot) => {
      if (!snapshot.online) {
        return totals;
      }
      totals.upload += snapshot.report.network.uploadBytesPerSecond;
      totals.download += snapshot.report.network.downloadBytesPerSecond;
      totals.totalUpload += snapshot.report.network.totalUploadBytes;
      totals.totalDownload += snapshot.report.network.totalDownloadBytes;
      return totals;
    },
    { upload: 0, download: 0, totalUpload: 0, totalDownload: 0 },
  );

  return (
    <div className="overview-summaries">
      <section className="summary-panel" aria-label="服务器摘要">
        <dl className="summary-stat">
          <div>
            <dt>全部</dt>
            <dd>{snapshots.length}</dd>
          </div>
          <div>
            <dt>在线</dt>
            <dd className="text-success">{online}</dd>
          </div>
          <div>
            <dt>异常</dt>
            <dd className={abnormal > 0 ? "text-danger" : undefined}>
              {abnormal}
            </dd>
          </div>
        </dl>
      </section>

      <section className="network-summary" aria-labelledby="network-summary-title">
        <div>
          <h2 id="network-summary-title">此刻的网络</h2>
          <p>仅汇总当前在线服务器</p>
        </div>
        <dl className="network-summary-values">
          <div>
            <dt>上传</dt>
            <dd>{formatRate(traffic.upload)}</dd>
          </div>
          <div>
            <dt>下载</dt>
            <dd>{formatRate(traffic.download)}</dd>
          </div>
          <div className="network-summary-cumulative">
            <dt>累计上传 / 下载</dt>
            <dd>
              {formatBytes(traffic.totalUpload)} / {formatBytes(traffic.totalDownload)}
            </dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
