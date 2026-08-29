import { useCallback, useEffect, useRef, useState } from "react";
import { api, PlaygroundStatus, PlaygroundLogEvent } from "../lib/api";
import { buttonPrimary, buttonSecondary, inputStyle } from "./LoginPage";

const MARKETING_URL = "https://tuma.koto7.io";

const PROVIDERS = [
  { key: "stripe", label: "Stripe" },
  { key: "github", label: "GitHub" },
  { key: "easypost", label: "EasyPost" },
  { key: "internal", label: "Internal" },
];

const STEPS = [
  "Send an event — watch it land in the destination log",
  "Break — simulates your app going down",
  "Send again — Tuma retries, then opens an Issue (~15s)",
  "Fix — your app is healthy again",
  "Replay — Tuma re-delivers the failed event",
];

const paneStyle: React.CSSProperties = {
  background: "var(--surface)",
  border: "1px solid var(--border)",
  borderRadius: 8,
  display: "flex",
  flexDirection: "column",
  minHeight: 320,
  overflow: "hidden",
};

const logStyle: React.CSSProperties = {
  flex: 1,
  margin: 0,
  padding: 12,
  fontFamily: "var(--mono)",
  fontSize: 12,
  lineHeight: 1.5,
  overflow: "auto",
  background: "var(--code-bg)",
  color: "var(--ink)",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
};

function statusColor(status: string): string {
  if (status === "failing") return "var(--red)";
  if (status === "degraded") return "var(--amber)";
  return "var(--green)";
}

function LogPane({ lines }: { lines: string[] }) {
  const ref = useRef<HTMLPreElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [lines]);
  return (
    <pre ref={ref} style={logStyle}>
      {lines.length ? lines.join("\n") : "— waiting for events —"}
    </pre>
  );
}

function friendlyPlaygroundError(e: unknown): string {
  const msg = String(e).toLowerCase();
  if (msg.includes("no_open_issue") || msg.includes("no failed event")) {
    return "No failed event yet. Break → Send → wait ~15s for retries → Fix → Replay.";
  }
  if (msg.includes("fix_first") || msg.includes("fix the destination")) {
    return "Fix the destination first, then Replay.";
  }
  if (msg.includes("429") || msg.includes("rate limit")) {
    return "Too many requests — wait a minute and try again.";
  }
  if (msg.includes("401") || msg.includes("unauthorized") || msg.includes("session")) {
    return "Session expired. Refresh the page to start a new demo.";
  }
  if (msg.includes("404") || msg.includes("not found")) {
    return "Playground unavailable. Refresh the page and try again.";
  }
  return "Something went wrong. Try again or refresh the page.";
}

function statusHint(status: PlaygroundStatus | null, waitingForIssue: boolean): string {
  if (!status) return "";
  if (status.fail_destination && waitingForIssue && status.open_issues === 0) {
    return "Retries running — an Issue opens in ~15 seconds…";
  }
  if (status.fail_destination && status.open_issues > 0) {
    return "Issue open. Fix the destination, then Replay.";
  }
  if (status.fail_destination) {
    return "Destination broken. Send an event to trigger retries.";
  }
  if (status.open_issues > 0) {
    return "Issue waiting — click Fix, then Replay.";
  }
  return "Healthy. Try Break → Send to see the failure story.";
}

export function PlaygroundPage() {
  const [ready, setReady] = useState<"loading" | "disabled" | "bootstrapping" | "ready">("loading");
  const [inboundURL, setInboundURL] = useState("");
  const [status, setStatus] = useState<PlaygroundStatus | null>(null);
  const [provider, setProvider] = useState("stripe");
  const [simLog, setSimLog] = useState<string[]>([]);
  const [destLog, setDestLog] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [waitingForIssue, setWaitingForIssue] = useState(false);
  const lastDupId = useRef<string>("");

  const openIssues = status?.open_issues ?? 0;
  const broken = status?.fail_destination ?? false;
  const canReplay = openIssues > 0 && !broken;

  const appendSim = useCallback((ev: PlaygroundLogEvent) => {
    const line = ev.line || JSON.stringify(ev);
    setSimLog((l) => [...l.slice(-99), `[${ev.ts?.slice(11, 19) || ""}] ${line}`]);
  }, []);

  const appendDest = useCallback((ev: PlaygroundLogEvent) => {
    let line = `[${ev.ts?.slice(11, 19) || ""}] ${ev.line}`;
    if (ev.delivery_id) line += `\n  X-Tuma-Delivery-Id: ${ev.delivery_id}`;
    if (ev.body_preview) line += `\n  ${ev.body_preview}`;
    setDestLog((l) => [...l.slice(-49), line]);
  }, []);

  useEffect(() => {
    api.getConfig()
      .then((c) => {
        if (!c.demo_mode) {
          setReady("disabled");
          return;
        }
        setReady("bootstrapping");
        return api.playgroundBootstrap();
      })
      .then((b) => {
        if (!b) return;
        setInboundURL(b.inbound_url);
        setReady("ready");
      })
      .catch(() => {
        setReady("disabled");
      });
  }, []);

  useEffect(() => {
    if (ready !== "ready") return;
    const poll = () => {
      api.playgroundStatus().then((s) => {
        setStatus(s);
        if (s.open_issues > 0) setWaitingForIssue(false);
      }).catch(() => {});
    };
    poll();
    const id = setInterval(poll, 3000);
    return () => clearInterval(id);
  }, [ready]);

  useEffect(() => {
    if (ready !== "ready") return;
    const es = new EventSource("/api/playground/stream", { withCredentials: true });
    es.addEventListener("sim", (e) => {
      try {
        appendSim(JSON.parse(e.data));
      } catch {
        /* ignore */
      }
    });
    es.addEventListener("dest", (e) => {
      try {
        appendDest(JSON.parse(e.data));
      } catch {
        /* ignore */
      }
    });
    es.onerror = () => {
      es.close();
    };
    return () => es.close();
  }, [ready, appendSim, appendDest]);

  async function simulate(count: number, duplicate = false) {
    setBusy(true);
    setError("");
    try {
      const body: { provider: string; count: number; duplicate_event_id?: string } = {
        provider,
        count,
      };
      if (duplicate && lastDupId.current) {
        body.duplicate_event_id = lastDupId.current;
      }
      const res = await api.playgroundSimulate(body);
      if (res.results?.[0]?.event_id) {
        lastDupId.current = res.results[0].event_id;
      }
      if (broken) setWaitingForIssue(true);
    } catch (e) {
      setError(friendlyPlaygroundError(e));
    } finally {
      setBusy(false);
    }
  }

  async function control(action: "break" | "fix" | "replay") {
    setBusy(true);
    setError("");
    try {
      if (action === "break") await api.playgroundBreak();
      else if (action === "fix") await api.playgroundFix();
      else await api.playgroundReplay();
      if (action === "replay") {
        setWaitingForIssue(false);
      }
    } catch (e) {
      setError(friendlyPlaygroundError(e));
    } finally {
      setBusy(false);
    }
  }

  if (ready === "loading" || ready === "bootstrapping") {
    return (
      <div style={{ padding: 40, color: "var(--muted)" }}>
        Starting playground session…
      </div>
    );
  }

  if (ready === "disabled") {
    return (
      <div style={{ minHeight: "100vh", background: "var(--bg)", padding: 40 }}>
        <h1 style={{ fontFamily: "var(--mono)", fontSize: 20 }}>Playground temporarily unavailable</h1>
        <p style={{ color: "var(--muted)", maxWidth: 520, lineHeight: 1.6 }}>
          The live demo is starting up or briefly offline. Try again in a minute, or head back to the Tuma site.
        </p>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginTop: 24 }}>
          <a href="/playground" style={{ fontSize: 14 }}>Try again</a>
          <a href={MARKETING_URL} style={{ fontSize: 14 }}>Back to tuma.koto7.io ↗</a>
        </div>
      </div>
    );
  }

  const pill = status?.status || "delivering";
  const hint = statusHint(status, waitingForIssue);

  return (
    <div style={{ minHeight: "100vh", background: "var(--bg)" }}>
      <header
        style={{
          padding: "16px 24px",
          borderBottom: "1px solid var(--border)",
          background: "var(--surface)",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          flexWrap: "wrap",
          gap: 12,
        }}
      >
        <div>
          <div style={{ fontFamily: "var(--mono)", fontWeight: 600, fontSize: 18 }}>tuma playground</div>
          <div style={{ fontSize: 13, color: "var(--muted)", marginTop: 4 }}>
            Simulated provider · Demo destination · Real Tuma stack
          </div>
        </div>
        <a href={MARKETING_URL} style={{ fontSize: 14 }}>
          Back to tuma.koto7.io ↗
        </a>
      </header>

      <div style={{ padding: "12px 24px", borderBottom: "1px solid var(--border)", background: "var(--surface)" }}>
        <div style={{ fontSize: 12, fontWeight: 600, color: "var(--muted)", marginBottom: 8 }}>TRY THE FAILURE STORY</div>
        <ol style={{ margin: 0, paddingLeft: 20, fontSize: 13, lineHeight: 1.7, color: "var(--ink)" }}>
          {STEPS.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
      </div>

      {error && (
        <div style={{ padding: "12px 24px", color: "var(--red)", fontSize: 14 }}>{error}</div>
      )}

      <div
        style={{
          padding: 24,
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
          gap: 16,
        }}
      >
        <div style={paneStyle}>
          <div style={{ padding: "12px 14px", borderBottom: "1px solid var(--border)", fontWeight: 600, fontSize: 13 }}>
            LIVE STATUS
          </div>
          <div style={{ padding: 14, flex: 1 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 8 }}>
              <span
                style={{
                  width: 10,
                  height: 10,
                  borderRadius: "50%",
                  background: statusColor(pill),
                }}
              />
              <span style={{ fontWeight: 600, textTransform: "capitalize" }}>{pill}</span>
              {broken && (
                <span style={{ fontSize: 12, color: "var(--red)" }}>(destination broken)</span>
              )}
            </div>
            {hint && (
              <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 12px", lineHeight: 1.5 }}>{hint}</p>
            )}
            <div style={{ fontSize: 14, lineHeight: 1.8 }}>
              <div>Delivered (24h): <strong>{status?.delivered_24h ?? 0}</strong></div>
              <div>Open issues: <strong>{openIssues}</strong></div>
            </div>
            {inboundURL && (
              <div style={{ marginTop: 16, fontSize: 12, color: "var(--muted)" }}>
                Your isolated inbound URL (this session only)
                <div style={{ fontFamily: "var(--mono)", fontSize: 11, wordBreak: "break-all", marginTop: 4, color: "var(--ink)" }}>
                  {inboundURL}
                </div>
              </div>
            )}
            <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 20 }}>
              <button style={buttonSecondary} disabled={busy || broken} onClick={() => control("break")} title={broken ? "Already broken" : "Simulate destination failure"}>
                Break
              </button>
              <button style={buttonSecondary} disabled={busy || !broken} onClick={() => control("fix")} title={!broken ? "Destination is healthy" : "Restore destination"}>
                Fix
              </button>
              <button
                style={buttonPrimary}
                disabled={busy || !canReplay}
                onClick={() => control("replay")}
                title={!canReplay ? "Need an open Issue and a fixed destination" : "Re-deliver the failed event"}
              >
                Replay
              </button>
            </div>
          </div>
        </div>

        <div style={paneStyle}>
          <div style={{ padding: "12px 14px", borderBottom: "1px solid var(--border)", fontWeight: 600, fontSize: 13 }}>
            PROVIDER SIMULATOR
          </div>
          <div style={{ padding: 14 }}>
            <label style={{ fontSize: 12, color: "var(--muted)" }}>Provider</label>
            <select
              value={provider}
              onChange={(e) => setProvider(e.target.value)}
              style={{ ...inputStyle, width: "100%", marginTop: 4, marginBottom: 12 }}
            >
              {PROVIDERS.map((p) => (
                <option key={p.key} value={p.key}>{p.label}</option>
              ))}
            </select>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
              <button style={buttonPrimary} disabled={busy} onClick={() => simulate(1)}>
                Send 1
              </button>
              <button style={buttonSecondary} disabled={busy} onClick={() => simulate(5)}>
                Send 5
              </button>
              <button style={buttonSecondary} disabled={busy || !lastDupId.current} onClick={() => simulate(1, true)}>
                Send duplicate
              </button>
            </div>
          </div>
          <LogPane lines={simLog} />
        </div>

        <div style={paneStyle}>
          <div style={{ padding: "12px 14px", borderBottom: "1px solid var(--border)", fontWeight: 600, fontSize: 13 }}>
            DESTINATION LOG
          </div>
          <LogPane lines={destLog} />
        </div>
      </div>
    </div>
  );
}
