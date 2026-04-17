CREATE TABLE IF NOT EXISTS agent_collaborators (
    id UUID PRIMARY KEY,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL DEFAULT 'user',
    invited_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    accepted_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_collaborators_agent ON agent_collaborators(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_collaborators_user ON agent_collaborators(user_id)
    WHERE revoked_at IS NULL AND accepted_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS agent_collab_invites (
    id UUID PRIMARY KEY,
    token VARCHAR(255) NOT NULL UNIQUE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invited_email VARCHAR(255),
    invited_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'user',
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_collab_invites_agent ON agent_collab_invites(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_collab_invites_expires ON agent_collab_invites(expires_at);
