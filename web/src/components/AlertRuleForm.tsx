import { useEffect, useState, type FormEvent } from "react";

import type { ServerRecord } from "../servers/api";
import type { AlertMetric, AlertRule, AlertRuleInput } from "../alerts/types";

interface AlertRuleFormProps {
  servers: ServerRecord[];
  initial?: AlertRule;
  saving: boolean;
  onSave: (input: AlertRuleInput) => Promise<void>;
  onCancel?: () => void;
}

const metricLabels: Record<AlertMetric, string> = {
  offline: "服务器离线",
  cpu_usage: "CPU 使用率",
  memory_usage: "内存使用率",
  disk_usage: "磁盘使用率",
  disk_free_bytes: "磁盘可用空间",
};

function initialInput(rule?: AlertRule): AlertRuleInput {
  return {
    serverId: rule?.serverId ?? 0,
    metric: rule?.metric ?? "cpu_usage",
    operator: rule?.operator ?? "gt",
    threshold: rule?.threshold ?? 85,
    durationSeconds: rule?.durationSeconds ?? 300,
    repeatSeconds: rule?.repeatSeconds ?? 0,
    enabled: rule?.enabled ?? true,
  };
}

export function metricDescription(metric: AlertMetric, threshold: number): string {
  switch (metric) {
    case "offline":
      return "服务器离线"
    case "cpu_usage":
      return `CPU 使用率大于 ${threshold}%`;
    case "memory_usage":
      return `内存使用率大于 ${threshold}%`;
    case "disk_usage":
      return `磁盘使用率大于 ${threshold}%`;
    case "disk_free_bytes":
      return `磁盘可用空间小于 ${threshold} 字节`;
  }
}

export function AlertRuleForm({
  servers,
  initial,
  saving,
  onSave,
  onCancel,
}: AlertRuleFormProps) {
  const [input, setInput] = useState<AlertRuleInput>(() => initialInput(initial));
  const [error, setError] = useState("");

  useEffect(() => {
    setInput(initialInput(initial));
    setError("");
  }, [initial]);

  const offline = input.metric === "offline";
  const percent = input.metric !== "disk_free_bytes" && !offline;

  const changeMetric = (metric: AlertMetric) => {
    setInput((current) => ({
      ...current,
      metric,
      operator: metric === "disk_free_bytes" ? "lt" : "gt",
      threshold: metric === "offline" ? 0 : current.threshold,
      durationSeconds: metric === "offline" ? 0 : current.durationSeconds || 300,
    }));
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (input.serverId < 1) {
      setError("请选择服务器。");
      return;
    }
    if (!offline && (!Number.isFinite(input.threshold) || input.threshold < 0 || (percent && input.threshold > 100))) {
      setError("请输入有效阈值。");
      return;
    }
    setError("");
    try {
      await onSave({ ...input, durationSeconds: offline ? 0 : input.durationSeconds });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "暂时无法保存告警规则，请稍后重试。");
    }
  };

  return (
    <form className="alert-rule-form" onSubmit={submit}>
      <label className="field">
        <span>服务器</span>
        <select
          aria-label="服务器"
          value={input.serverId}
          onChange={(event) => setInput((current) => ({ ...current, serverId: Number(event.target.value) }))}
        >
          <option value={0}>选择服务器</option>
          {servers.map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}
        </select>
      </label>
      <label className="field">
        <span>指标</span>
        <select aria-label="指标" value={input.metric} onChange={(event) => changeMetric(event.target.value as AlertMetric)}>
          {(Object.keys(metricLabels) as AlertMetric[]).map((metric) => <option key={metric} value={metric}>{metricLabels[metric]}</option>)}
        </select>
      </label>
      <label className="field">
        <span>阈值{percent ? "（%）" : input.metric === "disk_free_bytes" ? "（字节）" : ""}</span>
        <input aria-label="阈值" type="number" min={0} max={percent ? 100 : undefined} value={input.threshold} disabled={offline} onChange={(event) => setInput((current) => ({ ...current, threshold: Number(event.target.value) }))} />
      </label>
      <label className="field">
        <span>持续时间（秒）</span>
        <input aria-label="持续时间（秒）" type="number" min={0} max={86400} value={offline ? 0 : input.durationSeconds} disabled={offline} onChange={(event) => setInput((current) => ({ ...current, durationSeconds: Number(event.target.value) }))} />
      </label>
      <label className="field">
        <span>重复通知（秒）</span>
        <input aria-label="重复通知（秒）" type="number" min={0} max={604800} step={300} value={input.repeatSeconds} onChange={(event) => setInput((current) => ({ ...current, repeatSeconds: Number(event.target.value) }))} />
      </label>
      <label className="check-field">
        <input aria-label="启用规则" type="checkbox" checked={input.enabled} onChange={(event) => setInput((current) => ({ ...current, enabled: event.target.checked }))} />
        <span>启用规则</span>
      </label>
      <div className="alert-rule-form-actions">
        <button className="button button-primary" type="submit" disabled={saving}>{saving ? "保存中…" : initial ? "保存规则" : "添加规则"}</button>
        {onCancel ? <button className="button button-quiet" type="button" onClick={onCancel} disabled={saving}>取消</button> : null}
      </div>
      {error ? <p className="management-error" role="alert">{error}</p> : null}
    </form>
  );
}
