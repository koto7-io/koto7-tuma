import { FormEvent, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, Connection, Delivery } from "../lib/api";
import { usePageRestore } from "../lib/usePageRestore";
import { Card, Pill } from "../components/Layout";
import { ConnectionStatsRow } from "../components/StatsRow";
import { buttonPrimary, buttonSecondary, inputStyle } from "./LoginPage";

export function ConnectionDetailPage() {
  const { id } = useParams();
  const [conn, setConn] = useState<Connection | null>(null);
  const [deliveries, setDeliveries] = useState<Delivery[]>([]);
  const [copied, setCopied] = useState(false);

  usePageRestore(() => {
    if (!id) return;
    api.getConnection(id).then(setConn).catch(console.error);
    api.listDeliveries(id).then((r) => setDeliveries(r.deliveries)).catch(console.error);
  }, [id]);

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!conn || !id) return;
    const updated = await api.patchConnection(id, {
      destination_url: conn.destination_url,
      retry_attempts: conn.retry_attempts,
      retry_first_delay_s: conn.retry_first_delay_s,
      retry_backoff_factor: conn.retry_backoff_factor,
      retention_days: conn.retention_days,
    });
    setConn(updated);
  }

  if (!conn) return <p>Loading…</p>;

  const schedule = Array.from({ length: conn.retry_attempts }, (_, i) => {
    let delay = conn.retry_first_delay_s;
    for (let j = 1; j <= i; j++) delay *= conn.retry_backoff_factor;
    return `Attempt ${i + 1}: +${Math.round(delay)}s`;
  });

  return (
    <div>
      <Link to="/connections" style={{ fontSize: 13, color: "var(--muted)" }}>← All connections</Link>
      <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "10px 0 20px" }}>
        <h1 style={{ margin: 0, fontSize: 24 }}>{conn.name}</h1>
        <Pill status={conn.stats?.status ?? "healthy"} />
      </div>

      <Card title="Last 24 hours">
        <div style={{ padding: 16 }}>
          <ConnectionStatsRow stats={conn.stats} />
        </div>
      </Card>

      <div style={{ display: "grid", gap: 16, marginTop: 16 }}>
        <Card title="Webhook URL">
          <div style={{ padding: 16 }}>
            <p style={{ margin: "0 0 10px", color: "var(--muted)", fontSize: 14 }}>
              Paste this into your {conn.source_type} endpoint settings.
            </p>
            <div style={{ display: "flex", gap: 8 }}>
              <code style={{ flex: 1, fontFamily: "var(--mono)", fontSize: 12, background: "var(--code-bg)", border: "1px solid var(--border)", borderRadius: 6, padding: 10 }}>
                {conn.inbound_url}
              </code>
              <button
                style={buttonSecondary}
                onClick={() => {
                  navigator.clipboard.writeText(conn.inbound_url);
                  setCopied(true);
                  setTimeout(() => setCopied(false), 2000);
                }}
              >
                {copied ? "Copied" : "Copy"}
              </button>
            </div>
          </div>
        </Card>

        <Card title="Destination & retry policy">
          <form onSubmit={save} style={{ padding: 16, display: "grid", gap: 12 }}>
            <div>
              <label style={{ fontSize: 13 }}>Destination URL</label>
              <input style={{ ...inputStyle, marginTop: 6 }} value={conn.destination_url} onChange={(e) => setConn({ ...conn, destination_url: e.target.value })} />
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 10 }}>
              <div>
                <label style={{ fontSize: 13 }}>Retry attempts</label>
                <input type="number" min={1} style={{ ...inputStyle, marginTop: 6 }} value={conn.retry_attempts} onChange={(e) => setConn({ ...conn, retry_attempts: +e.target.value })} />
              </div>
              <div>
                <label style={{ fontSize: 13 }}>First delay (s)</label>
                <input type="number" min={1} style={{ ...inputStyle, marginTop: 6 }} value={conn.retry_first_delay_s} onChange={(e) => setConn({ ...conn, retry_first_delay_s: +e.target.value })} />
              </div>
              <div>
                <label style={{ fontSize: 13 }}>Backoff factor</label>
                <input type="number" step={0.1} min={1} style={{ ...inputStyle, marginTop: 6 }} value={conn.retry_backoff_factor} onChange={(e) => setConn({ ...conn, retry_backoff_factor: +e.target.value })} />
              </div>
            </div>
            <div>
              <label style={{ fontSize: 13 }}>Retention (days)</label>
              <input type="number" min={1} style={{ ...inputStyle, marginTop: 6, maxWidth: 120 }} value={conn.retention_days} onChange={(e) => setConn({ ...conn, retention_days: +e.target.value })} />
            </div>
            <div>
              <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 6 }}>Resulting schedule</div>
              <div style={{ fontFamily: "var(--mono)", fontSize: 11, color: "var(--muted)", lineHeight: 1.6 }}>
                {schedule.join(" · ")}
              </div>
            </div>
            <button type="submit" style={{ ...buttonPrimary, width: "fit-content" }}>Save changes</button>
          </form>
        </Card>

        <Card title="Recent deliveries">
          <div>
            {deliveries.length === 0 && (
              <p style={{ padding: 16, color: "var(--muted)", margin: 0 }}>No deliveries yet.</p>
            )}
            {deliveries.map((d) => (
              <div
                key={d.id}
                style={{
                  display: "grid",
                  gridTemplateColumns: "80px 1fr 80px 100px",
                  gap: 12,
                  padding: "12px 16px",
                  borderBottom: "1px solid var(--border-light)",
                  fontSize: 13,
                }}
              >
                <span style={{ fontFamily: "var(--mono)" }}>#{d.attempt_number}</span>
                <span>{d.status}</span>
                <span style={{ fontFamily: "var(--mono)" }}>{d.response_code ?? "—"}</span>
                <span style={{ color: "var(--muted)" }}>{d.latency_ms != null ? `${d.latency_ms} ms` : "—"}</span>
              </div>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}
