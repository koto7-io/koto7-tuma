import { FormEvent, useState } from "react";
import { api, NotificationSettings } from "../lib/api";
import { usePageRestore } from "../lib/usePageRestore";
import { Button } from "../components/Button";
import { Card } from "../components/Layout";
import { PageHeader } from "../components/PageHeader";

export function AdminPage() {
  const [settings, setSettings] = useState<NotificationSettings | null>(null);
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [username, setUsername] = useState("");
  const [from, setFrom] = useState("");
  const [password, setPassword] = useState("");
  const [clearPassword, setClearPassword] = useState(false);
  const [token, setToken] = useState("");
  const [clearToken, setClearToken] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [saving, setSaving] = useState(false);

  function apply(s: NotificationSettings) {
    setSettings(s);
    setHost(s.smtp_host);
    setPort(s.smtp_port > 0 ? String(s.smtp_port) : "");
    setUsername(s.smtp_username);
    setFrom(s.smtp_from);
    setPassword("");
    setToken("");
    setClearPassword(false);
    setClearToken(false);
  }

  usePageRestore(() => {
    api.getNotificationSettings().then(apply).catch(() => setError("Failed to load notification settings"));
  }, []);

  async function save(e: FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    setSaved(false);
    const parsedPort = port.trim() === "" ? 0 : Number(port);
    if (!Number.isInteger(parsedPort) || parsedPort < 0 || parsedPort > 65535) {
      setError("SMTP port must be between 1 and 65535, or blank to use the server environment.");
      setSaving(false);
      return;
    }
    try {
      const next = await api.saveNotificationSettings({
        smtp_host: host.trim(),
        smtp_port: parsedPort,
        smtp_username: username.trim(),
        smtp_from: from.trim(),
        smtp_password: clearPassword ? "" : password,
        clear_smtp_password: clearPassword,
        slack_bot_token: clearToken ? "" : token,
        clear_slack_token: clearToken,
      });
      apply(next);
      setSaved(true);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  }

  if (!settings && !error) return <p className="tuma-loading-page">Loading…</p>;

  return (
    <div>
      <PageHeader
        title="Admin"
        subtitle="Alert delivery. A value saved here overrides the server environment. Leave a field blank to keep using the environment."
      />

      <form onSubmit={save} className="tuma-stack" style={{ maxWidth: 640 }}>
        <Card title="Email">
          <div className="tuma-card-body ar-modal__form">
            <p className="ar-template-hint">{emailStatus(settings)}</p>
            {settings?.env_smtp_host && (
              <p className="ar-template-hint">
                Server environment: <code>{settings.env_smtp_host}</code>
                {settings.env_smtp_port ? `:${settings.env_smtp_port}` : ""}
                {settings.env_smtp_from ? ` · from ${settings.env_smtp_from}` : ""}
              </p>
            )}
            <label className="ar-modal__label">SMTP host</label>
            <input className="tuma-input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="smtp.example.com" />
            <label className="ar-modal__label">Port</label>
            <input className="tuma-input" inputMode="numeric" value={port} onChange={(e) => setPort(e.target.value)} placeholder="587" />
            <label className="ar-modal__label">Username</label>
            <input className="tuma-input" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />
            <label className="ar-modal__label">Password</label>
            <input
              className="tuma-input"
              type="password"
              value={clearPassword ? "" : password}
              onChange={(e) => { setPassword(e.target.value); setClearPassword(false); }}
              placeholder={passwordPlaceholder(settings)}
              autoComplete="new-password"
              disabled={clearPassword}
            />
            {settings?.smtp_password_saved && (
              <label className="ar-template-hint">
                <input type="checkbox" checked={clearPassword} onChange={(e) => setClearPassword(e.target.checked)} />
                {" "}Remove the saved password and use the server environment
              </label>
            )}
            <label className="ar-modal__label">From address</label>
            <input className="tuma-input" type="email" value={from} onChange={(e) => setFrom(e.target.value)} placeholder="alerts@example.com" />
          </div>
        </Card>

        <Card title="Slack DM">
          <div className="tuma-card-body ar-modal__form">
            <p className="ar-template-hint">{slackStatus(settings)}</p>
            <p className="ar-template-hint">
              Bot token for one workspace. The app needs <code>chat:write</code> and <code>im:write</code>. This is not a webhook URL.
            </p>
            <label className="ar-modal__label">Bot token</label>
            <input
              className="tuma-input"
              type="password"
              value={clearToken ? "" : token}
              onChange={(e) => { setToken(e.target.value); setClearToken(false); }}
              placeholder={tokenPlaceholder(settings)}
              autoComplete="new-password"
              disabled={clearToken}
            />
            {settings?.slack_token_saved && (
              <label className="ar-template-hint">
                <input type="checkbox" checked={clearToken} onChange={(e) => setClearToken(e.target.checked)} />
                {" "}Remove the saved token and use the server environment
              </label>
            )}
          </div>
        </Card>

        {error && <p className="ar-modal__error">{error}</p>}
        {saved && <p className="ar-template-hint">Saved. The next alert uses this.</p>}
        <div>
          <Button type="submit" disabled={saving || !settings}>{saving ? "Saving…" : "Save"}</Button>
        </div>
      </form>
    </div>
  );
}

function emailStatus(s: NotificationSettings | null): string {
  if (!s) return "";
  if (!s.email_enabled) return "Email is off.";
  if (s.smtp_host) return "Email is on. Values saved here override the server environment.";
  return "Email is on, using the server environment.";
}

function slackStatus(s: NotificationSettings | null): string {
  if (!s) return "";
  if (!s.slack_enabled) return "Slack DMs are off.";
  if (s.slack_token_saved) return "Slack DMs are on. The token saved here overrides the server environment.";
  return "Slack DMs are on, using SLACK_BOT_TOKEN from the server environment.";
}

function passwordPlaceholder(s: NotificationSettings | null): string {
  if (s?.smtp_password_saved) return "Saved. Type a new password to replace it.";
  if (s?.smtp_password_in_env) return "Using the server environment. Type here to override.";
  return "Not set";
}

function tokenPlaceholder(s: NotificationSettings | null): string {
  if (s?.slack_token_saved) return "Saved. Type a new token to replace it.";
  if (s?.slack_token_in_env) return "Using the server environment. Type here to override.";
  return "xoxb-…";
}
