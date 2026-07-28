import { useEffect, useState } from "react";

import { apiErrorMessage } from "../api/client";
import { alertsApi } from "../alerts/api";
import type {
  AlertRule,
  AlertRuleInput,
  WebhookConfig,
} from "../alerts/types";
import { AlertEventList } from "../components/AlertEventList";
import { AlertRuleForm, metricDescription } from "../components/AlertRuleForm";
import { WebhookForm } from "../components/WebhookForm";
import { serverApi, type ServerRecord } from "../servers/api";

const emptyWebhook: WebhookConfig = {
  url: "",
  headers: {},
  bodyTemplate: `{
  "server": {{json .ServerName}},
  "metric": "{{.Metric}}",
  "status": "{{.Status}}",
  "currentValue": {{.CurrentValue}},
  "threshold": {{.Threshold}},
  "detailUrl": {{json .DetailURL}}
}`,
  enabled: false,
};

export function AlertsPage() {
  const [servers, setServers] = useState<ServerRecord[]>([]);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [events, setEvents] = useState<Awaited<ReturnType<typeof alertsApi.listEvents>>>([]);
  const [webhook, setWebhook] = useState<WebhookConfig>(emptyWebhook);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<AlertRule | undefined>();
  const [savingRule, setSavingRule] = useState(false);
  const [deleting, setDeleting] = useState<number | null>(null);
  const [savingWebhook, setSavingWebhook] = useState(false);

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const [nextServers, nextRules, nextEvents, nextWebhook] = await Promise.all([
        serverApi.list(),
        alertsApi.listRules(),
        alertsApi.listEvents(),
        alertsApi.getWebhook(),
      ]);
      setServers(nextServers);
      setRules(nextRules);
      setEvents(nextEvents);
      setWebhook(nextWebhook);
    } catch (reason) {
      setError(apiErrorMessage(reason, "暂时无法加载告警配置，请稍后重试。"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const saveRule = async (input: AlertRuleInput) => {
    setSavingRule(true);
    try {
      const saved = editing
        ? await alertsApi.updateRule(editing.id, input)
        : await alertsApi.createRule(input);
      setRules((current) => editing
        ? current.map((rule) => rule.id === saved.id ? saved : rule)
        : [...current, saved]);
      setEditing(undefined);
    } finally {
      setSavingRule(false);
    }
  };

  const toggleRule = async (rule: AlertRule) => {
    try {
      const saved = await alertsApi.updateRule(rule.id, { enabled: !rule.enabled });
      setRules((current) => current.map((item) => item.id === saved.id ? saved : item));
    } catch (reason) {
      setError(apiErrorMessage(reason, "暂时无法更新告警规则，请稍后重试。"));
    }
  };

  const deleteRule = async (rule: AlertRule) => {
    try {
      await alertsApi.deleteRule(rule.id);
      setRules((current) => current.filter((item) => item.id !== rule.id));
      setDeleting(null);
      if (editing?.id === rule.id) setEditing(undefined);
    } catch (reason) {
      setError(apiErrorMessage(reason, "暂时无法删除告警规则，请稍后重试。"));
    }
  };

  const saveWebhook = async (config: WebhookConfig) => {
    setSavingWebhook(true);
    try {
      setWebhook(await alertsApi.updateWebhook(config));
    } finally {
      setSavingWebhook(false);
    }
  };

  if (loading) {
    return <main className="dashboard-content management-content" id="main-content"><section className="dashboard-state"><div><h1>正在加载告警配置…</h1></div></section></main>;
  }
  return <main className="dashboard-content management-content alerts-content" id="main-content">
    <section className="management-heading alerts-heading">
      <div><p className="eyebrow">告警</p><h1>告警与通知</h1><p>为单台服务器设置阈值或离线告警，并将触发与恢复事件投递到你的 Webhook。</p></div>
      <button className="button button-secondary" type="button" onClick={() => void load()}>刷新</button>
    </section>
    {error ? <p className="management-error" role="alert">{error}</p> : null}
    <section className="alert-section" aria-labelledby="alert-rules-title">
      <div className="alert-section-heading"><div><p className="eyebrow">规则</p><h2 id="alert-rules-title">告警规则</h2></div><p>资源指标持续越界后触发；离线状态由 30 秒未上报判断。</p></div>
      <AlertRuleForm servers={servers} initial={editing} saving={savingRule} onSave={saveRule} onCancel={editing ? () => setEditing(undefined) : undefined} />
      <div className="alert-rule-list">
        {rules.length === 0 ? <p className="alert-empty">还没有规则。先为一台服务器添加告警条件。</p> : rules.map((rule) => {
          const server = servers.find((item) => item.id === rule.serverId);
          return <article className="alert-rule-row" key={rule.id}>
            <div><strong>{server?.name ?? `服务器 #${rule.serverId}`}</strong><p>{metricDescription(rule.metric, rule.threshold)}{rule.metric === "offline" ? "" : `，持续 ${rule.durationSeconds} 秒`}</p></div>
            <span className={`alert-status alert-status--${rule.state}`}>{rule.state === "firing" ? "触发" : rule.state === "pending" ? "等待中" : rule.state === "recovered" ? "已恢复" : "正常"}</span>
            <label className="alert-switch"><input aria-label={`启用 ${server?.name ?? rule.id} 告警规则`} type="checkbox" checked={rule.enabled} onChange={() => void toggleRule(rule)} /><span>{rule.enabled ? "已启用" : "已停用"}</span></label>
            <div className="managed-server-actions"><button className="button button-quiet" type="button" onClick={() => setEditing(rule)}>编辑</button><button className="button button-danger-quiet" type="button" onClick={() => setDeleting(rule.id)}>删除</button></div>
            {deleting === rule.id ? <div className="inline-confirmation"><span>删除后无法恢复此规则及其事件记录。</span><button className="button button-danger" type="button" onClick={() => void deleteRule(rule)}>确认删除</button><button className="button button-quiet" type="button" onClick={() => setDeleting(null)}>取消</button></div> : null}
          </article>;
        })}
      </div>
    </section>
    <section className="alert-section" aria-labelledby="alert-events-title">
      <div className="alert-section-heading"><div><p className="eyebrow">历史</p><h2 id="alert-events-title">告警事件</h2></div><p>只记录触发、恢复和通知投递的结果。</p></div>
      <AlertEventList events={events} />
    </section>
    <section className="alert-section" aria-labelledby="webhook-title">
      <div className="alert-section-heading"><div><p className="eyebrow">通知</p><h2 id="webhook-title">Webhook 通知</h2></div><p>失败时最多重试三次；失败不会影响 Agent 上报。</p></div>
      <WebhookForm initial={webhook} saving={savingWebhook} onSave={saveWebhook} onTest={alertsApi.testWebhook} />
    </section>
  </main>;
}
