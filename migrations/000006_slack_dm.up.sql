-- 000004 created alert_rules with a Slack webhook channel. That is not a DM.
-- Drop those rows, then allow email and Slack member-ID delivery.
DELETE FROM alert_rules WHERE notification_type = 'slack';

ALTER TABLE alert_rules DROP CONSTRAINT alert_rules_notification_type_check;

ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_notification_type_check
    CHECK (notification_type IN ('email', 'slack_dm'));

-- Set only after a notification is actually delivered. NULL means not firing.
ALTER TABLE alert_rules ADD COLUMN firing_threshold NUMERIC(10,2);

ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_unit_matches_type CHECK (
    (rule_type = 'OPEN_ISSUES' AND unit = 'COUNT') OR
    (rule_type = 'DELIVERY_SUCCESS' AND unit = 'PERCENT') OR
    (rule_type = 'UNRESOLVED_TIME' AND unit = 'HOURS')
);

ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_delivery_threshold CHECK (
    rule_type <> 'DELIVERY_SUCCESS' OR threshold <= 100
);
