-- Only one active credential per bank
CREATE UNIQUE INDEX idx_bank_credentials_active_bank ON bank_credentials (bank_code) WHERE status = 'active';
