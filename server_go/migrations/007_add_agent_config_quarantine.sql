-- Stores the raw encrypted ciphertext of agent configs before they are overwritten
-- or force-deleted due to a decryption failure. Acts as a last-resort recovery
-- archive: if the correct ENCRYPTION_KEY is found later, the original config
-- can still be recovered from this table.
CREATE TABLE IF NOT EXISTS agent_config_quarantines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL,
    agent_name TEXT NOT NULL DEFAULT '',
    workspace_id UUID NOT NULL,
    original_ciphertext TEXT NOT NULL,
    quarantine_reason TEXT NOT NULL DEFAULT 'decryption_failed',
    action TEXT NOT NULL DEFAULT 'overwrite', -- 'overwrite' | 'force_delete'
    actor_user_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_config_quarantines_agent_id
    ON agent_config_quarantines(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_config_quarantines_workspace_id
    ON agent_config_quarantines(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agent_config_quarantines_created_at
    ON agent_config_quarantines(created_at DESC);
