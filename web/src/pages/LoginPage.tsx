import { FormEvent, useState } from "react";
import { api } from "../lib/api";
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
    <div
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        background: "var(--bg)",
      }}
    >
      <ThemeToggle compact />
      <form
        onSubmit={submit}
        style={{
          width: 380,
          background: "var(--surface)",
          border: "1px solid var(--border)",
          borderRadius: 12,
          padding: 28,
        }}
      >
        <div
          style={{
            width: 48,
            height: 48,
            borderRadius: 12,
            background: "var(--ink)",
            color: "var(--accent)",
            fontFamily: "var(--mono)",
            fontWeight: 600,
            fontSize: 24,
            display: "grid",
            placeItems: "center",
            marginBottom: 16,
          }}
        >
          t
        </div>
        <h1 style={{ margin: "0 0 6px", fontSize: 22 }}>Sign in to Tuma</h1>
        <p style={{ margin: "0 0 20px", color: "var(--muted)", fontSize: 14 }}>
          Webhook reliability for your stack
        </p>
        <label style={{ display: "block", fontSize: 13, marginBottom: 6 }}>Email</label>
        <input
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          style={inputStyle}
          type="email"
          required
        />
        <label style={{ display: "block", fontSize: 13, margin: "14px 0 6px" }}>
          Password
        </label>
        <input
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          style={inputStyle}
          type="password"
          required
        />
        {error && (
          <p style={{ color: "var(--red)", fontSize: 13, marginTop: 12 }}>{error}</p>
        )}
        <button
          type="submit"
          disabled={loading}
          style={{
            ...buttonPrimary,
            width: "100%",
            marginTop: 18,
            opacity: loading ? 0.7 : 1,
          }}
        >
          {loading ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  );
}

export const inputStyle: React.CSSProperties = {
  width: "100%",
  border: "1px solid var(--border)",
  borderRadius: 6,
  padding: "9px 10px",
  background: "var(--input-bg)",
};

export const buttonPrimary: React.CSSProperties = {
  border: "none",
  borderRadius: 6,
  padding: "10px 14px",
  background: "var(--ink)",
  color: "var(--bg)",
  fontWeight: 600,
  cursor: "pointer",
};

export const buttonSecondary: React.CSSProperties = {
  border: "1px solid var(--border)",
  borderRadius: 6,
  padding: "8px 12px",
  background: "var(--surface)",
  color: "var(--ink)",
  fontWeight: 500,
  cursor: "pointer",
};
