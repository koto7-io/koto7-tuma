CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('stripe', 'github', 'generic_hmac', 'internal')),
    inbound_path TEXT NOT NULL UNIQUE,
    destination_url TEXT NOT NULL,
    signing_secret_encrypted BYTEA NOT NULL,
    retry_attempts INT NOT NULL DEFAULT 8 CHECK (retry_attempts > 0),
    retry_first_delay_s INT NOT NULL DEFAULT 60 CHECK (retry_first_delay_s > 0),
    retry_backoff_factor NUMERIC(4,2) NOT NULL DEFAULT 2.0 CHECK (retry_backoff_factor >= 1),
    retention_days INT NOT NULL DEFAULT 7 CHECK (retention_days > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connections_inbound_path ON connections(inbound_path);

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    provider_event_id TEXT NOT NULL,
    raw_headers JSONB NOT NULL DEFAULT '{}',
    raw_payload BYTEA NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    workflow_id TEXT,
    UNIQUE (connection_id, provider_event_id)
);

CREATE INDEX idx_events_connection_id ON events(connection_id);
CREATE INDEX idx_events_received_at ON events(received_at DESC);

CREATE TABLE deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    attempt_number INT NOT NULL CHECK (attempt_number > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'failed')),
    response_code INT,
    latency_ms INT,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (event_id, attempt_number)
);

CREATE INDEX idx_deliveries_event_id ON deliveries(event_id);
CREATE INDEX idx_deliveries_attempted_at ON deliveries(attempted_at DESC);

CREATE TABLE issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    connection_id UUID NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    attempts_exhausted INT NOT NULL,
    first_failed_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    resolved_at TIMESTAMPTZ,
    resolved_by UUID REFERENCES users(id),
    UNIQUE (event_id)
);

CREATE INDEX idx_issues_status ON issues(status);
CREATE INDEX idx_issues_connection_id ON issues(connection_id);
CREATE INDEX idx_issues_first_failed_at ON issues(first_failed_at DESC);
