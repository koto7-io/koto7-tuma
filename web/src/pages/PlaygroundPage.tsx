import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, PlaygroundStatus, PlaygroundLogEvent } from "../lib/api";
import { Button } from "../components/Button";
import { Pill } from "../components/Layout";
import { FlowStrip, FlowNodeState } from "../components/FlowStrip";
import { ThemeToggle } from "../components/ThemeToggle";
import { TumaBadgerLoading, TumaLogo } from "../components/TumaLogo";
import "./PlaygroundPage.css";

const MARKETING_URL = "https://tuma.koto7.io";

const PROVIDERS = [
  { key: "stripe", label: "Stripe" },
  { key: "github", label: "GitHub" },
  { key: "easypost", label: "EasyPost" },
  { key: "internal", label: "Internal" },
];

const STEPS = [
  { title: "Send", body: "Watch a signed event deliver" },
  { title: "Break", body: "Simulate your app going down" },
  { title: "Send again", body: "Tuma retries, then opens an Issue" },
  { title: "Wait ~15s", body: "Retries exhaust into Issues" },
  { title: "Fix & Replay", body: "Restore destination, re-deliver" },
];

function deriveFlowState(
  status: PlaygroundStatus | null,
  waitingForIssue: boolean,
  busy: boolean,
): { provider: FlowNodeState; tuma: FlowNodeState; destination: FlowNodeState } {
  if (!status) {
    return { provider: "default", tuma: "active", destination: "default" };
  }

  if (status.fail_destination && waitingForIssue) {
    return { provider: "active", tuma: "retrying", destination: "failing" };
  }
  if (status.fail_destination && status.open_issues > 0) {
    return { provider: "active", tuma: "holding", destination: "failing" };
  }
  if (status.fail_destination) {
    return { provider: "active", tuma: "active", destination: "failing" };
  }
  if (status.open_issues > 0) {
    return { provider: "active", tuma: "holding", destination: "default" };
  }
  if (status.status === "delivering" || busy) {
    return { provider: "active", tuma: "active", destination: "active" };
  }
  return { provider: "active", tuma: "active", destination: "active" };
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

function deriveStepProgress(
  status: PlaygroundStatus | null,
  waitingForIssue: boolean,
  hasDestLog: boolean,
): number {
  if (!status) return 0;
  if (status.open_issues > 0 && !status.fail_destination) return 4;
  if (status.open_issues > 0 && status.fail_destination) return 3;
  if (status.fail_destination && waitingForIssue) return 2;
  if (status.fail_destination) return 1;
  if (hasDestLog) return 0;
  return 0;
}

function colorLogLine(line: string): string {
  if (/\b502\b|fail_mode|\bfailed\b/i.test(line)) return "pg-log-fail";
  if (/\b200\b|\bok\b/i.test(line)) return "pg-log-ok";
  return "";
}

function LogPane({ lines }: { lines: string[] }) {
  const ref = useRef<HTMLPreElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [lines]);

  if (!lines.length) {
    return (
      <pre ref={ref} className="pg-log">
        <span className="pg-log-empty">— waiting for events —</span>
      </pre>
    );
  }

  return (
    <pre ref={ref} className="pg-log">
      {lines.map((line, i) => (
        <span key={i} className={colorLogLine(line)}>
          {line}
          {i < lines.length - 1 ? "\n" : ""}
        </span>
      ))}
    </pre>
  );
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
  const activeStep = deriveStepProgress(status, waitingForIssue, destLog.length > 0);
  const hint = statusHint(status, waitingForIssue);
  const pillStatus = status?.status === "delivering" ? "healthy" : (status?.status ?? "healthy");
  const flow = deriveFlowState(status, waitingForIssue, busy);

  const stepStates = useMemo(
    () =>
      STEPS.map((_, i) => {
        if (i < activeStep) return "done" as const;
        if (i === activeStep) return "active" as const;
        return "pending" as const;
      }),
    [activeStep],
  );

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
    es.onerror = () => es.close();
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
      if (action === "replay") setWaitingForIssue(false);
    } catch (e) {
      setError(friendlyPlaygroundError(e));
    } finally {
      setBusy(false);
    }
  }

  if (ready === "loading" || ready === "bootstrapping") {
    return (
      <div className="pg-loading">
        <div className="pg-loading-inner">
          <TumaBadgerLoading size={96} />
          <span>Starting your isolated demo session…</span>
        </div>
      </div>
    );
  }

  if (ready === "disabled") {
    return (
      <div className="pg-unavailable">
        <div className="pg-unavailable-inner">
          <h1>Playground temporarily unavailable</h1>
          <p>The live demo is starting up or briefly offline. Try again in a minute.</p>
          <div className="pg-unavailable-links">
            <a href="/playground" className="pg-back">Try again</a>
            <a href={MARKETING_URL} className="pg-back">Back to tuma.koto7.io ↗</a>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="pg-root">
      <div className="pg-shell">
        <header className="pg-header">
          <div className="pg-header-brand">
            <TumaLogo size={36} wordmarkText="tuma" large />
            <div className="pg-header-sub">playground · Simulated provider · Demo destination · Real Tuma stack</div>
          </div>
          <div className="pg-header-actions">
            <ThemeToggle compact />
            <a href={MARKETING_URL} className="pg-back">tuma.koto7.io ↗</a>
          </div>
        </header>

        <FlowStrip provider={flow.provider} tuma={flow.tuma} destination={flow.destination} />

        <section className="pg-steps">
          <div className="pg-steps-label">Try the failure story</div>
          <ol className="pg-step-list">
            {STEPS.map((step, i) => (
              <li key={step.title} className={`pg-step ${stepStates[i]}`}>
                <span className="pg-step-num">{stepStates[i] === "done" ? "✓" : i + 1}</span>
                <span>
                  <strong>{step.title}</strong>
                  <br />
                  {step.body}
                </span>
              </li>
            ))}
          </ol>
        </section>

        {error && <div className="pg-alert tuma-alert tuma-alert--error">{error}</div>}

        <div className="pg-grid">
          <div className="pg-pane">
            <div className="pg-pane-head">
              <span className="pg-pane-title">Live status</span>
              <Pill status={pillStatus} />
            </div>
            <div className="pg-pane-body">
              {hint && (
                <p className={`pg-hint ${waitingForIssue ? "waiting" : ""}`}>{hint}</p>
              )}
              <div className="pg-stats">
                <div className="pg-stat">
                  <div className="pg-stat-label">Delivered (24h)</div>
                  <div className="pg-stat-value">{status?.delivered_24h ?? 0}</div>
                </div>
                <div className="pg-stat">
                  <div className="pg-stat-label">Open issues</div>
                  <div className={`pg-stat-value${openIssues > 0 ? " pg-stat-value--alert" : ""}`}>
                    {openIssues}
                  </div>
                </div>
              </div>
              {inboundURL && (
                <div className="pg-url-box">
                  Your session URL
                  <div className="pg-url">{inboundURL}</div>
                </div>
              )}
              <div className="pg-actions">
                <Button variant="danger" disabled={busy || broken} onClick={() => control("break")}>
                  Break
                </Button>
                <Button variant="secondary" disabled={busy || !broken} onClick={() => control("fix")}>
                  Fix
                </Button>
                <Button disabled={busy || !canReplay} onClick={() => control("replay")}>
                  Replay
                </Button>
              </div>
            </div>
          </div>

          <div className="pg-pane">
            <div className="pg-pane-head">
              <span className="pg-pane-title">Provider simulator</span>
            </div>
            <div className="pg-pane-body pg-pane-body--sim">
              <label className="pg-field-label">Provider</label>
              <select
                className="tuma-input pg-provider-select"
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
              >
                {PROVIDERS.map((p) => (
                  <option key={p.key} value={p.key}>{p.label}</option>
                ))}
              </select>
              <div className="pg-actions pg-actions--flush">
                <Button disabled={busy} onClick={() => simulate(1)}>Send 1</Button>
                <Button variant="secondary" disabled={busy} onClick={() => simulate(5)}>Send 5</Button>
                <Button variant="ghost" disabled={busy || !lastDupId.current} onClick={() => simulate(1, true)}>
                  Duplicate
                </Button>
              </div>
            </div>
            <LogPane lines={simLog} />
          </div>

          <div className="pg-pane">
            <div className="pg-pane-head">
              <span className="pg-pane-title">Destination log</span>
              {broken && <Pill status="failing" />}
            </div>
            <LogPane lines={destLog} />
          </div>
        </div>
      </div>
    </div>
  );
}
