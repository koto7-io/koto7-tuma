-- Drop in reverse dependency order (notifications before rules).
DROP TABLE IF EXISTS alert_notifications;
DROP TABLE IF EXISTS alert_rules;
