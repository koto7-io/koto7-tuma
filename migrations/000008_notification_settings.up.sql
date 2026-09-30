-- Console overrides for alert delivery. Empty fields fall back to the
-- server environment. Secrets are stored encrypted.

CREATE TABLE notification_settings (
    id                   SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    smtp_host            TEXT        NOT NULL DEFAULT '',
    smtp_port            INT         NOT NULL DEFAULT 0,
    smtp_username        TEXT        NOT NULL DEFAULT '',
    smtp_password_enc    BYTEA,
    smtp_from            TEXT        NOT NULL DEFAULT '',
    slack_bot_token_enc  BYTEA,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO notification_settings (id) VALUES (1);
