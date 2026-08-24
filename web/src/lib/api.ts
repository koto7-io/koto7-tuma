export type { Connection, ConnectionStats, PlatformMetrics, HourlyCount } from "./types";
import type { Connection, PlatformMetrics } from "./types";

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
  patchConnection: (id: string, body: Partial<Connection>) =>
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
};
