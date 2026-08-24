import { ConnectionStats } from "../lib/types";

export function StatCell({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 2 }}>{label}</div>
      <div style={{ fontFamily: "var(--mono)", fontSize: 15, fontWeight: 500 }}>{value}</div>
    </div>
  );
}

export function ConnectionStatsRow({ stats }: { stats?: ConnectionStats }) {
  const s = stats ?? { delivered_24h: 0, p95_latency_ms: 0, failed_count: 0, status: "healthy" as const };
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 16, marginTop: 12 }}>
      <StatCell label="Delivered (24h)" value={s.delivered_24h.toLocaleString()} />
      <StatCell label="p95 latency" value={s.p95_latency_ms > 0 ? `${s.p95_latency_ms} ms` : "—"} />
      <StatCell label="Open issues" value={s.failed_count} />
    </div>
  );
}

export function HourlyBars({ data, label }: { data: { hour: string; count: number }[]; label: string }) {
  const max = Math.max(1, ...data.map((d) => d.count));
  return (
    <div>
      <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 10 }}>{label}</div>
      {data.length === 0 ? (
        <p style={{ margin: 0, fontSize: 13, color: "var(--muted)" }}>No activity in the last 24 hours.</p>
      ) : (
        <div style={{ display: "flex", alignItems: "flex-end", gap: 3, height: 80 }}>
          {data.map((d) => (
            <div
              key={d.hour}
              title={`${formatHour(d.hour)}: ${d.count}`}
              style={{
                flex: 1,
                minWidth: 4,
                height: `${Math.max(4, (d.count / max) * 100)}%`,
                background: "var(--accent)",
                borderRadius: "2px 2px 0 0",
                opacity: 0.85,
              }}
            />
          ))}
        </div>
      )}
      {data.length > 0 && (
        <div style={{ display: "flex", justifyContent: "space-between", marginTop: 6, fontSize: 10, color: "var(--muted)", fontFamily: "var(--mono)" }}>
          <span>{formatHour(data[0].hour)}</span>
          <span>{formatHour(data[data.length - 1].hour)}</span>
        </div>
      )}
    </div>
  );
}

function formatHour(iso: string) {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
