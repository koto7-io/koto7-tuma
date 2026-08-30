import { ConnectionStats } from "../lib/types";

export function StatCell({ label, value }: { label: string; value: string | number }) {
  return (
    <div>
      <div className="tuma-stat-cell__label">{label}</div>
      <div className="tuma-stat-cell__value">{value}</div>
    </div>
  );
}

export function ConnectionStatsRow({ stats }: { stats?: ConnectionStats }) {
  const s = stats ?? { delivered_24h: 0, p95_latency_ms: 0, failed_count: 0, status: "healthy" as const };
  return (
    <div className="tuma-stats-row">
      <StatCell label="Delivered (24h)" value={s.delivered_24h.toLocaleString()} />
      <StatCell label="p95 latency" value={s.p95_latency_ms > 0 ? `${s.p95_latency_ms} ms` : "—"} />
      <StatCell label="Open issues" value={s.failed_count} />
    </div>
  );
}

export function HourlyBars({ data, label }: { data: { hour: string; count: number }[] | null | undefined; label: string }) {
  const rows = data ?? [];
  const max = Math.max(1, ...rows.map((d) => d.count));
  return (
    <div>
      <div className="tuma-hourly__label">{label}</div>
      {rows.length === 0 ? (
        <p className="tuma-empty">No activity in the last 24 hours.</p>
      ) : (
        <div className="tuma-hourly__bars">
          {rows.map((d) => (
            <div
              key={d.hour}
              title={`${formatHour(d.hour)}: ${d.count}`}
              className="tuma-hourly__bar"
              style={{ height: `${Math.max(4, (d.count / max) * 100)}%` }}
            />
          ))}
        </div>
      )}
      {rows.length > 0 && (
        <div className="tuma-hourly__axis">
          <span>{formatHour(rows[0].hour)}</span>
          <span>{formatHour(rows[rows.length - 1].hour)}</span>
        </div>
      )}
    </div>
  );
}

function formatHour(iso: string) {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
