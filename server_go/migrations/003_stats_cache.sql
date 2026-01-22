-- Migration: Create stats_cache table for on-demand stats aggregation
-- This enables caching of pre-computed statistics at workspace, organization, and global levels

CREATE TABLE IF NOT EXISTS stats_cache (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope VARCHAR(20) NOT NULL,  -- 'workspace', 'organization', 'global'
    scope_id UUID,               -- workspace_id or organization_id (NULL for global)
    data JSONB NOT NULL,         -- aggregated stats JSON
    computed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Unique index to ensure only one cache entry per scope
CREATE UNIQUE INDEX IF NOT EXISTS idx_stats_cache_scope ON stats_cache(scope, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'::uuid));

-- Index for quick lookups by scope type
CREATE INDEX IF NOT EXISTS idx_stats_cache_scope_type ON stats_cache(scope);

-- Index for expiration cleanup
CREATE INDEX IF NOT EXISTS idx_stats_cache_expires ON stats_cache(expires_at);
