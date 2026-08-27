import { useEffect, useState } from "react";
import { api, Issue } from "../lib/api";
import { usePageRestore } from "../lib/usePageRestore";
import { Card, Pill } from "../components/Layout";
import { buttonPrimary, buttonSecondary } from "./LoginPage";

export function IssuesPage() {
  const [filter, setFilter] = useState("open");
  const [issues, setIssues] = useState<Issue[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [drawerId, setDrawerId] = useState<string | null>(null);
  const [drawerPayload, setDrawerPayload] = useState("");
  const [toast, setToast] = useState<string | null>(null);

  function load(status = filter) {
    api.listIssues(status).then((r) => setIssues(r.issues ?? [])).catch(console.error);
  }

  usePageRestore(() => {
    load(filter);
  }, [filter]);

  useEffect(() => {
    if (!drawerId) return;
    api.getIssue(drawerId).then((r) => setDrawerPayload(r.payload)).catch(console.error);
  }, [drawerId]);

  function flash(msg: string) {
    setToast(msg);
    setTimeout(() => setToast(null), 2600);
  }

  async function replayOne(id: string) {
    await api.replayIssue(id);
    flash("Replay started");
  }

  async function replayBulk() {
    const r = await api.replayBulk(selected);
    flash(`Started ${r.started} replays`);
    setSelected([]);
  }

  const drawer = issues.find((i) => i.id === drawerId);
  const allSelected = issues.length > 0 && selected.length === issues.length;

  return (
    <div>
      <h1 style={{ margin: "0 0 6px", fontSize: 24 }}>Issues</h1>
      <p style={{ margin: "0 0 20px", color: "var(--muted)", fontSize: 14 }}>
        Failed deliveries waiting for replay
      </p>

      <div style={{ display: "flex", gap: 8, marginBottom: 16, flexWrap: "wrap" }}>
        {(["open", "resolved", "all"] as const).map((key) => (
          <button
            key={key}
            onClick={() => { setFilter(key); setSelected([]); }}
            style={{
              fontSize: 12.5,
              fontWeight: 500,
              padding: "5px 12px",
              borderRadius: 100,
              cursor: "pointer",
              border: `1px solid ${filter === key ? "var(--ink)" : "var(--border)"}`,
              background: filter === key ? "var(--ink)" : "var(--surface)",
              color: filter === key ? "var(--bg)" : "var(--muted)",
            }}
          >
            {key === "open" ? `Open · ${issues.length}` : key.charAt(0).toUpperCase() + key.slice(1)}
          </button>
        ))}
        {selected.length > 0 && (
          <button style={{ ...buttonPrimary, marginLeft: "auto" }} onClick={replayBulk}>
            Replay selected ({selected.length})
          </button>
        )}
      </div>

      <Card>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "34px minmax(88px,132px) minmax(0,1.5fr) minmax(0,1fr) 62px 80px",
            gap: 12,
            padding: "10px 16px",
            borderBottom: "1px solid var(--border-light)",
            fontSize: 11,
            fontWeight: 600,
            color: "var(--muted)",
            textTransform: "uppercase",
            letterSpacing: "0.04em",
          }}
        >
          <button
            aria-label="Select all"
            onClick={() => setSelected(allSelected ? [] : issues.map((i) => i.id))}
            style={{
              width: 15,
              height: 15,
              borderRadius: 4,
              border: `1px solid ${allSelected ? "var(--ink)" : "var(--border)"}`,
              background: allSelected ? "var(--ink)" : "var(--surface)",
              cursor: "pointer",
            }}
          />
          <span>Event</span>
          <span>Failure</span>
          <span>Destination</span>
          <span>Tries</span>
          <span>Status</span>
        </div>

        {issues.length === 0 && (
          <p style={{ padding: 24, textAlign: "center", color: "var(--muted)", margin: 0 }}>
            Nothing in the queue. Every event was delivered.
          </p>
        )}

        {issues.map((issue) => {
          const on = selected.includes(issue.id);
          return (
            <div
              key={issue.id}
              onClick={() => setDrawerId(issue.id)}
              style={{
                display: "grid",
                gridTemplateColumns: "34px minmax(88px,132px) minmax(0,1.5fr) minmax(0,1fr) 62px 80px",
                gap: 12,
                alignItems: "center",
                padding: "12px 16px",
                borderBottom: "1px solid var(--border-light)",
                cursor: "pointer",
                background: on ? "var(--row-selected)" : "transparent",
              }}
            >
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setSelected((s) => (s.includes(issue.id) ? s.filter((x) => x !== issue.id) : [...s, issue.id]));
                }}
                style={{
                  width: 15,
                  height: 15,
                  borderRadius: 4,
                  border: `1px solid ${on ? "var(--ink)" : "var(--border)"}`,
                  background: on ? "var(--ink)" : "var(--surface)",
                  cursor: "pointer",
                }}
              />
              <span style={{ fontFamily: "var(--mono)", fontSize: 12 }}>
                {(issue as Issue & { provider_event_id?: string }).provider_event_id?.slice(0, 16) || issue.event_id.slice(0, 8)}
              </span>
              <span style={{ fontSize: 13 }}>{issue.reason}</span>
              <span style={{ fontSize: 12, color: "var(--muted)" }}>{issue.destination || "—"}</span>
              <span style={{ fontFamily: "var(--mono)", fontSize: 12 }}>{issue.attempts_exhausted}</span>
              <Pill status={issue.status === "open" ? "open" : "resolved"} />
            </div>
          );
        })}
      </Card>

      {drawer && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            background: "var(--overlay)",
            display: "flex",
            justifyContent: "flex-end",
            zIndex: 50,
          }}
          onClick={() => setDrawerId(null)}
        >
          <div
            onClick={(e) => e.stopPropagation()}
            style={{
              width: 420,
              maxWidth: "100%",
              background: "var(--surface)",
              borderLeft: "1px solid var(--border)",
              padding: 20,
              overflow: "auto",
              animation: "tuma-slide 0.2s ease",
            }}
          >
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <h2 style={{ margin: 0, fontSize: 18 }}>Issue detail</h2>
              <Pill status={drawer.status === "open" ? "open" : "resolved"} />
            </div>
            <dl style={{ fontSize: 13, marginTop: 16 }}>
              <dt style={{ color: "var(--muted)" }}>Reason</dt>
              <dd>{drawer.reason}</dd>
              <dt style={{ color: "var(--muted)", marginTop: 10 }}>Destination</dt>
              <dd>{drawer.destination}</dd>
              <dt style={{ color: "var(--muted)", marginTop: 10 }}>Attempts</dt>
              <dd>{drawer.attempts_exhausted}</dd>
            </dl>
            <pre
              style={{
                background: "var(--code-bg)",
                border: "1px solid var(--border)",
                borderRadius: 8,
                padding: 12,
                fontFamily: "var(--mono)",
                fontSize: 11,
                overflow: "auto",
                maxHeight: 240,
              }}
            >
              {drawerPayload || "Loading payload…"}
            </pre>
            {drawer.status === "open" && (
              <button style={{ ...buttonPrimary, marginTop: 16 }} onClick={() => replayOne(drawer.id)}>
                Replay event
              </button>
            )}
            <button style={{ ...buttonSecondary, marginTop: 8, marginLeft: 8 }} onClick={() => setDrawerId(null)}>
              Close
            </button>
          </div>
        </div>
      )}

      {toast && (
        <div
          style={{
            position: "fixed",
            bottom: 20,
            right: 20,
            background: "var(--ink)",
            color: "var(--bg)",
            padding: "10px 14px",
            borderRadius: 8,
            fontSize: 13,
          }}
        >
          {toast}
        </div>
      )}
    </div>
  );
}
