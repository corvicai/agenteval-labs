-- Templates reutilizáveis de comparação
CREATE TABLE IF NOT EXISTS run_comparison_templates (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    name                TEXT NOT NULL,
    config              JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_run_comparison_templates_workspace ON run_comparison_templates(workspace_id);

-- Snapshots/histórico de comparações executadas
CREATE TABLE IF NOT EXISTS run_comparisons (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    template_id         UUID REFERENCES run_comparison_templates(id) ON DELETE SET NULL,
    created_by_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    name                TEXT NOT NULL,
    run_ids             JSONB NOT NULL,
    labels              JSONB NOT NULL DEFAULT '{}',
    metrics_enabled     JSONB NOT NULL DEFAULT '{}',
    report_data         JSONB NOT NULL DEFAULT '{}',
    runs_snapshot_hash  TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_run_comparisons_workspace ON run_comparisons(workspace_id);
CREATE INDEX IF NOT EXISTS idx_run_comparisons_template ON run_comparisons(template_id);
CREATE INDEX IF NOT EXISTS idx_run_comparisons_created ON run_comparisons(created_at DESC);
