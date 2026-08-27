import { FormEvent, useState } from "react";
import { Link } from "react-router-dom";
import { api, Connection } from "../lib/api";
import { usePageRestore } from "../lib/usePageRestore";
import { Card, Pill } from "../components/Layout";
import { ConnectionStatsRow } from "../components/StatsRow";
import { buttonPrimary, buttonSecondary, inputStyle } from "./LoginPage";

const SOURCES = [
  { key: "stripe", name: "Stripe", sub: "Events API" },
  { key: "github", name: "GitHub", sub: "Repo webhooks" },
  { key: "generic_hmac", name: "Custom", sub: "HMAC signed" },
  { key: "internal", name: "Internal", sub: "Trusted network" },
];

export function ConnectionsPage() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [wizard, setWizard] = useState(false);
  const [step, setStep] = useState(0);
  const [source, setSource] = useState("stripe");
  const [name, setName] = useState("");
  const [dest, setDest] = useState("");
  const [signingSecret, setSigningSecret] = useState("");
  const [created, setCreated] = useState<{ connection: Connection; signing_secret: string } | null>(null);
  const [copied, setCopied] = useState(false);

  usePageRestore(() => {
    api.listConnections().then((r) => setConnections(r.connections)).catch(console.error);
  }, []);

  async function create(e: FormEvent) {
    e.preventDefault();
    const res = await api.createConnection({
      name,
      source_type: source,
      destination_url: dest,
      signing_secret: signingSecret || undefined,
    });
    setCreated(res);
    setConnections((c) => [res.connection, ...c]);
    setStep(2);
  }

  function copy(text: string) {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 20 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 24 }}>Connections</h1>
          <p style={{ margin: "6px 0 0", color: "var(--muted)", fontSize: 14 }}>
            Inbound webhooks → reliable delivery to your apps
          </p>
        </div>
        <button style={buttonPrimary} onClick={() => { setWizard(true); setStep(0); setCreated(null); }}>
          New connection
        </button>
      </div>

      {wizard && (
        <Card title="New connection">
          <div style={{ padding: 16 }}>
            {step === 0 && (
              <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 10 }}>
                {SOURCES.map((s) => (
                  <button
                    key={s.key}
                    type="button"
                    onClick={() => { setSource(s.key); setStep(1); }}
                    style={{
                      textAlign: "left",
                      padding: 14,
                      borderRadius: 8,
                      border: `1px solid ${source === s.key ? "var(--ink)" : "var(--border)"}`,
                      background: "var(--input-bg)",
                      cursor: "pointer",
                    }}
                  >
                    <div style={{ fontWeight: 600 }}>{s.name}</div>
                    <div style={{ fontSize: 12, color: "var(--muted)" }}>{s.sub}</div>
                  </button>
                ))}
              </div>
            )}
            {step === 1 && (
              <form onSubmit={create}>
                <label style={{ fontSize: 13 }}>Name</label>
                <input style={{ ...inputStyle, marginTop: 6, marginBottom: 14 }} value={name} onChange={(e) => setName(e.target.value)} required />
                <label style={{ fontSize: 13 }}>Destination URL</label>
                <input style={{ ...inputStyle, marginTop: 6, marginBottom: 14 }} value={dest} onChange={(e) => setDest(e.target.value)} placeholder="https://api.example.com/webhooks" required />
                {source !== "internal" && (
                  <>
                    <label style={{ fontSize: 13 }}>Signing secret</label>
                    <input style={{ ...inputStyle, marginTop: 6, marginBottom: 14 }} value={signingSecret} onChange={(e) => setSigningSecret(e.target.value)} placeholder={source === "stripe" ? "whsec_..." : "From provider dashboard"} />
                  </>
                )}
                <div style={{ display: "flex", gap: 8 }}>
                  <button type="button" style={buttonSecondary} onClick={() => setStep(0)}>Back</button>
                  <button type="submit" style={buttonPrimary}>Create</button>
                </div>
              </form>
            )}
            {step === 2 && created && (
              <div>
                <p style={{ color: "var(--muted)", fontSize: 14 }}>
                  Paste this into your {source} endpoint settings. Nothing else changes.
                </p>
                <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 6 }}>Webhook URL</div>
                <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
                  <code style={{ flex: 1, fontFamily: "var(--mono)", fontSize: 12, background: "var(--code-bg)", border: "1px solid var(--border)", borderRadius: 6, padding: 10 }}>
                    {created.connection.inbound_url}
                  </code>
                  <button style={buttonSecondary} onClick={() => copy(created.connection.inbound_url)}>
                    {copied ? "Copied" : "Copy"}
                  </button>
                </div>
                {created.signing_secret && (
                  <>
                    <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 6 }}>Signing secret</div>
                    <code style={{ display: "block", fontFamily: "var(--mono)", fontSize: 12, background: "var(--code-bg)", border: "1px solid var(--border)", borderRadius: 6, padding: 10 }}>
                      {created.signing_secret}
                    </code>
                  </>
                )}
                <button style={{ ...buttonPrimary, marginTop: 16 }} onClick={() => setWizard(false)}>Done</button>
              </div>
            )}
          </div>
        </Card>
      )}

      <div style={{ display: "grid", gap: 12, marginTop: wizard ? 16 : 0 }}>
        {connections.map((c) => (
          <Link key={c.id} to={`/connections/${c.id}`} style={{ textDecoration: "none", color: "inherit" }}>
            <Card>
              <div style={{ padding: "14px 16px" }}>
                <div style={{ display: "grid", gridTemplateColumns: "1fr auto", gap: 12, alignItems: "start" }}>
                  <div>
                    <div style={{ fontWeight: 600 }}>{c.name}</div>
                    <div style={{ fontSize: 12, color: "var(--muted)", fontFamily: "var(--mono)", marginTop: 4 }}>
                      {c.source_type} → {c.destination_url}
                    </div>
                  </div>
                  <Pill status={c.stats?.status ?? "healthy"} />
                </div>
                <ConnectionStatsRow stats={c.stats} />
              </div>
            </Card>
          </Link>
        ))}
        {connections.length === 0 && !wizard && (
          <p style={{ color: "var(--muted)" }}>No connections yet. Create one to get started.</p>
        )}
      </div>
    </div>
  );
}
