ALTER TABLE events
    ALTER COLUMN raw_headers TYPE JSONB USING convert_from(raw_headers, 'UTF8')::jsonb;

ALTER TABLE events
    DROP COLUMN payload_encrypted;
