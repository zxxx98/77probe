import { request } from "../api/client";

import type {
  AlertEvent,
  AlertRule,
  AlertRuleInput,
  DeliveryOutcome,
  WebhookConfig,
} from "./types";

export const alertsApi = {
  listRules: () => request<AlertRule[]>("/api/alert-rules"),
  createRule: (input: AlertRuleInput) =>
    request<AlertRule>("/api/alert-rules", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  updateRule: (id: number, input: Partial<AlertRuleInput>) =>
    request<AlertRule>(`/api/alert-rules/${id}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteRule: (id: number) =>
    request<void>(`/api/alert-rules/${id}`, { method: "DELETE" }),
  listEvents: () => request<AlertEvent[]>("/api/alert-events"),
  getWebhook: () => request<WebhookConfig>("/api/webhook"),
  updateWebhook: (config: WebhookConfig) =>
    request<WebhookConfig>("/api/webhook", {
      method: "PUT",
      body: JSON.stringify(config),
    }),
  testWebhook: () =>
    request<DeliveryOutcome>("/api/webhook/test", { method: "POST" }),
};
