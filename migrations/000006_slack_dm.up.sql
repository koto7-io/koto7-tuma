-- Slack DMs are a second delivery channel. The destination is a Slack member
-- ID. The bot token lives in SLACK_BOT_TOKEN, not on the rule.

ALTER TABLE alert_rules DROP CONSTRAINT alert_rules_notification_type_check;

ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_notification_type_check
    CHECK (notification_type IN ('email', 'slack_dm'));
