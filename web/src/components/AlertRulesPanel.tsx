import { useEffect, useState } from "react";
import { api, AlertRule } from "../lib/api";
import { Button } from "./Button";

// ─── helpers & defaults ──────────────────────────────────────────────────────

const UNIT_FOR_TYPE: Record<string, string> = {
  OPEN_ISSUES: "COUNT",
  DELIVERY_SUCCESS: "PERCENT",
  UNRESOLVED_TIME: "HOURS",
};

const RULE_LABELS: Record<string, string> = {
  OPEN_ISSUES: "Open issues exceed",
  DELIVERY_SUCCESS: "Delivery success drops below",
  UNRESOLVED_TIME: "An issue stays unresolved for",
};

const UNIT_SUFFIX: Record<string, string> = {
  COUNT: "",
  PERCENT: "%",
  HOURS: "h",
};

export const DEFAULT_TEMPLATES: Record<string, { subject: string; body: string }> = {
  OPEN_ISSUES: {
    subject: "Alert: Open Issues Exceeded — {{resource_name}}",
    body: `Your open issues have exceeded the configured limit.

Current open issues : {{current_value}}
Threshold           : {{limit}}

Please review your open issues in the Tuma console.`,
  },
  DELIVERY_SUCCESS: {
    subject: "Alert: Delivery Success Dropped Below Threshold — {{resource_name}}",
    body: `Your delivery success rate has dropped below the configured threshold.

Current success rate : {{current_value}}%
Threshold            : {{limit}}%

Please review recent delivery failures in the Tuma console.`,
  },
  UNRESOLVED_TIME: {
    subject: "Alert: Issue Unresolved Exceeded Threshold — {{resource_name}}",
    body: `An open issue has remained unresolved longer than the configured threshold.

Oldest issue age : {{current_value}}h
Threshold        : {{limit}}h

Please review and resolve open issues in the Tuma console.`,
  },
};

function destLabel(rule: AlertRule): string {
  if (rule.notification_type === "slack") return `#${rule.notification_dest.split("/").pop()} on Slack`;
  return rule.notification_dest;
}

// ─── Toggle ─────────────────────────────────────────────────────────────────

function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      onClick={() => onChange(!on)}
      className={`ar-toggle${on ? " ar-toggle--on" : ""}`}
    >
      <span className="ar-toggle__thumb" />
    </button>
  );
}

// ─── Stepper ────────────────────────────────────────────────────────────────

function Stepper({
  value,
  unit,
  onChange,
}: {
  value: number;
  unit: "COUNT" | "PERCENT" | "HOURS";
  onChange: (v: number) => void;
}) {
  const step = 1;
  const max = unit === "PERCENT" ? 100 : 9999;

  return (
    <div className="ar-stepper">
      <button
        type="button"
        className="ar-stepper__btn"
        onClick={() => onChange(Math.max(0, value - step))}
        aria-label="Decrease"
      >
        −
      </button>
      <span className="ar-stepper__val">
        {value}
        {UNIT_SUFFIX[unit]}
      </span>
      <button
        type="button"
        className="ar-stepper__btn"
        onClick={() => onChange(Math.min(max, value + step))}
        aria-label="Increase"
      >
        +
      </button>
    </div>
  );
}

// ─── New Rule Modal ──────────────────────────────────────────────────────────

const NOTIF_TYPES = ["slack", "email"] as const;

function NewRuleModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (rule: AlertRule) => void;
}) {
  const [ruleType, setRuleType] = useState<keyof typeof RULE_LABELS>("OPEN_ISSUES");
  const [threshold, setThreshold] = useState(5);
  const [name, setName] = useState("");
  const [notifType, setNotifType] = useState<(typeof NOTIF_TYPES)[number]>("email");
  const [notifDest, setNotifDest] = useState("");
  const [subjectTemplate, setSubjectTemplate] = useState(DEFAULT_TEMPLATES.OPEN_ISSUES.subject);
  const [bodyTemplate, setBodyTemplate] = useState(DEFAULT_TEMPLATES.OPEN_ISSUES.body);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  const unit = UNIT_FOR_TYPE[ruleType] as "COUNT" | "PERCENT" | "HOURS";

  function handleTypeChange(nextType: keyof typeof RULE_LABELS) {
    setRuleType(nextType);
    setThreshold(nextType === "DELIVERY_SUCCESS" ? 95 : 5);
    const def = DEFAULT_TEMPLATES[nextType];
    if (def) {
      setSubjectTemplate(def.subject);
      setBodyTemplate(def.body);
    }
  }

  function resetTemplate() {
    const def = DEFAULT_TEMPLATES[ruleType];
    if (def) {
      setSubjectTemplate(def.subject);
      setBodyTemplate(def.body);
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) { setError("Name is required"); return; }
    if (!notifDest.trim()) { setError("Destination is required"); return; }
    setSaving(true);
    setError("");
    try {
      const r = await api.createAlertRule({
        name: name.trim(),
        rule_type: ruleType,
        threshold,
        unit,
        notification_type: notifType,
        notification_dest: notifDest.trim(),
        subject_template: subjectTemplate.trim(),
        body_template: bodyTemplate.trim(),
      });
      onCreated(r.alert_rule);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to create rule");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="ar-modal-overlay" onClick={onClose}>
      <div className="ar-modal ar-modal--wide" onClick={(e) => e.stopPropagation()}>
        <h3 className="ar-modal__title">New alert rule</h3>
        <form onSubmit={submit} className="ar-modal__form">
          <label className="ar-modal__label">Name</label>
          <input
            className="tuma-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. High open issues"
          />

          <label className="ar-modal__label">Rule type</label>
          <select
            className="tuma-input"
            value={ruleType}
            onChange={(e) => handleTypeChange(e.target.value as keyof typeof RULE_LABELS)}
          >
            {Object.entries(RULE_LABELS).map(([k, v]) => (
              <option key={k} value={k}>{v}</option>
            ))}
          </select>

          <label className="ar-modal__label">
            Threshold ({unit === "COUNT" ? "count" : unit === "PERCENT" ? "%" : "hours"})
          </label>
          <input
            className="tuma-input"
            type="number"
            min={0}
            max={unit === "PERCENT" ? 100 : 9999}
            value={threshold}
            onChange={(e) => setThreshold(Number(e.target.value))}
          />

          <label className="ar-modal__label">Notify via</label>
          <select
            className="tuma-input"
            value={notifType}
            onChange={(e) => setNotifType(e.target.value as (typeof NOTIF_TYPES)[number])}
          >
            {NOTIF_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>

          <label className="ar-modal__label">
            {notifType === "email" ? "Email address" : "Slack webhook URL"}
          </label>
          <input
            className="tuma-input"
            value={notifDest}
            onChange={(e) => setNotifDest(e.target.value)}
            placeholder={notifType === "email" ? "ops@company.com" : "https://hooks.slack.com/..."}
          />

          {/* Template customization */}
          <div className="ar-template-section">
            <div className="ar-template-head">
              <span className="ar-template-head__title">Notification Template</span>
              <button type="button" className="ar-btn-reset" onClick={resetTemplate}>
                Reset to default
              </button>
            </div>
            <p className="ar-template-hint">
              Variables: <code>{"{{resource_name}}"}</code>, <code>{"{{limit}}"}</code>, <code>{"{{current_value}}"}</code>
            </p>

            <label className="ar-modal__label">Subject template</label>
            <input
              className="tuma-input"
              value={subjectTemplate}
              onChange={(e) => setSubjectTemplate(e.target.value)}
            />

            <label className="ar-modal__label">Body template</label>
            <textarea
              className="tuma-input ar-textarea"
              value={bodyTemplate}
              onChange={(e) => setBodyTemplate(e.target.value)}
              rows={5}
            />
          </div>

          {error && <p className="ar-modal__error">{error}</p>}

          <div className="ar-modal__actions">
            <Button type="submit" disabled={saving}>{saving ? "Creating…" : "Create rule"}</Button>
            <Button variant="secondary" type="button" onClick={onClose}>Cancel</Button>
          </div>
        </form>
      </div>
    </div>
  );
}

// ─── Edit Rule Modal ─────────────────────────────────────────────────────────

function EditRuleModal({
  rule,
  onClose,
  onUpdated,
}: {
  rule: AlertRule;
  onClose: () => void;
  onUpdated: (rule: AlertRule) => void;
}) {
  const def = DEFAULT_TEMPLATES[rule.rule_type] ?? DEFAULT_TEMPLATES.OPEN_ISSUES;
  const [threshold, setThreshold] = useState(rule.threshold);
  const [subjectTemplate, setSubjectTemplate] = useState(rule.subject_template ?? def.subject);
  const [bodyTemplate, setBodyTemplate] = useState(rule.body_template ?? def.body);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  const unit = rule.unit as "COUNT" | "PERCENT" | "HOURS";

  function resetTemplate() {
    setSubjectTemplate(def.subject);
    setBodyTemplate(def.body);
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError("");
    try {
      const updated = await api.patchAlertRule(rule.id, {
        threshold,
        subject_template: subjectTemplate.trim(),
        body_template: bodyTemplate.trim(),
      });
      onUpdated(updated);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to update rule");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="ar-modal-overlay" onClick={onClose}>
      <div className="ar-modal ar-modal--wide" onClick={(e) => e.stopPropagation()}>
        <h3 className="ar-modal__title">Edit rule: {rule.name}</h3>
        <form onSubmit={submit} className="ar-modal__form">
          <label className="ar-modal__label">Rule type</label>
          <input className="tuma-input" disabled value={RULE_LABELS[rule.rule_type] ?? rule.rule_type} />

          <label className="ar-modal__label">
            Threshold ({unit === "COUNT" ? "count" : unit === "PERCENT" ? "%" : "hours"})
          </label>
          <input
            className="tuma-input"
            type="number"
            min={0}
            max={unit === "PERCENT" ? 100 : 9999}
            value={threshold}
            onChange={(e) => setThreshold(Number(e.target.value))}
          />

          <label className="ar-modal__label">Destination ({rule.notification_type})</label>
          <input className="tuma-input" disabled value={rule.notification_dest} />

          {/* Template customization */}
          <div className="ar-template-section">
            <div className="ar-template-head">
              <span className="ar-template-head__title">Notification Template</span>
              <button type="button" className="ar-btn-reset" onClick={resetTemplate}>
                Reset to default
              </button>
            </div>
            <p className="ar-template-hint">
              Variables: <code>{"{{resource_name}}"}</code>, <code>{"{{limit}}"}</code>, <code>{"{{current_value}}"}</code>
            </p>

            <label className="ar-modal__label">Subject template</label>
            <input
              className="tuma-input"
              value={subjectTemplate}
              onChange={(e) => setSubjectTemplate(e.target.value)}
            />

            <label className="ar-modal__label">Body template</label>
            <textarea
              className="tuma-input ar-textarea"
              value={bodyTemplate}
              onChange={(e) => setBodyTemplate(e.target.value)}
              rows={5}
            />
          </div>

          {error && <p className="ar-modal__error">{error}</p>}

          <div className="ar-modal__actions">
            <Button type="submit" disabled={saving}>{saving ? "Saving…" : "Save changes"}</Button>
            <Button variant="secondary" type="button" onClick={onClose}>Cancel</Button>
          </div>
        </form>
      </div>
    </div>
  );
}

// ─── Main Panel ──────────────────────────────────────────────────────────────

export function AlertRulesPanel() {
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [activeCount, setActiveCount] = useState(0);
  const [showModal, setShowModal] = useState(false);
  const [editingRule, setEditingRule] = useState<AlertRule | null>(null);
  const [error, setError] = useState("");

  function load() {
    api.listAlertRules()
      .then((r) => {
        setRules(r.alert_rules ?? []);
        setActiveCount(r.active_count ?? 0);
      })
      .catch(() => setError("Failed to load alert rules"));
  }

  useEffect(() => { load(); }, []);

  async function toggleRule(rule: AlertRule) {
    try {
      const updated = await api.patchAlertRule(rule.id, { active: !rule.active });
      setRules((prev) => prev.map((r) => (r.id === rule.id ? updated : r)));
      setActiveCount((prev) => prev + (rule.active ? -1 : 1));
    } catch {
      setError("Failed to update rule");
    }
  }

  async function changeThreshold(rule: AlertRule, value: number) {
    try {
      const updated = await api.patchAlertRule(rule.id, { threshold: value });
      setRules((prev) => prev.map((r) => (r.id === rule.id ? updated : r)));
    } catch {
      setError("Failed to update threshold");
    }
  }

  async function deleteRule(rule: AlertRule) {
    if (!window.confirm(`Delete alert rule "${rule.name}"?`)) return;
    try {
      await api.deleteAlertRule(rule.id);
      setRules((prev) => prev.filter((r) => r.id !== rule.id));
      if (rule.active) {
        setActiveCount((prev) => Math.max(0, prev - 1));
      }
    } catch {
      setError("Failed to delete rule");
    }
  }

  function onCreated(rule: AlertRule) {
    setRules((prev) => [rule, ...prev]);
    setActiveCount((prev) => prev + 1); // new rules are active by default
    setShowModal(false);
  }

  function onUpdated(updated: AlertRule) {
    setRules((prev) => prev.map((r) => (r.id === updated.id ? updated : r)));
    setEditingRule(null);
  }

  const total = rules.length;

  return (
    <>
      <section className="tuma-card ar-panel" style={{ maxWidth: 580, width: "100%" }}>
        {/* Header */}
        <div className="ar-panel__head">
          <div>
            <span className="ar-panel__title">Alert rules</span>
            <span className="ar-panel__meta">
              {activeCount} of {total} rule{total !== 1 ? "s" : ""} active
            </span>
          </div>
          <Button
            onClick={() => setShowModal(true)}
            disabled={total >= 10}
            title={total >= 10 ? "Maximum 10 rules reached" : undefined}
          >
            New rule
          </Button>
        </div>

        {/* Error */}
        {error && <p className="ar-panel__error">{error}</p>}

        {/* Empty state */}
        {rules.length === 0 && !error && (
          <p className="tuma-empty tuma-empty--center">
            No alert rules yet. Create one to get notified when something goes wrong.
          </p>
        )}

        {/* Rule rows */}
        {rules.map((rule, idx) => (
          <div
            key={rule.id}
            className={`ar-row${!rule.active ? " ar-row--inactive" : ""}${idx < rules.length - 1 ? " ar-row--bordered" : ""}`}
          >
            <Toggle on={rule.active} onChange={() => toggleRule(rule)} />
            <div className="ar-row__info">
              <span className="ar-row__name">{RULE_LABELS[rule.rule_type] ?? rule.name}</span>
              <span className="ar-row__dest">{destLabel(rule)}</span>
            </div>
            <Stepper
              value={rule.threshold}
              unit={rule.unit as "COUNT" | "PERCENT" | "HOURS"}
              onChange={(v) => changeThreshold(rule, v)}
            />
            <button
              type="button"
              className="ar-row__edit"
              onClick={() => setEditingRule(rule)}
              title="Edit template & threshold"
              aria-label="Edit rule"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
              </svg>
            </button>
            <button
              type="button"
              className="ar-row__delete"
              onClick={() => deleteRule(rule)}
              title="Delete rule"
              aria-label="Delete rule"
            >
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="3 6 5 6 21 6" />
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                <line x1="10" y1="11" x2="10" y2="17" />
                <line x1="14" y1="11" x2="14" y2="17" />
              </svg>
            </button>
          </div>
        ))}
      </section>

      {showModal && (
        <NewRuleModal onClose={() => setShowModal(false)} onCreated={onCreated} />
      )}

      {editingRule && (
        <EditRuleModal
          rule={editingRule}
          onClose={() => setEditingRule(null)}
          onUpdated={onUpdated}
        />
      )}
    </>
  );
}
