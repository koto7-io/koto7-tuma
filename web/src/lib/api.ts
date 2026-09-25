export type { Connection, ConnectionStats, PlatformMetrics, HourlyCount } from "./types";

export type AlertRule = {
  id: string;
  name: string;
  rule_type: "OPEN_ISSUES" | "DELIVERY_SUCCESS" | "UNRESOLVED_TIME";
  threshold: number;
  unit: "COUNT" | "PERCENT" | "HOURS";
  active: boolean;
  notification_type: string;
  notification_dest: string;
  subject_template?: string | null;
  body_template?: string | null;
  created_at: string;
  updated_at: string;
};
import type { Connection, PlatformMetrics } from "./types";

export type PlaygroundLogEvent = {
  ts?: string;
  line?: string;
  delivery_id?: string;
  body_preview?: string;
};

export type PlaygroundStatus = {
  status: string;
  delivered_24h: number;
  open_issues: number;
  inbound_url: string;
  fail_destination: boolean;
  recent_deliveries?: Delivery[];
};

export type PlaygroundBootstrap = {
  session_id: string;
  connection_id: string;
  inbound_url: string;
  inbound_path: string;
  source_type: string;
  destination_fail: boolean;
};

export type SinkEvent = {
  ts: string;
  status: number;
  delivery_id?: string;
  attempt?: string;
  body: string;
};

export type Delivery = {
  id: string;
  event_id: string;
  attempt_number: number;
  status: string;
  response_code?: number;
  latency_ms?: number;
  attempted_at: string;
};

export type Issue = {
  id: string;
  event_id: string;
  connection_id: string;
  reason: string;
  attempts_exhausted: number;
  first_failed_at: string;
  status: string;
  provider_event_id?: string;
  destination?: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
  login: (email: string, password: string) =>
    request<{ email: string }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  logout: () => request<void>("/api/auth/logout", { method: "POST" }),
  listConnections: () =>
    request<{ connections: Connection[] }>("/api/connections"),
  getConnection: (id: string) => request<Connection>(`/api/connections/${id}`),
  createConnection: (body: {
    name: string;
    source_type: string;
    destination_url: string;
    signing_secret?: string;
  }) =>
    request<{ connection: Connection; signing_secret: string }>(
      "/api/connections",
      { method: "POST", body: JSON.stringify(body) }
    ),
  patchConnection: (id: string, body: Partial<Connection> & { signing_secret?: string }) =>
    request<Connection>(`/api/connections/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  listDeliveries: (id: string) =>
    request<{ deliveries: Delivery[] }>(`/api/connections/${id}/deliveries`),
  listIssues: (status: string) =>
    request<{ issues: Issue[] }>(`/api/issues?status=${status}`),
  getIssue: (id: string) =>
    request<{ issue: Issue; payload: string }>(`/api/issues/${id}`),
  replayIssue: (id: string) =>
    request<{ status: string }>(`/api/issues/${id}/replay`, { method: "POST" }),
  replayBulk: (ids: string[]) =>
    request<{ started: number }>("/api/issues/replay-bulk", {
      method: "POST",
      body: JSON.stringify({ ids }),
    }),
  getMetrics: () => request<PlatformMetrics>("/api/metrics"),
  getConfig: () => request<{ demo_mode: boolean; sink_url: string }>("/api/config"),
  getSink: () =>
    request<{ url: string; fail: boolean; events: SinkEvent[] }>("/api/sink"),
  sinkBreak: () => request<{ fail: boolean }>("/api/sink/break", { method: "POST" }),
  sinkFix: () => request<{ fail: boolean }>("/api/sink/fix", { method: "POST" }),
  getConfig: () => request<{ demo_mode: boolean }>("/api/config"),
  listAlertRules: () =>
    request<{ alert_rules: AlertRule[]; active_count: number; total_count: number }>("/api/alert-rules"),
  createAlertRule: (body: {
    name: string;
    rule_type: string;
    threshold: number;
    unit: string;
    notification_type: string;
    notification_dest: string;
    subject_template?: string;
    body_template?: string;
  }) => request<{ alert_rule: AlertRule }>("/api/alert-rules", { method: "POST", body: JSON.stringify(body) }),
  patchAlertRule: (id: string, body: { active?: boolean; threshold?: number; subject_template?: string | null; body_template?: string | null }) =>
    request<AlertRule>(`/api/alert-rules/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteAlertRule: (id: string) =>
    request<void>(`/api/alert-rules/${id}`, { method: "DELETE" }),
  playgroundBootstrap: () => request<PlaygroundBootstrap>("/api/playground/bootstrap"),
  playgroundStatus: () => request<PlaygroundStatus>("/api/playground/status"),
  playgroundSimulate: (body: { provider: string; count: number; duplicate_event_id?: string }) =>
    request<{ sent: number; results: { status: number; body: string; event_id: string }[] }>(
      "/api/playground/simulate",
      { method: "POST", body: JSON.stringify(body) }
    ),
  playgroundBreak: () => request<{ status: string }>("/api/playground/break", { method: "POST" }),
  playgroundFix: () => request<{ status: string }>("/api/playground/fix", { method: "POST" }),
  playgroundReplay: () => request<{ status: string }>("/api/playground/replay", { method: "POST" }),
};
