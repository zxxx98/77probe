import { useEffect, useState, type FormEvent } from "react";

import type { DeliveryOutcome, WebhookConfig } from "../alerts/types";

interface HeaderRow { name: string; value: string; }
interface WebhookFormProps {
  initial: WebhookConfig;
  saving: boolean;
  onSave: (config: WebhookConfig) => Promise<void>;
  onTest: () => Promise<DeliveryOutcome>;
}

function rowsFromHeaders(headers: Record<string, string>): HeaderRow[] {
  const rows = Object.entries(headers).map(([name, value]) => ({ name, value }));
  return rows.length > 0 ? rows : [{ name: "", value: "" }];
}

export function WebhookForm({ initial, saving, onSave, onTest }: WebhookFormProps) {
  const [url, setURL] = useState(initial.url);
  const [enabled, setEnabled] = useState(initial.enabled);
  const [bodyTemplate, setBodyTemplate] = useState(initial.bodyTemplate);
  const [headers, setHeaders] = useState<HeaderRow[]>(() => rowsFromHeaders(initial.headers));
  const [error, setError] = useState("");
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState("");

  useEffect(() => {
    setURL(initial.url); setEnabled(initial.enabled); setBodyTemplate(initial.bodyTemplate); setHeaders(rowsFromHeaders(initial.headers)); setError("");
  }, [initial]);

  const config = (): WebhookConfig => ({ url: url.trim(), enabled, bodyTemplate, headers: headers.reduce<Record<string, string>>((result, row) => { if (row.name.trim()) { result[row.name.trim()] = row.value; } return result; }, {}) });
  const changeHeader = (index: number, key: keyof HeaderRow, value: string) => setHeaders((current) => current.map((row, rowIndex) => rowIndex === index ? { ...row, [key]: value } : row));
  const removeHeader = (index: number) => setHeaders((current) => current.length === 1 ? [{ name: "", value: "" }] : current.filter((_, rowIndex) => rowIndex !== index));

  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); setError(""); setTestResult("");
    try { await onSave(config()); } catch (reason) { setError(reason instanceof Error ? reason.message : "暂时无法保存 Webhook，请稍后重试。"); }
  };
  const test = async () => {
    setTesting(true); setError(""); setTestResult("");
    try { const outcome = await onTest(); setTestResult(outcome.success ? `测试发送成功（${outcome.attempts.length} 次尝试）。` : `测试发送失败（${outcome.attempts.length} 次尝试）。`); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "暂时无法发送测试。") }
    finally { setTesting(false) }
  };

  return <form className="webhook-form" onSubmit={save}>
    <label className="field"><span>Webhook URL</span><input aria-label="Webhook URL" type="url" value={url} placeholder="https://example.com/webhook" onChange={(event) => setURL(event.target.value)} required /></label>
    <label className="check-field"><input aria-label="启用 Webhook" type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /><span>启用 Webhook 通知</span></label>
    <fieldset className="webhook-headers"><legend>请求头</legend>{headers.map((header, index) => <div className="webhook-header-row" key={index}>
      <input aria-label={`请求头名称 ${index + 1}`} value={header.name} placeholder="Authorization" onChange={(event) => changeHeader(index, "name", event.target.value)} />
      <input aria-label={header.name ? `请求头值 ${header.name}` : `请求头值 ${index + 1}`} value={header.value} placeholder="值" onChange={(event) => changeHeader(index, "value", event.target.value)} />
      <button className="button button-quiet" type="button" onClick={() => removeHeader(index)}>删除</button>
    </div>)}<button className="button button-quiet" type="button" onClick={() => setHeaders((current) => [...current, { name: "", value: "" }])}>添加请求头</button></fieldset>
    <label className="field"><span>JSON 模板</span><textarea aria-label="JSON 模板" value={bodyTemplate} rows={12} onChange={(event) => setBodyTemplate(event.target.value)} /></label>
    <aside className="webhook-variables"><strong>可用变量</strong><code>.ServerName</code><code>.Metric</code><code>.Status</code><code>.CurrentValue</code><code>.Threshold</code><code>.StartedAt</code><code>.DetailURL</code></aside>
    <div className="webhook-actions"><button className="button button-primary" type="submit" disabled={saving}>{saving ? "保存中…" : "保存 Webhook"}</button><button className="button button-secondary" type="button" onClick={test} disabled={testing || saving}>{testing ? "发送中…" : "发送测试"}</button></div>
    {testResult ? <p className="copy-feedback" role="status">{testResult}</p> : null}
    {error ? <p className="management-error" role="alert">{error}</p> : null}
  </form>;
}
