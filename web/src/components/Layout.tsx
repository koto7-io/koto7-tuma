import { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { Button } from "./Button";
import { TumaLogo } from "./TumaLogo";
import { ThemeToggle } from "./ThemeToggle";

const navClass = (active: boolean) =>
  `tuma-sidebar-nav${active ? " tuma-sidebar-nav--active" : ""}`;

const PILL_LABELS: Record<string, string> = {
  healthy: "Delivering",
  delivered: "Delivered",
  degraded: "Degraded",
  failing: "Failing",
  open: "Open",
  resolved: "Resolved",
  failed: "Failed",
};

function pillClass(status: string) {
  const key = status in PILL_LABELS ? status : "healthy";
  return `tuma-pill tuma-pill--${key}`;
}

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
    <div className="tuma-app-shell">
      <aside className="tuma-sidebar">
        <div className="tuma-sidebar__brand">
          <TumaLogo size={36} wordmarkText="tuma" />
          <div className="tuma-sidebar__tagline">Pipeline</div>
        </div>

        <nav className="tuma-sidebar__nav">
          <NavLink to="/connections" className={({ isActive }) => navClass(isActive)}>
            Connections
          </NavLink>
          <NavLink to="/metrics" className={({ isActive }) => navClass(isActive)}>
            Metrics
          </NavLink>
          <NavLink to="/issues" className={({ isActive }) => navClass(isActive)}>
            Issues
            {issueCount > 0 && <span className="tuma-sidebar__badge">{issueCount}</span>}
          </NavLink>
        </nav>

        <div className="tuma-sidebar__footer">
          <ThemeToggle />
          <Button variant="ghost" onClick={onLogout} block>
            Sign out
          </Button>
        </div>
      </aside>
      <main className="tuma-main">{children}</main>
    </div>
  );
}

export function Pill({ status }: { status: string }) {
  const label = PILL_LABELS[status] ?? PILL_LABELS.healthy;
  return <span className={pillClass(status)}>{label}</span>;
}

export function Card({ children, title }: { children: ReactNode; title?: string }) {
  return (
    <section className="tuma-card">
      {title && <header className="tuma-card__head">{title}</header>}
      {children}
    </section>
  );
}
