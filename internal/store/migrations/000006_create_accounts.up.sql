CREATE TABLE accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bank_code       VARCHAR(20) NOT NULL,
    account_number  VARCHAR(50) NOT NULL,
    currency        VARCHAR(3)  NOT NULL,
    account_type    VARCHAR(20) NOT NULL DEFAULT 'checking',
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    credential_id   UUID NOT NULL REFERENCES bank_credentials(id),
    last_synced_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(bank_code, account_number)
);
