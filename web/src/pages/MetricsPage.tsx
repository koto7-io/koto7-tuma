import { useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../lib/api";
import type { PlatformMetrics } from "../lib/types";
import { Card, Pill } from "../components/Layout";
import { PageHeader } from "../components/PageHeader";
import { HourlyBars, StatCell } from "../components/StatsRow";
import { usePageRestore } from "../lib/usePageRestore";

export function MetricsPage() {
  const [m, setM] = useState<PlatformMetrics | null>(null);

  usePageRestore(() => {
    api.getMetrics().then(setM).catch(console.error);
    const t = setInterval(() => api.getMetrics().then(setM).catch(console.error), 30_000);
    return () => clearInterval(t);
  }, []);

  if (!m) return <p className="tuma-loading-page">Loading…</p>;

  const connections = m.connections ?? [];

  return (
    <div>
      <PageHeader
        title="Metrics"
        subtitle="Last 24 hours · refreshes every 30s"
      />

      <div className="tuma-metrics-grid">
        <Card>
          <div className="tuma-card-body">
            <StatCell label="Events received" value={m.events_received_24h.toLocaleString()} />
          </div>
        </Card>
        <Card>
          <div className="tuma-card-body">
            <StatCell label="Delivered" value={m.deliveries_delivered_24h.toLocaleString()} />
          </div>
        </Card>
        <Card>
          <div className="tuma-card-body">
            <StatCell label="Failed deliveries" value={m.deliveries_failed_24h.toLocaleString()} />
          </div>
        </Card>
        <Card>
          <div className="tuma-card-body">
            <StatCell label="p95 latency" value={m.p95_latency_ms > 0 ? `${m.p95_latency_ms} ms` : "—"} />
          </div>
        </Card>
        <Card>
          <div className="tuma-card-body">
            <StatCell label="Open issues" value={m.open_issues} />
          </div>
        </Card>
      </div>

      <div className="tuma-metrics-charts">
        <Card title="Ingestion">
          <div className="tuma-card-body">
            <HourlyBars data={m.ingestion_hourly} label="Events per hour" />
          </div>
        </Card>
        <Card title="Deliveries">
          <div className="tuma-card-body">
            <HourlyBars data={m.delivery_hourly} label="Successful deliveries per hour" />
          </div>
        </Card>
      </div>

      <Card title="By connection">
        <div>
          {connections.length === 0 && (
            <p className="tuma-empty tuma-pad-16">No connections yet.</p>
          )}
          {connections.length > 0 && (
            <div className="tuma-table-head tuma-table-head--metrics">
              <span>Connection</span>
              <span>Delivered</span>
              <span>Failed</span>
              <span>Issues</span>
              <span>p95</span>
              <span>Status</span>
            </div>
          )}
          {connections.map((c) => (
            <Link key={c.connection_id} to={`/connections/${c.connection_id}`} className="tuma-link-card">
              <div className="tuma-table-row tuma-table-row--metrics">
                <span className="tuma-fw-500">{c.name}</span>
                <span className="tuma-mono">{c.delivered_24h}</span>
                <span className="tuma-mono">{c.failed_24h}</span>
                <span className="tuma-mono">{c.open_issues}</span>
                <span className="tuma-mono">
                  {c.p95_latency_ms > 0 ? `${c.p95_latency_ms}ms` : "—"}
                </span>
                <span className="tuma-text-right">
                  <Pill status={c.status} />
                </span>
              </div>
            </Link>
          ))}
        </div>
      </Card>
    </div>
  );
}
