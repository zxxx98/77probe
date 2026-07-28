export type AlertMetric =
  | "offline"
  | "cpu_usage"
  | "memory_usage"
  | "disk_usage"
  | "disk_free_bytes";

export type AlertOperator = "gt" | "lt";
export type AlertStatus = "normal" | "pending" | "firing" | "recovered";

export interface AlertRule {
  id: number;
  serverId: number;
  metric: AlertMetric;
  operator: AlertOperator;
  threshold: number;
  durationSeconds: number;
  repeatSeconds: number;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
  state: AlertStatus;
}

export interface AlertRuleInput {
  serverId: number;
  metric: AlertMetric;
  operator: AlertOperator;
  threshold: number;
  durationSeconds?: number;
  repeatSeconds: number;
  enabled: boolean;
}

export interface WebhookAttempt {
  id: number;
  eventId?: number;
  isTest: boolean;
  attempt: number;
  responseStatus?: number;
  errorText: string;
  sentAt: string;
}

export interface AlertEvent {
  id: number;
  ruleId: number;
  serverId: number;
  serverName: string;
  metric: AlertMetric;
  status: AlertStatus;
  currentValue?: number;
  threshold?: number;
  startedAt: string;
  endedAt?: string;
  createdAt: string;
  attempts: WebhookAttempt[];
}

export interface WebhookConfig {
  url: string;
  headers: Record<string, string>;
  bodyTemplate: string;
  enabled: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface DeliveryOutcome {
  success: boolean;
  attempts: WebhookAttempt[];
}
