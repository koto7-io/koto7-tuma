ALTER TABLE events
    ADD COLUMN payload_encrypted BOOLEAN NOT NULL DEFAULT FALSE;

-- Store encrypted header blobs alongside encrypted payloads (both BYTEA).
ALTER TABLE events
    ALTER COLUMN raw_headers TYPE BYTEA USING convert_to(raw_headers::text, 'UTF8');
