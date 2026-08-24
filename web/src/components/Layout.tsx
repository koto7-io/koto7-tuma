import { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { ThemeToggle } from "./ThemeToggle";

const navStyle = (active: boolean): React.CSSProperties => ({
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: "7px 10px",
  borderRadius: 6,
  cursor: "pointer",
  color: active ? "var(--ink)" : "var(--muted)",
  background: active ? "var(--nav-active-bg)" : "transparent",
  textDecoration: "none",
  fontSize: 14,
  fontWeight: 500,
});

export function Layout({
  children,
  issueCount,
  onLogout,
}: {
  children: ReactNode;
  issueCount: number;
  onLogout: () => void;
}) {
  return (
    <div
      style={{
        minHeight: "100vh",
        display: "grid",
        gridTemplateColumns: "var(--sidebar-width) 1fr",
      }}
    >
      <aside
        style={{
          background: "var(--surface)",
          borderRight: "1px solid var(--border)",
          padding: "20px 14px",
          display: "flex",
          flexDirection: "column",
          gap: 20,
        }}
      >
        <div style={{ padding: "0 10px" }}>
          <div
            style={{
              fontFamily: "var(--mono)",
              fontWeight: 600,
              fontSize: 18,
              letterSpacing: "-0.02em",
            }}
          >
            tuma
          </div>
          <div style={{ fontSize: 12, color: "var(--muted)", marginTop: 4 }}>
            Pipeline
          </div>
        </div>

        <nav style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <NavLink to="/connections" style={({ isActive }) => navStyle(isActive)}>
            Connections
          </NavLink>
          <NavLink to="/metrics" style={({ isActive }) => navStyle(isActive)}>
            Metrics
          </NavLink>
          <NavLink to="/issues" style={({ isActive }) => navStyle(isActive)}>
            Issues
            {issueCount > 0 && (
              <span
                style={{
                  marginLeft: "auto",
                  fontFamily: "var(--mono)",
                  fontSize: 10,
                  padding: "1px 6px",
                  borderRadius: 100,
                  background: "var(--issue-badge-bg)",
                  color: "var(--issue-badge-fg)",
                }}
              >
                {issueCount}
              </span>
            )}
          </NavLink>
        </nav>

        <div style={{ marginTop: "auto", display: "flex", flexDirection: "column", gap: 8 }}>
          <ThemeToggle />
          <button
            onClick={onLogout}
            style={{
              border: "1px solid var(--border)",
              background: "transparent",
              borderRadius: 6,
              padding: "8px 10px",
              cursor: "pointer",
              color: "var(--muted)",
              fontSize: 13,
            }}
          >
            Sign out
          </button>
        </div>
      </aside>
      <main style={{ padding: "28px 32px" }}>{children}</main>
    </div>
  );
}

export function Pill({ status }: { status: string }) {
  const map: Record<string, [string, string, string, string]> = {
    healthy: ["var(--green)", "var(--green-bg)", "var(--green-border)", "Delivering"],
    delivered: ["var(--green)", "var(--green-bg)", "var(--green-border)", "Delivered"],
    degraded: ["var(--amber)", "var(--amber-bg)", "var(--amber-border)", "Degraded"],
    failing: ["var(--red)", "var(--red-bg)", "var(--red-border)", "Failing"],
    open: ["var(--red)", "var(--red-bg)", "var(--red-border)", "Open"],
    resolved: ["var(--green)", "var(--green-bg)", "var(--green-border)", "Resolved"],
    failed: ["var(--red)", "var(--red-bg)", "var(--red-border)", "Failed"],
  };
  const [fg, bg, bd, label] = map[status] || map.healthy;
  return (
    <span
      style={{
        fontFamily: "var(--mono)",
        fontSize: 10,
        letterSpacing: "0.06em",
        textTransform: "uppercase",
        color: fg,
        background: bg,
        border: `1px solid ${bd}`,
        borderRadius: 100,
        padding: "2px 8px",
      }}
    >
      {label}
    </span>
  );
}

export function Card({ children, title }: { children: ReactNode; title?: string }) {
  return (
    <section
      style={{
        background: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: 10,
        overflow: "hidden",
      }}
    >
      {title && (
        <header
          style={{
            padding: "14px 16px",
            borderBottom: "1px solid var(--border-light)",
            fontWeight: 600,
            fontSize: 14,
          }}
        >
          {title}
        </header>
      )}
      {children}
    </section>
  );
}
