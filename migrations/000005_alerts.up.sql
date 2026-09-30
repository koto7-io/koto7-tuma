-- Alert rules and their fired notifications.
-- Version 000005 because 000004 is event encryption. Two files sharing a
-- version make golang-migrate refuse to start.

CREATE TABLE alert_rules (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT          NOT NULL,
    rule_type         TEXT          NOT NULL CHECK (rule_type IN ('OPEN_ISSUES', 'DELIVERY_SUCCESS', 'UNRESOLVED_TIME')),
    threshold         NUMERIC(10,2) NOT NULL CHECK (threshold >= 0),
    unit              TEXT          NOT NULL CHECK (unit IN ('COUNT', 'PERCENT', 'HOURS')),
    active            BOOLEAN       NOT NULL DEFAULT TRUE,
    notification_type TEXT          NOT NULL CHECK (notification_type IN ('email')),
    notification_dest TEXT          NOT NULL,
    subject_template  TEXT,
    body_template     TEXT,
    -- Set only after a notification is actually delivered. NULL means not firing.
    firing_threshold  NUMERIC(10,2),
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT alert_rules_unit_matches_type CHECK (
        (rule_type = 'OPEN_ISSUES' AND unit = 'COUNT') OR
        (rule_type = 'DELIVERY_SUCCESS' AND unit = 'PERCENT') OR
        (rule_type = 'UNRESOLVED_TIME' AND unit = 'HOURS')
    ),
    CONSTRAINT alert_rules_delivery_threshold CHECK (
        rule_type <> 'DELIVERY_SUCCESS' OR threshold <= 100
    )
);

CREATE INDEX idx_alert_rules_active ON alert_rules(active);

CREATE TABLE alert_notifications (
    id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID          NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    rule_name     TEXT          NOT NULL,
    rule_type     TEXT          NOT NULL,
    threshold     NUMERIC(10,2) NOT NULL,
    current_value NUMERIC(10,2) NOT NULL,
    fired_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_notifications_fired_at ON alert_notifications(fired_at DESC);
