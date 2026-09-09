import { FormEvent, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, Connection, Delivery, SinkEvent } from "../lib/api";
import { usePageRestore } from "../lib/usePageRestore";
import { Button } from "../components/Button";
import { Card, Pill } from "../components/Layout";
import { ConnectionStatsRow } from "../components/StatsRow";

export function ConnectionDetailPage() {
  const { id } = useParams();
  const [conn, setConn] = useState<Connection | null>(null);
  const [deliveries, setDeliveries] = useState<Delivery[]>([]);
  const [copiedInbound, setCopiedInbound] = useState(false);
  const [copiedSink, setCopiedSink] = useState(false);
  const [signingSecret, setSigningSecret] = useState("");
  const [saved, setSaved] = useState(false);
  const [sinkURL, setSinkURL] = useState("");
  const [sinkFail, setSinkFail] = useState(false);
  const [sinkEvents, setSinkEvents] = useState<SinkEvent[]>([]);

  usePageRestore(() => {
    if (!id) return;
    api.getConnection(id).then(setConn).catch(console.error);
    api.listDeliveries(id).then((r) => setDeliveries(r.deliveries)).catch(console.error);
    refreshSink();
  }, [id]);

  function refreshSink() {
    api.getSink().then((s) => {
      setSinkURL(s.url);
      setSinkFail(s.fail);
      setSinkEvents(s.events);
    }).catch(console.error);
  }

  useEffect(() => {
    const t = setInterval(() => {
      refreshSink();
      if (id) {
        api.listDeliveries(id).then((r) => setDeliveries(r.deliveries)).catch(() => {});
      }
    }, 2000);
    return () => clearInterval(t);
  }, [id]);

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!conn || !id) return;
    const updated = await api.patchConnection(id, {
      name: conn.name,
      destination_url: conn.destination_url,
      retry_attempts: conn.retry_attempts,
      retry_first_delay_s: conn.retry_first_delay_s,
      retry_backoff_factor: conn.retry_backoff_factor,
      retention_days: conn.retention_days,
      ...(signingSecret.trim() ? { signing_secret: signingSecret.trim() } : {}),
    });
    setConn(updated);
    setSigningSecret("");
    setSaved(true);
    setTimeout(() => setSaved(false), 1500);
  }

  async function useTestReceiver() {
    if (!conn || !id || !sinkURL) return;
    const updated = await api.patchConnection(id, { destination_url: sinkURL });
    setConn(updated);
  }

  if (!conn) return <p className="tuma-loading-page">Loading…</p>;

  const schedule = Array.from({ length: conn.retry_attempts }, (_, i) => {
    let delay = conn.retry_first_delay_s;
    for (let j = 1; j <= i; j++) delay *= conn.retry_backoff_factor;
    return `Attempt ${i + 1}: +${Math.round(delay)}s`;
  });

  return (
    <div>
      <Link to="/connections" className="tuma-back-link">← All connections</Link>
      <div className="tuma-page-title-row">
        <h1 className="tuma-page-title">{conn.name}</h1>
        <Pill status={conn.stats?.status ?? "healthy"} />
      </div>

      <Card title="Last 24 hours">
        <div className="tuma-card-body">
          <ConnectionStatsRow stats={conn.stats} />
        </div>
      </Card>

      <div className="tuma-stack tuma-stack--lg tuma-mt-16">
        <Card title="Webhook URL">
          <div className="tuma-card-body">
            <p className="tuma-page-sub tuma-mb-10">
              {conn.source_type === "stripe"
                ? "Paste this into Stripe Dashboard → Developers → Webhooks. Then save Stripe’s signing secret below."
                : `Paste this into your ${conn.source_type} endpoint settings.`}
            </p>
            <div className="tuma-code-row">
              <code className="tuma-code">{conn.inbound_url}</code>
              <Button
                variant="secondary"
                onClick={() => {
                  navigator.clipboard.writeText(conn.inbound_url);
                  setCopiedInbound(true);
                  setTimeout(() => setCopiedInbound(false), 2000);
                }}
              >
                {copiedInbound ? "Copied" : "Copy"}
              </Button>
            </div>
          </div>
        </Card>

        <Card title="Destination & retry policy">
          <form onSubmit={save} className="tuma-card-body tuma-stack">
            <label className="tuma-field tuma-field--flush">
              <span className="tuma-field__label">Name</span>
              <input
                className="tuma-input"
                value={conn.name}
                onChange={(e) => setConn({ ...conn, name: e.target.value })}
              />
            </label>
            <label className="tuma-field tuma-field--flush">
              <span className="tuma-field__label">Destination URL</span>
              <input
                className="tuma-input"
                value={conn.destination_url}
                onChange={(e) => setConn({ ...conn, destination_url: e.target.value })}
              />
            </label>
            <label className="tuma-field tuma-field--flush">
              <span className="tuma-field__label">
                {conn.source_type === "stripe" ? "Stripe signing secret" : "Signing secret"}
              </span>
              <input
                className="tuma-input"
                type="password"
                autoComplete="off"
                value={signingSecret}
                onChange={(e) => setSigningSecret(e.target.value)}
                placeholder={conn.source_type === "stripe" ? "whsec_… from Stripe (Reveal secret)" : "Paste to rotate"}
              />
              <span className="tuma-field-caption">
                {conn.source_type === "stripe"
                  ? "401s mean this does not match the endpoint in Stripe. Leave blank to keep the current secret."
                  : "Leave blank to keep the current secret."}
              </span>
            </label>
            <div className="tuma-form-grid-3">
              <label className="tuma-field tuma-field--flush">
                <span className="tuma-field__label">Retry attempts</span>
                <input
                  type="number"
                  min={1}
                  className="tuma-input"
                  value={conn.retry_attempts}
                  onChange={(e) => setConn({ ...conn, retry_attempts: +e.target.value })}
                />
              </label>
              <label className="tuma-field tuma-field--flush">
                <span className="tuma-field__label">First delay (s)</span>
                <input
                  type="number"
                  min={1}
                  className="tuma-input"
                  value={conn.retry_first_delay_s}
                  onChange={(e) => setConn({ ...conn, retry_first_delay_s: +e.target.value })}
                />
              </label>
              <label className="tuma-field tuma-field--flush">
                <span className="tuma-field__label">Backoff factor</span>
                <input
                  type="number"
                  step={0.1}
                  min={1}
                  className="tuma-input"
                  value={conn.retry_backoff_factor}
                  onChange={(e) => setConn({ ...conn, retry_backoff_factor: +e.target.value })}
                />
              </label>
            </div>
            <label className="tuma-field tuma-field--flush">
              <span className="tuma-field__label">Retention (days)</span>
              <input
                type="number"
                min={1}
                className="tuma-input tuma-input--narrow"
                value={conn.retention_days}
                onChange={(e) => setConn({ ...conn, retention_days: +e.target.value })}
              />
            </label>
            <div>
              <div className="tuma-field-caption">Resulting schedule</div>
              <div className="tuma-schedule">{schedule.join(" · ")}</div>
            </div>
            <Button type="submit" className="tuma-btn-fit">{saved ? "Saved" : "Save changes"}</Button>
          </form>
        </Card>

        <Card title="Test receiver">
          <div className="tuma-card-body tuma-stack">
            <p className="tuma-page-sub">
              Built-in echo sink — same job as <code className="tuma-code">scripts/webhook-echo.py</code>. The Tuma worker POSTs here (Docker hostname is expected). Watch payloads below; don’t open this URL in your browser.
            </p>
            <div className="tuma-code-row">
              <code className="tuma-code">{sinkURL || "…"}</code>
              <Button
                variant="secondary"
                disabled={!sinkURL}
                onClick={() => {
                  if (!sinkURL) return;
                  navigator.clipboard.writeText(sinkURL);
                  setCopiedSink(true);
                  setTimeout(() => setCopiedSink(false), 2000);
                }}
              >
                {copiedSink ? "Copied" : "Copy"}
              </Button>
            </div>
            <div className="tuma-field-row">
              <Button
                variant="secondary"
                disabled={!sinkURL || conn.destination_url === sinkURL}
                onClick={useTestReceiver}
              >
                {conn.destination_url === sinkURL ? "This connection uses the sink" : "Point this connection here"}
              </Button>
              {sinkFail ? (
                <Button variant="secondary" onClick={async () => { await api.sinkFix(); refreshSink(); }}>
                  Fix receiver (200)
                </Button>
              ) : (
                <Button variant="danger" onClick={async () => { await api.sinkBreak(); refreshSink(); }}>
                  Break receiver (502)
                </Button>
              )}
            </div>
            <div>
              <div className="tuma-field-caption">Last deliveries to the sink</div>
              {sinkEvents.length === 0 && (
                <p className="tuma-empty">Nothing received yet. Point the connection here, then send a Stripe/test event.</p>
              )}
              {[...sinkEvents].reverse().map((ev, i) => (
                <div key={`${ev.ts}-${i}`} className="tuma-sink-event">
                  <div className="tuma-sink-event__meta">
                    <span className="tuma-mono">{ev.status}</span>
                    <span className="tuma-muted">{ev.ts}</span>
                    {ev.delivery_id && <span className="tuma-mono">{ev.delivery_id}</span>}
                  </div>
                  <pre className="tuma-sink-event__body">{ev.body}</pre>
                </div>
              ))}
            </div>
          </div>
        </Card>

        <Card title="Recent deliveries">
          <div>
            {deliveries.length === 0 && (
              <p className="tuma-empty tuma-pad-16">No deliveries yet.</p>
            )}
            {deliveries.map((d) => (
              <div key={d.id} className="tuma-table-row tuma-table-row--deliveries">
                <span className="tuma-mono">#{d.attempt_number}</span>
                <span>{d.status}</span>
                <span className="tuma-mono">{d.response_code ?? "—"}</span>
                <span className="tuma-muted">{d.latency_ms != null ? `${d.latency_ms} ms` : "—"}</span>
              </div>
            ))}
          </div>
        </Card>
      </div>
    </div>
  );
}
