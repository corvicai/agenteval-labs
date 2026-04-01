CREATE TABLE IF NOT EXISTS question_set_share_links (
    id UUID PRIMARY KEY,
    token VARCHAR(255) NOT NULL UNIQUE,
    question_set_id UUID NOT NULL REFERENCES question_sets(id) ON DELETE CASCADE,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    used_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    accepted_question_set_id UUID REFERENCES question_sets(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_question_set_share_links_question_set_id
    ON question_set_share_links(question_set_id);

CREATE INDEX IF NOT EXISTS idx_question_set_share_links_expires_at
    ON question_set_share_links(expires_at);
