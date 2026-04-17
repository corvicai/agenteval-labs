-- Append-only audit trail of every change to the active encryption key state.
-- Provides forensic evidence for "when did the key change and why?" questions.
CREATE TABLE IF NOT EXISTS encryption_key_state_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT NOT NULL, -- 'auto_promoted' | 'rotation_completed' | 'initialized' | 'startup_blocked'
    previous_fingerprint_prefix TEXT,
    new_fingerprint_prefix TEXT NOT NULL,
    previous_status TEXT,
    new_status TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'unknown', -- 'startup_auto_promote' | 'startup_rotation' | 'reconcile_init'
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_encryption_key_state_history_created_at
    ON encryption_key_state_history(created_at DESC);
