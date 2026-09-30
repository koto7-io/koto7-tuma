-- Last delivery failure, shown on the rule so a blocked DM or a bad
-- SMTP setup is visible without reading worker logs.
ALTER TABLE alert_rules ADD COLUMN delivery_error TEXT;
