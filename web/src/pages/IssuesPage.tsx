import { useEffect, useState } from "react";
import { api, Issue } from "../lib/api";
import { usePageRestore } from "../lib/usePageRestore";
import { Button } from "../components/Button";
import { Card, Pill } from "../components/Layout";
import { PageHeader } from "../components/PageHeader";
import { AlertRulesPanel } from "../components/AlertRulesPanel";

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
      <PageHeader
        title="Issues"
        subtitle="Failed deliveries waiting for replay"
      />

      <div className="tuma-filter-bar">
        {(["open", "resolved", "all"] as const).map((key) => (
          <button
            key={key}
            type="button"
            onClick={() => { setFilter(key); setSelected([]); }}
            className={`tuma-filter-chip${filter === key ? " tuma-filter-chip--active" : ""}`}
          >
            {key === "open" ? `Open · ${issues.length}` : key.charAt(0).toUpperCase() + key.slice(1)}
          </button>
        ))}
        {selected.length > 0 && (
          <Button className="tuma-filter-bar__spacer" onClick={replayBulk}>
            Replay selected ({selected.length})
          </Button>
        )}
      </div>

      <Card>
        <div className="tuma-table-head tuma-table-head--issues">
          <button
            type="button"
            aria-label="Select all"
            onClick={() => setSelected(allSelected ? [] : issues.map((i) => i.id))}
            className={`tuma-checkbox${allSelected ? " tuma-checkbox--checked" : ""}`}
          />
          <span>Event</span>
          <span>Failure</span>
          <span>Destination</span>
          <span>Tries</span>
          <span>Status</span>
        </div>

        {issues.length === 0 && (
          <p className="tuma-empty tuma-empty--center">
            Nothing in the queue. Every event was delivered.
          </p>
        )}

        {issues.map((issue) => {
          const on = selected.includes(issue.id);
          return (
            <div
              key={issue.id}
              onClick={() => setDrawerId(issue.id)}
              className={`tuma-table-row tuma-table-row--issues tuma-table-row--clickable${on ? " tuma-table-row--selected" : ""}`}
            >
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setSelected((s) => (s.includes(issue.id) ? s.filter((x) => x !== issue.id) : [...s, issue.id]));
                }}
                className={`tuma-checkbox${on ? " tuma-checkbox--checked" : ""}`}
              />
              <span className="tuma-mono">
                {(issue as Issue & { provider_event_id?: string }).provider_event_id?.slice(0, 16) || issue.event_id.slice(0, 8)}
              </span>
              <span>{issue.reason}</span>
              <span className="tuma-muted">{issue.destination || "—"}</span>
              <span className="tuma-mono">{issue.attempts_exhausted}</span>
              <Pill status={issue.status === "open" ? "open" : "resolved"} />
            </div>
          );
        })}
      </Card>

      <AlertRulesPanel />

      {drawer && (
        <div className="tuma-drawer-overlay" onClick={() => setDrawerId(null)}>
          <div className="tuma-drawer" onClick={(e) => e.stopPropagation()}>
            <div className="tuma-drawer__head">
              <h2 className="tuma-drawer__title">Issue detail</h2>
              <Pill status={drawer.status === "open" ? "open" : "resolved"} />
            </div>
            <dl className="tuma-drawer__dl">
              <dt>Reason</dt>
              <dd>{drawer.reason}</dd>
              <dt>Destination</dt>
              <dd>{drawer.destination}</dd>
              <dt>Attempts</dt>
              <dd>{drawer.attempts_exhausted}</dd>
            </dl>
            <pre className="tuma-code tuma-drawer__payload">
              {drawerPayload || "Loading payload…"}
            </pre>
            <div className="tuma-drawer__actions">
              {drawer.status === "open" && (
                <Button onClick={() => replayOne(drawer.id)}>Replay event</Button>
              )}
              <Button variant="secondary" onClick={() => setDrawerId(null)}>Close</Button>
            </div>
          </div>
        </div>
      )}

      {toast && <div className="tuma-toast">{toast}</div>}
    </div>
  );
}
