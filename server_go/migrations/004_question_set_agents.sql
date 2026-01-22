-- Migration: Create question_set_agents junction table
-- This enables per-Question-Set agent configuration

CREATE TABLE IF NOT EXISTS question_set_agents (
    question_set_id UUID NOT NULL REFERENCES question_sets(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT true,
    position INT DEFAULT 0,
    config JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (question_set_id, agent_id)
);

-- Index for efficient lookups by question set
CREATE INDEX IF NOT EXISTS idx_qsa_question_set ON question_set_agents(question_set_id);

-- Index for efficient lookups by agent
CREATE INDEX IF NOT EXISTS idx_qsa_agent ON question_set_agents(agent_id);
