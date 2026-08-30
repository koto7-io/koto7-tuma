import { FormEvent, useState } from "react";
import { api } from "../lib/api";
import { Button } from "../components/Button";
import { TumaLogo } from "../components/TumaLogo";
import { ThemeToggle } from "../components/ThemeToggle";

export function LoginPage({ onLogin }: { onLogin: () => void }) {
  const [email, setEmail] = useState("admin@localhost");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      await api.login(email, password);
      onLogin();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="tuma-auth">
      <div className="tuma-auth__theme">
        <ThemeToggle compact />
      </div>
      <form onSubmit={submit} className="tuma-card tuma-auth__form">
        <TumaLogo size={40} large wordmarkText="tuma" />
        <h1 className="tuma-auth__title">Sign in to Tuma</h1>
        <p className="tuma-auth__sub">Webhook reliability for your stack</p>
        <label className="tuma-auth__field">Email</label>
        <input
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="tuma-input"
          type="email"
          required
        />
        <label className="tuma-auth__field tuma-auth__field-spaced">Password</label>
        <input
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="tuma-input"
          type="password"
          required
        />
        {error && (
          <p className="tuma-alert tuma-alert--error tuma-auth__error">{error}</p>
        )}
        <Button type="submit" disabled={loading} block className="tuma-auth__submit">
          {loading ? "Signing in…" : "Sign in"}
        </Button>
      </form>
    </div>
  );
}
