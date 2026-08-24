export type ConnectionStats = {
  delivered_24h: number;
  p95_latency_ms: number;
  failed_count: number;
  status: "healthy" | "degraded" | "failing";
};

export type Connection = {
  id: string;
  name: string;
  source_type: string;
  inbound_url: string;
  inbound_path: string;
  destination_url: string;
  retry_attempts: number;
  retry_first_delay_s: number;
  retry_backoff_factor: number;
  retention_days: number;
  created_at: string;
  stats?: ConnectionStats;
};

export type HourlyCount = { hour: string; count: number };

export type PlatformMetrics = {
  events_received_24h: number;
  deliveries_delivered_24h: number;
  deliveries_failed_24h: number;
  p95_latency_ms: number;
  open_issues: number;
  ingestion_hourly: HourlyCount[];
  delivery_hourly: HourlyCount[];
  connections: {
    connection_id: string;
    name: string;
    delivered_24h: number;
    failed_24h: number;
    open_issues: number;
    p95_latency_ms: number;
    status: string;
  }[];
};
