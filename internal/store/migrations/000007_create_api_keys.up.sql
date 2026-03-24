CREATE TABLE api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash     BYTEA        NOT NULL UNIQUE,
    client_id    VARCHAR(50)  NOT NULL,
    description  VARCHAR(200),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);
