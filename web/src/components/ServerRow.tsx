import type { MouseEvent } from "react";

import type { ServerSnapshot } from "../api/types";
import {
  formatDuration,
  formatRate,
  formatBytes,
  hasReport,
  highestDiskUsage,
  percentage,
} from "../utils/format";
import { MetricBar } from "./MetricBar";

interface ServerRowProps {
  snapshot: ServerSnapshot;
  onNavigate: (path: string) => void;
}

interface DataFieldProps {
  label: string;
  className?: string;
  children: React.ReactNode;
}

function DataField({ label, className = "", children }: DataFieldProps) {
  return (
    <div className={`server-row-field ${className}`.trim()}>
      <span className="server-row-label">{label}</span>
      {children}
    </div>
  );
}

export function ServerRow({ snapshot, onNavigate }: ServerRowProps) {
  const reported = hasReport(snapshot);
  const memoryUsage = reported
    ? percentage(snapshot.report.memory.usedBytes, snapshot.report.memory.totalBytes)
    : null;
  const diskUsage = reported ? highestDiskUsage(snapshot.report.disks) : null;
  const platform = reported
    ? [snapshot.report.host.platform, snapshot.report.host.platformVersion]
        .filter(Boolean)
        .join(" ") || snapshot.report.host.os || "—"
    : "—";

  const navigate = (event: MouseEvent<HTMLAnchorElement>) => {
    event.preventDefault();
    onNavigate(`/servers/${snapshot.serverId}`);
  };

  return (
    <article className="server-row" data-testid="server-row">
      <div className="server-row-identity">
        <span
          className={`server-status-dot server-status-dot--${snapshot.online ? "online" : "offline"}`}
          aria-hidden="true"
        />
        <div>
          <a
            className="server-name-link"
            href={`/servers/${snapshot.serverId}`}
            onClick={navigate}
          >
            {snapshot.serverName}
            <span className="sr-only">，查看详情</span>
          </a>
          <span className="server-status-text">
            {snapshot.online ? "在线" : "离线"}
          </span>
        </div>
      </div>

      <DataField label="系统" className="server-row-field--secondary">
        <strong>{platform}</strong>
      </DataField>
      <DataField label="运行时间" className="server-row-field--secondary">
        <strong>
          {reported ? formatDuration(snapshot.report.host.uptimeSeconds) : "—"}
        </strong>
      </DataField>
      <DataField label="CPU" className="server-row-field--mobile-core">
        <MetricBar
          label={`${snapshot.serverName} CPU`}
          value={reported ? snapshot.report.cpu.usagePercent : null}
        />
      </DataField>
      <DataField label="内存" className="server-row-field--mobile-core">
        <MetricBar label={`${snapshot.serverName} 内存`} value={memoryUsage} />
      </DataField>
      <DataField label="最高磁盘" className="server-row-field--mobile-core">
        <MetricBar label={`${snapshot.serverName} 最高磁盘`} value={diskUsage} />
      </DataField>
      <DataField label="上传" className="server-row-field--mobile-core">
        <strong>
          {reported ? formatRate(snapshot.report.network.uploadBytesPerSecond) : "—"}
        </strong>
      </DataField>
      <DataField label="下载" className="server-row-field--mobile-core">
        <strong>
          {reported
            ? formatRate(snapshot.report.network.downloadBytesPerSecond)
            : "—"}
        </strong>
      </DataField>
      <DataField label="累计上传" className="server-row-field--cumulative">
        <strong>
          {reported ? formatBytes(snapshot.report.network.totalUploadBytes) : "—"}
        </strong>
      </DataField>
      <DataField label="累计下载" className="server-row-field--cumulative">
        <strong>
          {reported ? formatBytes(snapshot.report.network.totalDownloadBytes) : "—"}
        </strong>
      </DataField>
    </article>
  );
}
