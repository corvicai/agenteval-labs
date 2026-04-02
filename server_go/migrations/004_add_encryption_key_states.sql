CREATE TABLE IF NOT EXISTS encryption_key_states (
    id VARCHAR(64) PRIMARY KEY,
    cipher_version VARCHAR(64) NOT NULL,
    active_fingerprint VARCHAR(128) NOT NULL,
    active_format VARCHAR(32),
    active_char_length INTEGER NOT NULL DEFAULT 0,
    active_parsed_bytes INTEGER NOT NULL DEFAULT 0,
    sentinel_ciphertext TEXT NOT NULL,
    last_seen_fingerprint VARCHAR(128) NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_status VARCHAR(64) NOT NULL DEFAULT 'unknown',
    last_mismatch_at TIMESTAMPTZ,
    last_mismatch_fingerprint VARCHAR(128),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_encryption_key_states_last_seen_at
    ON encryption_key_states(last_seen_at);
