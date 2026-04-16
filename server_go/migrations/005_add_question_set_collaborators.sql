CREATE TABLE IF NOT EXISTS question_set_collaborators (
    id UUID PRIMARY KEY,
    question_set_id UUID NOT NULL REFERENCES question_sets(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(32) NOT NULL DEFAULT 'editor',
    invited_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    accepted_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (question_set_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_qs_collaborators_qs ON question_set_collaborators(question_set_id);
CREATE INDEX IF NOT EXISTS idx_qs_collaborators_user ON question_set_collaborators(user_id)
    WHERE revoked_at IS NULL AND accepted_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS question_set_collab_invites (
    id UUID PRIMARY KEY,
    token VARCHAR(255) NOT NULL UNIQUE,
    question_set_id UUID NOT NULL REFERENCES question_sets(id) ON DELETE CASCADE,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invited_email VARCHAR(255),
    invited_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'editor',
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_qs_collab_invites_qs ON question_set_collab_invites(question_set_id);
CREATE INDEX IF NOT EXISTS idx_qs_collab_invites_expires ON question_set_collab_invites(expires_at);
