DELETE FROM alert_rules WHERE notification_type = 'slack_dm';

ALTER TABLE alert_rules DROP CONSTRAINT alert_rules_notification_type_check;

ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_notification_type_check
    CHECK (notification_type IN ('email'));
