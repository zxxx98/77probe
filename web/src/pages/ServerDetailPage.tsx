import { useEffect, useState, type MouseEvent } from "react";

import { api, apiErrorMessage } from "../api/client";
import type { ServerSnapshot } from "../api/types";
import { MetricBar } from "../components/MetricBar";
import {
  formatBytes,
  formatDuration,
  formatRate,
  formatTimestamp,
  formatUnixTimestamp,
  hasReport,
  percentage,
} from "../utils/format";

interface ServerDetailPageProps {
  serverId: number;
  onNavigate: (path: string) => void;
}

interface FactProps {
  label: string;
  value: string;
}

function Fact({ label, value }: FactProps) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

export function ServerDetailPage({
  serverId,
  onNavigate,
}: ServerDetailPageProps) {
  const [snapshot, setSnapshot] = useState<ServerSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setSnapshot(null);
    setLoading(true);
    setError(null);
    api
      .getServerStatus(serverId, controller.signal)
      .then((next) => {
        if (active) {
          setSnapshot(next);
        }
      })
      .catch((loadError: unknown) => {
        if (active && !controller.signal.aborted) {
          setError(
            apiErrorMessage(loadError, "暂时无法获取这台服务器的状态。"),
          );
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false);
        }
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [serverId, reload]);

  const navigateBack = (event: MouseEvent<HTMLAnchorElement>) => {
    event.preventDefault();
    onNavigate("/");
  };

  if (loading && !snapshot) {
    return (
      <main className="dashboard-content" id="main-content">
        <a className="back-link" href="/" onClick={navigateBack}>
          <span aria-hidden="true">←</span> 返回概览
        </a>
        <section className="dashboard-state dashboard-state--loading" role="status">
          <span className="loading-indicator" aria-hidden="true" />
          <div>
            <h1>正在读取服务器状态…</h1>
            <p>很快就好。</p>
          </div>
        </section>
      </main>
    );
  }

  if (error && !snapshot) {
    return (
      <main className="dashboard-content" id="main-content">
        <a className="back-link" href="/" onClick={navigateBack}>
          <span aria-hidden="true">←</span> 返回概览
        </a>
        <section className="dashboard-state" role="alert">
          <div>
            <h1>没有取到这台服务器</h1>
            <p>{error}</p>
          </div>
          <button
            className="button button-secondary"
            type="button"
            onClick={() => setReload((value) => value + 1)}
          >
            重新获取
          </button>
        </section>
      </main>
    );
  }

  if (!snapshot) {
    return null;
  }

  const reported = hasReport(snapshot);
  const memoryUsage = reported
    ? percentage(snapshot.report.memory.usedBytes, snapshot.report.memory.totalBytes)
    : null;
  const swapUsage = reported
    ? percentage(
        snapshot.report.memory.swapUsedBytes,
        snapshot.report.memory.swapTotalBytes,
      )
    : null;
  const host = snapshot.report.host;
  const network = snapshot.report.network;
  const disks = snapshot.report.disks ?? [];

  return (
    <main className="dashboard-content detail-page" id="main-content">
      <a className="back-link" href="/" onClick={navigateBack}>
        <span aria-hidden="true">←</span> 返回概览
      </a>

      <header className="detail-heading">
        <div>
          <p className="calm-status">
            <span
              className={`status-dot${snapshot.online ? "" : " status-dot--offline"}`}
              aria-hidden="true"
            />
            {snapshot.online ? "在线，最近一次状态正常送达" : "离线，展示最近一次状态"}
          </p>
          <h1>{snapshot.serverName}</h1>
        </div>
        <dl className="detail-report-time">
          <div>
            <dt>最后上报</dt>
            <dd>{formatTimestamp(snapshot.lastReceivedAt)}</dd>
          </div>
          <div>
            <dt>采集时间</dt>
            <dd>
              {reported
                ? formatUnixTimestamp(snapshot.report.collectedAtUnix)
                : "尚无数据"}
            </dd>
          </div>
        </dl>
      </header>

      {!reported ? (
        <section className="connection-notice" role="status">
          <div>
            <strong>这台服务器还没有上报过</strong>
            <p>安装并启动探针后，系统和指标信息会在这里出现。</p>
          </div>
        </section>
      ) : null}

      <section className="detail-section" aria-labelledby="system-facts-title">
        <div className="detail-section-heading">
          <h2 id="system-facts-title">系统信息</h2>
          <p>当前主机身份与运行环境</p>
        </div>
        <dl className="fact-list">
          <Fact label="主机名" value={reported ? host.hostname || "—" : "—"} />
          <Fact label="操作系统" value={reported ? host.os || "—" : "—"} />
          <Fact
            label="发行版"
            value={
              reported
                ? [host.platform, host.platformVersion].filter(Boolean).join(" ") || "—"
                : "—"
            }
          />
          <Fact label="内核" value={reported ? host.kernelVersion || "—" : "—"} />
          <Fact label="架构" value={reported ? host.architecture || "—" : "—"} />
          <Fact label="CPU" value={reported ? host.cpuModel || "—" : "—"} />
          <Fact label="核心数" value={reported ? `${host.cpuCores}` : "—"} />
          <Fact label="主 IP" value={reported ? host.primaryIp || "—" : "—"} />
          <Fact
            label="运行时间"
            value={reported ? formatDuration(host.uptimeSeconds) : "—"}
          />
          <Fact
            label="启动时间"
            value={reported ? formatUnixTimestamp(host.bootTimeUnix) : "—"}
          />
          <Fact
            label="探针版本"
            value={reported ? snapshot.report.agentVersion || "—" : "—"}
          />
          <Fact label="来源 IP" value={snapshot.sourceIp || "—"} />
        </dl>
      </section>

      <section className="detail-section" aria-labelledby="current-metrics-title">
        <div className="detail-section-heading detail-section-heading--range">
          <div>
            <h2 id="current-metrics-title">当前指标</h2>
            <p>历史趋势将在下一阶段开放</p>
          </div>
          <div className="range-strip" aria-label="时间范围">
            <button type="button" aria-pressed="true">
              实时
            </button>
            {[
              "1天",
              "7天",
              "30天",
            ].map((label) => (
              <button key={label} type="button" disabled aria-pressed="false">
                {label}
              </button>
            ))}
          </div>
        </div>

        <div className="detail-metric-groups">
          <article className="detail-metric-group">
            <h3>处理器与负载</h3>
            <dl>
              <div>
                <dt>CPU 使用率</dt>
                <dd>
                  <MetricBar
                    label="CPU 使用率"
                    value={reported ? snapshot.report.cpu.usagePercent : null}
                  />
                </dd>
              </div>
              <div>
                <dt>Load 1 / 5 / 15</dt>
                <dd>
                  {reported
                    ? `${snapshot.report.cpu.load1.toFixed(2)} / ${snapshot.report.cpu.load5.toFixed(2)} / ${snapshot.report.cpu.load15.toFixed(2)}`
                    : "—"}
                </dd>
              </div>
            </dl>
          </article>

          <article className="detail-metric-group">
            <h3>内存与交换空间</h3>
            <dl>
              <div>
                <dt>内存</dt>
                <dd>
                  <MetricBar label="内存使用率" value={memoryUsage} />
                  <span className="metric-detail">
                    {reported
                      ? `${formatBytes(snapshot.report.memory.usedBytes)} / ${formatBytes(snapshot.report.memory.totalBytes)}`
                      : "—"}
                  </span>
                </dd>
              </div>
              <div>
                <dt>Swap</dt>
                <dd>
                  <MetricBar label="Swap 使用率" value={swapUsage} />
                  <span className="metric-detail">
                    {reported
                      ? snapshot.report.memory.swapTotalBytes > 0
                        ? `${formatBytes(snapshot.report.memory.swapUsedBytes)} / ${formatBytes(snapshot.report.memory.swapTotalBytes)}`
                        : "未配置 Swap"
                      : "—"}
                  </span>
                </dd>
              </div>
            </dl>
          </article>

          <article className="detail-metric-group detail-metric-group--wide">
            <h3>磁盘</h3>
            {reported && disks.length > 0 ? (
              <div className="disk-list">
                {disks.map((disk) => (
                  <div className="disk-row" key={disk.mountpoint}>
                    <strong>{disk.mountpoint}</strong>
                    <MetricBar
                      label={`${disk.mountpoint} 使用率`}
                      value={percentage(disk.usedBytes, disk.totalBytes)}
                    />
                    <span>
                      {formatBytes(disk.usedBytes)} / {formatBytes(disk.totalBytes)}
                    </span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="missing-value">暂无磁盘数据</p>
            )}
            <dl className="detail-inline-values">
              <div>
                <dt>磁盘读取</dt>
                <dd>
                  {reported
                    ? formatRate(snapshot.report.diskIo.readBytesPerSecond)
                    : "—"}
                </dd>
              </div>
              <div>
                <dt>磁盘写入</dt>
                <dd>
                  {reported
                    ? formatRate(snapshot.report.diskIo.writeBytesPerSecond)
                    : "—"}
                </dd>
              </div>
            </dl>
          </article>

          <article className="detail-metric-group detail-metric-group--wide">
            <h3>网络</h3>
            <dl className="network-detail-values">
              <Fact label="接口" value={reported ? network.interface || "—" : "—"} />
              <Fact
                label="当前上传"
                value={reported ? formatRate(network.uploadBytesPerSecond) : "—"}
              />
              <Fact
                label="当前下载"
                value={reported ? formatRate(network.downloadBytesPerSecond) : "—"}
              />
              <Fact
                label="累计上传"
                value={reported ? formatBytes(network.totalUploadBytes) : "—"}
              />
              <Fact
                label="累计下载"
                value={reported ? formatBytes(network.totalDownloadBytes) : "—"}
              />
            </dl>
          </article>
        </div>
      </section>
    </main>
  );
}
