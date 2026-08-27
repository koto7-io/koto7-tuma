import { useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../lib/api";
import type { PlatformMetrics } from "../lib/types";
import { Card, Pill } from "../components/Layout";
import { HourlyBars, StatCell } from "../components/StatsRow";
import { usePageRestore } from "../lib/usePageRestore";

export function MetricsPage() {
  const [m, setM] = useState<PlatformMetrics | null>(null);

  usePageRestore(() => {
    api.getMetrics().then(setM).catch(console.error);
    const t = setInterval(() => api.getMetrics().then(setM).catch(console.error), 30_000);
    return () => clearInterval(t);
  }, []);

  if (!m) return <p>Loading…</p>;

  const connections = m.connections ?? [];

  return (
    <div>
      <div style={{ marginBottom: 20 }}>
        <h1 style={{ margin: 0, fontSize: 24 }}>Metrics</h1>
        <p style={{ margin: "6px 0 0", color: "var(--muted)", fontSize: 14 }}>
          Last 24 hours · refreshes every 30s
        </p>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(5, 1fr)", gap: 12, marginBottom: 16 }}>
        <Card>
          <div style={{ padding: 16 }}>
            <StatCell label="Events received" value={m.events_received_24h.toLocaleString()} />
          </div>
        </Card>
        <Card>
          <div style={{ padding: 16 }}>
            <StatCell label="Delivered" value={m.deliveries_delivered_24h.toLocaleString()} />
          </div>
        </Card>
        <Card>
          <div style={{ padding: 16 }}>
            <StatCell label="Failed deliveries" value={m.deliveries_failed_24h.toLocaleString()} />
          </div>
        </Card>
        <Card>
          <div style={{ padding: 16 }}>
            <StatCell label="p95 latency" value={m.p95_latency_ms > 0 ? `${m.p95_latency_ms} ms` : "—"} />
          </div>
        </Card>
        <Card>
          <div style={{ padding: 16 }}>
            <StatCell label="Open issues" value={m.open_issues} />
          </div>
        </Card>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, marginBottom: 16 }}>
        <Card title="Ingestion">
          <div style={{ padding: 16 }}>
            <HourlyBars data={m.ingestion_hourly} label="Events per hour" />
          </div>
        </Card>
        <Card title="Deliveries">
          <div style={{ padding: 16 }}>
            <HourlyBars data={m.delivery_hourly} label="Successful deliveries per hour" />
          </div>
        </Card>
      </div>

      <Card title="By connection">
        <div>
          {connections.length === 0 && (
            <p style={{ padding: 16, margin: 0, color: "var(--muted)" }}>No connections yet.</p>
          )}
          {connections.length > 0 && (
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "1fr repeat(4, 90px) 80px",
                gap: 12,
                padding: "8px 16px",
                fontSize: 10,
                color: "var(--muted)",
                textTransform: "uppercase",
                letterSpacing: "0.04em",
                borderBottom: "1px solid var(--border-light)",
              }}
            >
              <span>Connection</span>
              <span style={{ textAlign: "right" }}>Delivered</span>
              <span style={{ textAlign: "right" }}>Failed</span>
              <span style={{ textAlign: "right" }}>Issues</span>
              <span style={{ textAlign: "right" }}>p95</span>
              <span style={{ textAlign: "right" }}>Status</span>
            </div>
          )}
          {connections.map((c) => (
            <Link
              key={c.connection_id}
              to={`/connections/${c.connection_id}`}
              style={{ textDecoration: "none", color: "inherit", display: "block" }}
            >
              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "1fr repeat(4, 90px) 80px",
                  gap: 12,
                  padding: "12px 16px",
                  borderBottom: "1px solid var(--border-light)",
                  fontSize: 13,
                  alignItems: "center",
                }}
              >
                <span style={{ fontWeight: 500 }}>{c.name}</span>
                <span style={{ fontFamily: "var(--mono)", textAlign: "right" }}>{c.delivered_24h}</span>
                <span style={{ fontFamily: "var(--mono)", textAlign: "right" }}>{c.failed_24h}</span>
                <span style={{ fontFamily: "var(--mono)", textAlign: "right" }}>{c.open_issues}</span>
                <span style={{ fontFamily: "var(--mono)", textAlign: "right" }}>
                  {c.p95_latency_ms > 0 ? `${c.p95_latency_ms}ms` : "—"}
                </span>
                <span style={{ textAlign: "right" }}>
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
