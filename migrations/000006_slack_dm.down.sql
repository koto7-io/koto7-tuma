ALTER TABLE alert_rules DROP CONSTRAINT alert_rules_delivery_threshold;
ALTER TABLE alert_rules DROP CONSTRAINT alert_rules_unit_matches_type;
ALTER TABLE alert_rules DROP COLUMN firing_threshold;

DELETE FROM alert_rules WHERE notification_type = 'slack_dm';

ALTER TABLE alert_rules DROP CONSTRAINT alert_rules_notification_type_check;

ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_notification_type_check
    CHECK (notification_type IN ('slack', 'email'));
