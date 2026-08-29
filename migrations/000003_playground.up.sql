CREATE TABLE playground_sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id    UUID NOT NULL UNIQUE REFERENCES connections(id) ON DELETE CASCADE,
    fail_destination BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_playground_sessions_last_seen ON playground_sessions(last_seen_at);

ALTER TABLE connections ADD COLUMN IF NOT EXISTS is_playground BOOLEAN NOT NULL DEFAULT FALSE;
