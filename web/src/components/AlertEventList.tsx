import { useState } from "react";

import type { AlertEvent, AlertMetric, AlertStatus } from "../alerts/types";

interface AlertEventListProps { events: AlertEvent[]; }

const statusLabels: Record<AlertStatus, string> = { normal: "正常", pending: "等待中", firing: "触发", recovered: "已恢复" };
const metricLabels: Record<AlertMetric, string> = { offline: "服务器离线", cpu_usage: "CPU 使用率", memory_usage: "内存使用率", disk_usage: "磁盘使用率", disk_free_bytes: "磁盘可用空间" };

function formatTime(value?: string): string { return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—"; }
function formatValue(value?: number): string { return value === undefined ? "—" : Number.isInteger(value) ? String(value) : value.toFixed(2); }

export function AlertEventList({ events }: AlertEventListProps) {
  const [expanded, setExpanded] = useState<number | null>(null);
  if (events.length === 0) { return <p className="alert-empty">尚无告警事件。规则触发或恢复后会显示在这里。</p>; }
  return <div className="alert-event-list">
    {events.map((event) => <article className="alert-event-row" key={event.id}>
      <div className="alert-event-primary">
        <span className={`alert-status alert-status--${event.status}`}>{statusLabels[event.status]}</span>
        <strong>{event.serverName}</strong>
        <span>{metricLabels[event.metric]}</span>
      </div>
      <dl className="alert-event-details">
        <div><dt>当前值</dt><dd>{formatValue(event.currentValue)}</dd></div>
        <div><dt>阈值</dt><dd>{formatValue(event.threshold)}</dd></div>
        <div><dt>开始</dt><dd>{formatTime(event.startedAt)}</dd></div>
        <div><dt>结束</dt><dd>{formatTime(event.endedAt)}</dd></div>
      </dl>
      <button className="button button-quiet" type="button" aria-expanded={expanded === event.id} onClick={() => setExpanded((current) => current === event.id ? null : event.id)}>
        {expanded === event.id ? "收起投递记录" : `投递记录（${event.attempts.length}）`}
      </button>
      {expanded === event.id ? <ol className="webhook-attempt-list">
        {event.attempts.length === 0 ? <li>尚未投递或未配置 Webhook。</li> : event.attempts.map((attempt) => <li key={attempt.id}>
          第 {attempt.attempt} 次：{attempt.responseStatus ? `HTTP ${attempt.responseStatus}` : "未收到响应"}{attempt.errorText ? ` · ${attempt.errorText}` : ""} · {formatTime(attempt.sentAt)}
        </li>)}
      </ol> : null}
    </article>)}
  </div>;
}
