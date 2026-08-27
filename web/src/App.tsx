import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./components/Layout";
import { LoginPage } from "./pages/LoginPage";
import { ConnectionsPage } from "./pages/ConnectionsPage";
import { ConnectionDetailPage } from "./pages/ConnectionDetailPage";
import { IssuesPage } from "./pages/IssuesPage";
import { MetricsPage } from "./pages/MetricsPage";
import { api } from "./lib/api";
import { usePageRestore } from "./lib/usePageRestore";

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [issueCount, setIssueCount] = useState(0);

  usePageRestore(() => {
    api
      .listConnections()
      .then(() => setAuthed(true))
      .catch(() => setAuthed(false));
  }, []);

  useEffect(() => {
    if (!authed) return;
    api.listIssues("open").then((r) => setIssueCount((r.issues ?? []).length)).catch(() => {});
  }, [authed]);

  if (authed === null) {
    return <div style={{ padding: 40, color: "var(--muted)" }}>Loading…</div>;
  }

  if (!authed) {
    return <LoginPage onLogin={() => setAuthed(true)} />;
  }

  return (
    <Layout
      issueCount={issueCount}
      onLogout={async () => {
        await api.logout();
        setAuthed(false);
      }}
    >
      <Routes>
        <Route path="/" element={<Navigate to="/connections" replace />} />
        <Route path="/connections" element={<ConnectionsPage />} />
        <Route path="/connections/:id" element={<ConnectionDetailPage />} />
        <Route path="/metrics" element={<MetricsPage />} />
        <Route path="/issues" element={<IssuesPage />} />
      </Routes>
    </Layout>
  );
}
