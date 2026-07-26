import type { MouseEvent } from "react";

import { ServerRow } from "../components/ServerRow";
import { SummaryPanel } from "../components/SummaryPanel";
import { useOverviewServerSnapshots } from "../live/useServerSnapshots";

interface OverviewPageProps {
  onNavigate: (path: string) => void;
}

interface CurrentErrorNoticeProps {
  disconnected: boolean;
  requestError: string | null;
  onRetry: (event: MouseEvent<HTMLButtonElement>) => void;
}

function CurrentErrorNotice({
  disconnected,
  requestError,
  onRetry,
}: CurrentErrorNoticeProps) {
  if (!disconnected && !requestError) {
    return null;
  }

  const message = disconnected
    ? requestError
      ? `页面仍在自动重连；刷新也没有成功：${requestError} 当前结果仍可继续查看。`
      : "页面会自动重连；当前结果仍可继续查看。"
    : `${requestError} 当前结果仍可继续查看。`;

  return (
    <div className="connection-notice" role={requestError ? "alert" : "status"}>
      <div>
        <strong>{disconnected ? "实时连接已断开" : "刷新没有成功"}</strong>
        <p>{message}</p>
      </div>
      <button className="button button-secondary" type="button" onClick={onRetry}>
        立即刷新
      </button>
    </div>
  );
}

function statusCopy(total: number, online: number): string {
  const offline = total - online;
  if (total === 0) {
    return "这里已经准备好，等第一台服务器来报到。";
  }
  if (offline === 0) {
    return `刚刚看过，${online} 台服务器都很安稳。`;
  }
  return `${online} 台在线，${offline} 台暂时没回应，值得看一眼。`;
}

export function OverviewPage({ onNavigate }: OverviewPageProps) {
  const {
    snapshots,
    connected,
    error,
    refresh,
    initialLoadStatus,
    connectionFailed,
    requestError,
  } = useOverviewServerSnapshots();
  const online = snapshots.filter((snapshot) => snapshot.online).length;
  const initialLoading =
    snapshots.length === 0 && initialLoadStatus === "pending";
  const initialError =
    snapshots.length === 0 && initialLoadStatus === "error";
  const initialEmpty =
    snapshots.length === 0 && initialLoadStatus === "success";
  const disconnected = connectionFailed;
  const introHeading = initialLoading
    ? "正在确认服务器状态"
    : initialError
      ? "暂时无法确认服务器状态"
      : snapshots.length > online
        ? "有服务器需要看一眼"
        : "服务器们都很安稳";
  const introStatus = initialLoading
    ? "正在看看每台服务器的近况。"
    : initialError
      ? "这会儿还不能确认服务器是否安稳。"
      : statusCopy(snapshots.length, online);
  const introDotClass = initialError
    ? " status-dot--offline"
    : initialLoading || snapshots.length > online
      ? " status-dot--attention"
      : "";

  const retry = (event: MouseEvent<HTMLButtonElement>) => {
    event.currentTarget.blur();
    void refresh();
  };

  return (
    <main className="dashboard-content" id="main-content">
      <section className="overview-intro" aria-labelledby="overview-title">
        <p className="calm-status">
          <span className={`status-dot${introDotClass}`} aria-hidden="true" />
          {introStatus}
        </p>
        <h1 id="overview-title">{introHeading}</h1>
        <p className="overview-description">
          一眼确认家里和身边的小服务器，安静地持续工作着。
        </p>
      </section>

      {snapshots.length > 0 ? <SummaryPanel snapshots={snapshots} /> : null}

      {initialLoading ? (
        <section className="dashboard-state dashboard-state--loading" role="status">
          <span className="loading-indicator" aria-hidden="true" />
          <div>
            <h2>正在接收服务器状态…</h2>
            <p>第一次载入可能需要片刻。</p>
          </div>
        </section>
      ) : null}

      {initialError ? (
        <section className="dashboard-state" role="alert">
          <div>
            <h2>这次没有取到状态</h2>
            <p>{error}</p>
          </div>
          <button className="button button-secondary" type="button" onClick={retry}>
            重新获取
          </button>
        </section>
      ) : null}

      {initialEmpty ? (
        <section className="dashboard-state dashboard-empty">
          <div>
            <h2>还没有服务器来报到</h2>
            <p>添加服务器后，它的在线状态和关键指标会出现在这里。</p>
          </div>
        </section>
      ) : null}

      {initialEmpty ? (
        <CurrentErrorNotice
          disconnected={disconnected}
          requestError={requestError}
          onRetry={retry}
        />
      ) : null}

      {snapshots.length > 0 ? (
        <section className="server-section" aria-labelledby="server-list-title">
          <div className="server-section-heading">
            <div>
              <h2 id="server-list-title">服务器</h2>
              <p>离线设备会排在前面，方便你先看到需要关心的变化。</p>
            </div>
            <p className="live-state" aria-live="polite">
              <span
                className={`server-status-dot server-status-dot--${connected ? "online" : "waiting"}`}
                aria-hidden="true"
              />
              {connected ? "实时连接正常" : "正在建立实时连接"}
            </p>
          </div>

          <CurrentErrorNotice
            disconnected={disconnected}
            requestError={requestError}
            onRetry={retry}
          />

          <div className="server-list">
            {snapshots.map((snapshot) => (
              <ServerRow
                key={snapshot.serverId}
                snapshot={snapshot}
                onNavigate={onNavigate}
              />
            ))}
          </div>
        </section>
      ) : null}

      <footer className="dashboard-footer">
        <p>指标通常每 5 秒更新一次；短暂断线时会保留最近数据并自动重连。</p>
      </footer>
    </main>
  );
}
