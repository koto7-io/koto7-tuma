import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, Connection } from "../lib/api";
import { usePageRestore } from "../lib/usePageRestore";
import { Button } from "../components/Button";
import { Card, Pill } from "../components/Layout";
import { PageHeader } from "../components/PageHeader";
import { ConnectionStatsRow } from "../components/StatsRow";

const SOURCES = [
  { key: "stripe", name: "Stripe", sub: "Events API" },
  { key: "github", name: "GitHub", sub: "Repo webhooks" },
  { key: "easypost", name: "EasyPost", sub: "Tracking & shipping events" },
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
  const [sinkURL, setSinkURL] = useState("");
  const navigate = useNavigate();

  usePageRestore(() => {
    api.listConnections().then((r) => setConnections(r.connections)).catch(console.error);
    api.getConfig().then((c) => setSinkURL(c.sink_url)).catch(console.error);
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
      <PageHeader
        title="Connections"
        subtitle="Inbound webhooks → reliable delivery to your apps"
        action={
          <Button onClick={() => { setWizard(true); setStep(0); setCreated(null); }}>
            New connection
          </Button>
        }
      />

      {wizard && (
        <Card title="New connection">
          <div className="tuma-card-body">
            {step === 0 && (
              <div className="tuma-source-grid">
                {SOURCES.map((s) => (
                  <button
                    key={s.key}
                    type="button"
                    onClick={() => { setSource(s.key); setStep(1); }}
                    className={`tuma-source-btn${source === s.key ? " tuma-source-btn--selected" : ""}`}
                  >
                    <div className="tuma-source-btn__name">{s.name}</div>
                    <div className="tuma-source-btn__sub">{s.sub}</div>
                  </button>
                ))}
              </div>
            )}
            {step === 1 && (
              <form onSubmit={create}>
                <label className="tuma-field">
                  <span className="tuma-field__label">Name</span>
                  <input className="tuma-input" value={name} onChange={(e) => setName(e.target.value)} required />
                </label>
                <label className="tuma-field">
                  <span className="tuma-field__label">Destination URL</span>
                  <input
                    className="tuma-input"
                    value={dest}
                    onChange={(e) => setDest(e.target.value)}
                    placeholder="https://api.example.com/webhooks"
                    required
                  />
                  {sinkURL && (
                    <span className="tuma-field-caption">
                      No app yet?{" "}
                      <button type="button" className="tuma-text-btn" onClick={() => setDest(sinkURL)}>
                        Use Tuma test receiver
                      </button>
                    </span>
                  )}
                </label>
                {source !== "internal" && (
                  <label className="tuma-field">
                    <span className="tuma-field__label">Signing secret</span>
                    <input
                      className="tuma-input"
                      value={signingSecret}
                      onChange={(e) => setSigningSecret(e.target.value)}
                      placeholder={source === "stripe" ? "whsec_… from Stripe Dashboard" : "From provider dashboard"}
                    />
                    {source === "stripe" && (
                      <span className="tuma-field-caption">
                        Must be the secret Stripe shows after you add the webhook URL (Reveal secret). Tuma cannot invent this — a generated whsec_ will 401 every real Stripe event.
                      </span>
                    )}
                  </label>
                )}
                <div className="tuma-field-row">
                  <Button type="button" variant="secondary" onClick={() => setStep(0)}>Back</Button>
                  <Button type="submit">Create</Button>
                </div>
              </form>
            )}
            {step === 2 && created && (
              <div>
                <p className="tuma-page-sub">
                  {source === "stripe"
                    ? "Paste this URL into Stripe Dashboard → Developers → Webhooks. Then paste Stripe’s signing secret on the connection page — not a Tuma-generated one."
                    : `Paste this into your ${source} endpoint settings. Nothing else changes.`}
                </p>
                <div className="tuma-field-caption">Webhook URL</div>
                <div className="tuma-code-row tuma-mb-16">
                  <code className="tuma-code">{created.connection.inbound_url}</code>
                  <Button variant="secondary" onClick={() => copy(created.connection.inbound_url)}>
                    {copied ? "Copied" : "Copy"}
                  </Button>
                </div>
                {created.signing_secret && source !== "stripe" && (
                  <>
                    <div className="tuma-field-caption">Signing secret</div>
                    <code className="tuma-code tuma-code--block">{created.signing_secret}</code>
                  </>
                )}
                {source === "stripe" && (
                  <p className="tuma-field-caption">
                    Open the connection and save the <code className="tuma-code">whsec_</code> Stripe shows under Reveal secret. Until that matches, Stripe will get 401 invalid signature.
                  </p>
                )}
                <Button className="tuma-mt-16" onClick={() => setWizard(false)}>Done</Button>
              </div>
            )}
          </div>
        </Card>
      )}

      <div className={`tuma-stack${wizard ? " tuma-mt-16" : ""}`}>
        {connections.map((c) => (
          <Card key={c.id}>
            <div className="tuma-card-body tuma-pad-card">
              <div className="tuma-conn-row__top">
                <div>
                  <div className="tuma-conn-row__title">{c.name}</div>
                  <div className="tuma-conn-row__meta">
                    {c.source_type} → {c.destination_url}
                  </div>
                </div>
                <div className="tuma-conn-row__actions">
                  <Pill status={c.stats?.status ?? "healthy"} />
                  <Button onClick={() => navigate(`/connections/${c.id}`)}>Edit</Button>
                </div>
              </div>
              <Link to={`/connections/${c.id}`} className="tuma-link-card">
                <ConnectionStatsRow stats={c.stats} />
              </Link>
            </div>
          </Card>
        ))}
        {connections.length === 0 && !wizard && (
          <p className="tuma-empty">No connections yet. Create one to get started.</p>
        )}
      </div>
    </div>
  );
}
