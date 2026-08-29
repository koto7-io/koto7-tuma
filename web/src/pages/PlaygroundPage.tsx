import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api, PlaygroundStatus, PlaygroundLogEvent } from "../lib/api";
import { buttonPrimary, buttonSecondary, inputStyle } from "./LoginPage";

const PROVIDERS = [
  { key: "stripe", label: "Stripe" },
  { key: "github", label: "GitHub" },
  { key: "easypost", label: "EasyPost" },
  { key: "internal", label: "Internal" },
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

export function PlaygroundPage() {
  const [ready, setReady] = useState<"loading" | "disabled" | "bootstrapping" | "ready">("loading");
  const [inboundURL, setInboundURL] = useState("");
  const [status, setStatus] = useState<PlaygroundStatus | null>(null);
  const [provider, setProvider] = useState("stripe");
  const [simLog, setSimLog] = useState<string[]>([]);
  const [destLog, setDestLog] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const lastDupId = useRef<string>("");

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
      .catch((e) => {
        setError(String(e));
        setReady("disabled");
      });
  }, []);

  useEffect(() => {
    if (ready !== "ready") return;
    const poll = () => {
      api.playgroundStatus().then(setStatus).catch(() => {});
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
    } catch (e) {
      setError(String(e));
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
    } catch (e) {
      setError(String(e));
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
        <h1 style={{ fontFamily: "var(--mono)", fontSize: 20 }}>Playground unavailable</h1>
        <p style={{ color: "var(--muted)", maxWidth: 520, lineHeight: 1.6 }}>
          Demo mode is not enabled on this server. Set{" "}
          <code>TUMA_DEMO_MODE=true</code> and{" "}
          <code>TUMA_PUBLIC_BASE_URL=https://tuma-demo.koto7.dev</code> in{" "}
          <code>deploy/.env</code>, then restart <code>tuma-api</code>.
        </p>
        {error && <p style={{ color: "var(--red)", marginTop: 16 }}>{error}</p>}
        <Link to="/" style={{ display: "inline-block", marginTop: 20 }}>
          Open console ↗
        </Link>
      </div>
    );
  }

  const pill = status?.status || "delivering";

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
            Simulated Stripe · Demo destination · Real Tuma stack
          </div>
        </div>
        <Link to="/" style={{ fontSize: 14 }}>
          Open console ↗
        </Link>
      </header>

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
        {/* Live status */}
        <div style={paneStyle}>
          <div style={{ padding: "12px 14px", borderBottom: "1px solid var(--border)", fontWeight: 600, fontSize: 13 }}>
            LIVE STATUS
          </div>
          <div style={{ padding: 14, flex: 1 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 16 }}>
              <span
                style={{
                  width: 10,
                  height: 10,
                  borderRadius: "50%",
                  background: statusColor(pill),
                }}
              />
              <span style={{ fontWeight: 600, textTransform: "capitalize" }}>{pill}</span>
              {status?.fail_destination && (
                <span style={{ fontSize: 12, color: "var(--red)" }}>(destination broken)</span>
              )}
            </div>
            <div style={{ fontSize: 14, lineHeight: 1.8 }}>
              <div>Delivered (24h): <strong>{status?.delivered_24h ?? 0}</strong></div>
              <div>Open issues: <strong>{status?.open_issues ?? 0}</strong></div>
            </div>
            {inboundURL && (
              <div style={{ marginTop: 16, fontSize: 12, color: "var(--muted)" }}>
                Inbound URL
                <div style={{ fontFamily: "var(--mono)", fontSize: 11, wordBreak: "break-all", marginTop: 4, color: "var(--ink)" }}>
                  {inboundURL}
                </div>
              </div>
            )}
            <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginTop: 20 }}>
              <button style={buttonSecondary} disabled={busy} onClick={() => control("break")}>
                Break
              </button>
              <button style={buttonSecondary} disabled={busy} onClick={() => control("fix")}>
                Fix
              </button>
              <button style={buttonPrimary} disabled={busy} onClick={() => control("replay")}>
                Replay
              </button>
            </div>
          </div>
        </div>

        {/* Provider simulator */}
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

        {/* Destination log */}
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
