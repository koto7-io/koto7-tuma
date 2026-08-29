ALTER TABLE connections DROP CONSTRAINT IF EXISTS connections_source_type_check;

ALTER TABLE connections ADD CONSTRAINT connections_source_type_check
    CHECK (source_type IN ('stripe', 'github', 'generic_hmac', 'internal'));
