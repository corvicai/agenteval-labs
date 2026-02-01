-- 001_initial_schema.sql
-- consolidated schema for Benchmarking Platform

-- Organizations
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    is_suspended BOOLEAN DEFAULT false,
    audit_logs_enabled BOOLEAN DEFAULT false,
    manager_id UUID,
    created_by_user_id UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Users
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_admin BOOLEAN DEFAULT false,
    is_suspended BOOLEAN DEFAULT false,
    invited_by_user_id UUID,
    last_login_at TIMESTAMPTZ,
    terms_accepted_at TIMESTAMPTZ,
    firebase_uid VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indices for users
CREATE INDEX IF NOT EXISTS idx_users_firebase_uid ON users(firebase_uid);

-- Add manager FK (after users table exists)
DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_org_manager') THEN
        ALTER TABLE organizations ADD CONSTRAINT fk_org_manager FOREIGN KEY (manager_id) REFERENCES users(id);
    END IF;
END $$;

-- Workspaces
CREATE TABLE IF NOT EXISTS workspaces (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Clients
CREATE TABLE IF NOT EXISTS clients (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Agents
CREATE TABLE IF NOT EXISTS agents (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    provider_type VARCHAR(50) NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    enabled BOOLEAN DEFAULT true,
    position INTEGER DEFAULT 0,
    max_concurrency INTEGER DEFAULT 5,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Question Sets
CREATE TABLE IF NOT EXISTS question_sets (
    id UUID PRIMARY KEY,
    client_id UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    version VARCHAR(50),
    data JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Runs
CREATE TABLE IF NOT EXISTS runs (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    question_set_id UUID NOT NULL REFERENCES question_sets(id),
    status VARCHAR(50) NOT NULL DEFAULT 'running',
    total_tasks INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Run Results
CREATE TABLE IF NOT EXISTS run_results (
    id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    question_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    answer TEXT,
    error TEXT,
    metadata JSONB DEFAULT '{}',
    duration_ms INTEGER,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Evaluations
CREATE TABLE IF NOT EXISTS evaluations (
    id UUID PRIMARY KEY,
    run_result_id UUID NOT NULL REFERENCES run_results(id) ON DELETE CASCADE,
    rater_type VARCHAR(50) NOT NULL,
    rater_id UUID,
    rating VARCHAR(50) NOT NULL,
    rating_code INTEGER,
    score INTEGER,
    comments TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Stats Cache
CREATE TABLE IF NOT EXISTS stats_caches (
    id UUID PRIMARY KEY,
    scope VARCHAR(20) NOT NULL,
    scope_id UUID,
    data JSONB NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indices for stats_cache
CREATE UNIQUE INDEX IF NOT EXISTS idx_stats_cache_scope ON stats_caches(scope, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX IF NOT EXISTS idx_stats_cache_scope_type ON stats_caches(scope);
CREATE INDEX IF NOT EXISTS idx_stats_cache_expires ON stats_caches(expires_at);

-- Question Set Agents
CREATE TABLE IF NOT EXISTS question_set_agents (
    question_set_id UUID NOT NULL REFERENCES question_sets(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT true,
    position INT DEFAULT 0,
    config JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (question_set_id, agent_id)
);

-- Indices for question_set_agents
CREATE INDEX IF NOT EXISTS idx_qsa_question_set ON question_set_agents(question_set_id);
CREATE INDEX IF NOT EXISTS idx_qsa_agent ON question_set_agents(agent_id);

-- User Organizations
CREATE TABLE IF NOT EXISTS user_organizations (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    invited_by_user_id UUID,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, organization_id)
);

-- Audit Logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(100),
    details TEXT,
    remote_ip VARCHAR(50),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Login Logs
CREATE TABLE IF NOT EXISTS login_logs (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    user_email VARCHAR(255) NOT NULL,
    organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
    ip_address VARCHAR(50) NOT NULL,
    user_agent TEXT,
    status VARCHAR(50) NOT NULL,
    failure_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Invite Codes
CREATE TABLE IF NOT EXISTS invite_codes (
    code VARCHAR(255) PRIMARY KEY,
    created_by UUID NOT NULL REFERENCES users(id),
    organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE,
    role VARCHAR(50) DEFAULT 'member',
    is_new_org BOOLEAN DEFAULT false,
    expires_at TIMESTAMPTZ NOT NULL,
    max_uses INTEGER DEFAULT 1,
    use_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Invite Code Usages
CREATE TABLE IF NOT EXISTS invite_code_usages (
    id UUID PRIMARY KEY,
    code VARCHAR(255) NOT NULL REFERENCES invite_codes(code) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    used_at TIMESTAMPTZ DEFAULT NOW()
);

-- Passkeys
CREATE TABLE IF NOT EXISTS passkeys (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA UNIQUE NOT NULL,
    public_key BYTEA NOT NULL,
    attestation VARCHAR(50),
    sign_count BIGINT DEFAULT 0,
    backup_eligible BOOLEAN DEFAULT false,
    backup_state BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- General Indices
CREATE INDEX IF NOT EXISTS idx_users_org_id_deprecated ON users(id); -- note: users no longer have organization_id directly
CREATE INDEX IF NOT EXISTS idx_workspaces_user_id ON workspaces(user_id);
-- Organization index removed - workspaces no longer have organization_id
CREATE INDEX IF NOT EXISTS idx_clients_workspace_id ON clients(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agents_workspace_id ON agents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_question_sets_client_id ON question_sets(client_id);
CREATE INDEX IF NOT EXISTS idx_runs_workspace_id ON runs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_run_results_run_id ON run_results(run_id);
CREATE INDEX IF NOT EXISTS idx_evaluations_run_result_id ON evaluations(run_result_id);
CREATE INDEX IF NOT EXISTS idx_evaluations_rating_code ON evaluations(rating_code);

-- Data Backfills
UPDATE evaluations SET rating_code = CASE
    WHEN rating = 'like' THEN 1
    WHEN rating = 'valid' THEN 2
    WHEN rating = 'dislike' THEN 3
    WHEN rating = 'wrong' THEN 4
    ELSE NULL
END WHERE rating_code IS NULL;

UPDATE evaluations SET score = CASE
    WHEN rating_code = 1 THEN 100
    WHEN rating_code = 2 THEN 75
    WHEN rating_code = 3 THEN 25
    WHEN rating_code = 4 THEN 0
    ELSE NULL
END WHERE score IS NULL;
