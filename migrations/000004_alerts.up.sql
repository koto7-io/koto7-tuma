-- Alert rules and their fired notifications.
-- Both tables are created together because alert_notifications references alert_rules.

CREATE TABLE alert_rules (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT        NOT NULL,
    rule_type         TEXT        NOT NULL CHECK (rule_type IN ('OPEN_ISSUES', 'DELIVERY_SUCCESS', 'UNRESOLVED_TIME')),
    threshold         NUMERIC(10,2) NOT NULL CHECK (threshold >= 0),
    unit              TEXT        NOT NULL CHECK (unit IN ('COUNT', 'PERCENT', 'HOURS')),
    active            BOOLEAN     NOT NULL DEFAULT TRUE,
    notification_type TEXT        NOT NULL CHECK (notification_type IN ('slack', 'email')),
    notification_dest TEXT        NOT NULL,
    subject_template  TEXT,
    body_template     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_rules_active ON alert_rules(active);

CREATE TABLE alert_notifications (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID        NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    rule_name     TEXT        NOT NULL,
    rule_type     TEXT        NOT NULL,
    threshold     NUMERIC(10,2) NOT NULL,
    current_value NUMERIC(10,2) NOT NULL,
    fired_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_notifications_fired_at ON alert_notifications(fired_at DESC);
